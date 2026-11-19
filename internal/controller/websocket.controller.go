package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shoonya0/ECHO/internal/config"
	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/realtime"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// connectNameTimeout bounds the name read at connect, so a slow database holds a
// new connection only briefly before it falls back to the token's name (#36).
const connectNameTimeout = 2 * time.Second

// getUsernameFromClient returns the username for a client
func getUsernameFromClient(client *models.Client) string {
	userInfo, err := realtime.GetHubInstance().GetUserInfo(client.UserID)
	if err != nil {
		return client.UserID.Hex() // Fallback to user ID
	}
	return userInfo.Username
}

// normalizeChatID returns a valid chat ID in the lowercase hex the server uses
// for chat channels and hub rooms. ObjectIDFromHex also accepts uppercase, so
// without this a request saved its change but published its event to a channel
// named after the uppercase text, which nobody follows (#60). A value that isn't
// a valid ID is returned unchanged, so the handler still answers INVALID_CHAT_ID.
func normalizeChatID(chatID string) string {
	id, err := bson.ObjectIDFromHex(chatID)
	if err != nil {
		return chatID
	}
	return id.Hex()
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkOrigin,
}

// checkOrigin allows an upgrade without an Origin header (the Android app sends none),
// from the server's own host, or from an origin in config.Cfg.AllowedOrigins. Any
// other web page is refused with 403, so a site can't open a socket in a visitor's
// name once auth also works by cookie or query token (#40).
func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, allowed := range config.Cfg.AllowedOrigins {
		if strings.EqualFold(origin, allowed) {
			return true
		}
	}
	return false
}

// wsLog returns the shared logger for connection-level WebSocket events.
func wsLog() *logrus.Logger { return logger.Base() }

// connectionLogFields names a new connection in the logs by IDs only. It is
// logged at Info on every connect, so it must hold no personal data (#88).
func connectionLogFields(user models.LoginUserResponse, client *models.Client) logrus.Fields {
	return logrus.Fields{
		string(constants.UserIDKey):   user.ID.Hex(),
		string(constants.ClientIDKey): client.ID,
	}
}

// connectionPresence returns the presence a new connection registers with:
// the status and privacy settings the user chose, read from MongoDB, since
// the token's copy dates from login, up to 7 days ago (#31, #32). When the
// read fails the user counts as invisible: an outage mustn't announce a user
// who chose to hide. REST still shows them, from their stored settings.
func connectionPresence(ctx context.Context, log logrus.Entry, user models.LoginUserResponse, clientID string) *realtime.UserPresence {
	settings, err := services.GetPresenceSettings(ctx, user.ID)
	if err != nil {
		log.WithError(err).WithField(string(constants.UserIDKey), user.ID.Hex()).Warn("Could not read the user's presence settings; not announcing them")
		settings = services.PresenceSettings{Status: constants.UserStatusInvisible}
	}
	presence := realtime.PresenceFromSettings(user.ID, settings)
	presence.ClientID, presence.DeviceInfo = clientID, user.Presence.DeviceInfo
	return &presence
}

// HandleWebSocketChat upgrades the request to a WebSocket and serves the chat
// protocol until the client disconnects.
func HandleWebSocketChat(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user not found")
		return
	}

	user, exists := reqCtx.Value(constants.UserDataKey).(models.LoginUserResponse)
	if !exists {
		log.Debug("user not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// AuthMiddleware refuses tokens without a jti, so every connection has one (#33).
	claimsValue, _ := ctx.Get("jwtClaims")
	claims, ok := claimsValue.(utils.JwtClaims)
	if !ok || claims.ID == "" {
		log.Debug("jwt claims not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// An account at its connection limit is refused before the upgrade (#40).
	// QueueRegister checks again under the hub's lock, for two connects at once.
	hub := realtime.GetHubInstance()
	if hub.ConnectionCount(user.ID.Hex()) >= realtime.MaxConnectionsPerUser {
		log.WithField(string(constants.UserIDKey), user.ID.Hex()).Warn("Refused a WebSocket connection over the account limit")
		utils.ErrorResponse(ctx, http.StatusTooManyRequests, "Too many connections", nil)
		return
	}

	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil) // Upgrade HTTP connection to WebSocket
	if err != nil {
		// The upgrader has already answered (403 for a refused origin, 400 for a bad
		// handshake); a second reply would be appended to it.
		log.WithError(err).Warn("Failed to upgrade WebSocket connection")
		return
	}

	// The JWT's name is from login and it has no avatar; join and leave events show
	// these (#36). A failed read keeps the JWT's: a name isn't worth refusing the connection.
	nameCtx, cancelName := context.WithTimeout(reqCtx, connectNameTimeout)
	current, err := services.WithCurrentDisplayInfo(nameCtx, user)
	cancelName()
	if err != nil {
		log.WithError(err).Warn("Could not read the user's current name; using the token's")
	} else {
		user = current
	}

	client := &models.Client{ // Create new client with unique ID for each connection
		ID:         uuid.New().String(), // Generate unique ID for each WebSocket connection
		UserID:     user.ID,
		TokenID:    claims.ID,     // a logout closes only this token's connections (#76)
		Username:   user.Username, // shown on join and leave events (#50)
		Avatar:     user.Profile.Avatar,
		Connection: conn,
		Send:       make(chan models.WebSocketMessage, 10),
	}
	if claims.ExpiresAt != nil {
		client.TokenExpiresAt = claims.ExpiresAt.Time // the re-check closes the connection then (#76)
	}

	hub.UpdateUserInfoCache(user.ID.Hex(), models.UserDisplayInfo{
		Email:       user.Email,
		Username:    user.Username,
		DisplayName: user.Profile.DisplayName,
	})

	// The registration decides from this presence whether to announce the
	// user online (#31). QueueRegister gives up if the hub stopped;
	// WaitRegistered reports it (#22).
	if !hub.QueueRegister(client, connectionPresence(reqCtx, log, user, client.ID)) {
		log.WithFields(connectionLogFields(user, client)).Warn("Refused a WebSocket connection over the account limit")
		closeTooManyConnections(conn)
		return
	}

	// Read requests only once the hub has registered the connection: a
	// join_chat handled earlier would be dropped (#122).
	waitCtx, cancelWait := context.WithTimeout(reqCtx, registrationTimeout)
	err = hub.WaitRegistered(waitCtx, client)
	cancelWait()
	if err != nil {
		log.WithError(err).WithFields(connectionLogFields(user, client)).Error("WebSocket connection was not registered")
		hub.QueueUnregister(client) // a registration still queued then skips it (#54)
		_ = conn.Close()
		return
	}

	log.WithFields(connectionLogFields(user, client)).Info("New WebSocket connection established")

	go func() {
		handleClientWrite(client) // Handle writing in a simple goroutine
	}()

	handleClientRead(reqCtx, client) // Handle reading in the main routine (blocking)
}

// registrationTimeout is how long a new connection waits for the hub to
// register it before it is closed.
const registrationTimeout = 10 * time.Second

// closeHandshakeTimeout is how long the read loop waits for the client to answer
// the server's close frame before the socket is closed anyway.
const closeHandshakeTimeout = 5 * time.Second

// handleClientWrite handles writing messages to the WebSocket connection
// message to be sent to the client
func handleClientWrite(client *models.Client) {
	ticker := time.NewTicker(54 * time.Second) // Ping every 54 seconds
	defer func() {
		ticker.Stop()
		wsLog().Printf("Write goroutine closed for client: %s", client.ID)
	}()

	for {
		select {
		case message, ok := <-client.Send:
			client.Connection.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// Send closed: say why if the session ended (#76). The read loop
				// closes the socket once the client answers, or at this deadline.
				// Closing it here, with client data still unread, can reset the
				// connection and lose the frame, so the app would reconnect
				// instead of logging out.
				frame := []byte{}
				if code, reason := client.CloseReason(); code != 0 {
					frame = websocket.FormatCloseMessage(code, reason)
				}
				_ = client.Connection.WriteMessage(websocket.CloseMessage, frame) // best effort: the peer may be gone
				_ = client.Connection.SetReadDeadline(time.Now().Add(closeHandshakeTimeout))
				return
			}

			if err := client.Connection.WriteJSON(message); err != nil { // Send message as JSON
				wsLog().Printf("Failed to write message to client %s: %v", client.ID, err)
				return
			}

			client.Touch() // Update client activity
		case <-ticker.C:
			client.Connection.SetWriteDeadline(time.Now().Add(10 * time.Second))

			if err := client.Connection.WriteMessage(websocket.PingMessage, nil); err != nil { // Send ping
				wsLog().Printf("Failed to send ping to client %s: %v", client.ID, err)
				return
			}
			// Update presence
		}
	}
}

// handleClientRead handles reading messages from the WebSocket connection
// message to be read from the client
func handleClientRead(ctx context.Context, client *models.Client) {
	defer func() {
		realtime.GetHubInstance().QueueUnregister(client) // never waits on a stopped hub (#22)
		_ = client.Connection.Close()
		wsLog().Printf("Read goroutine closed for client: %s", client.ID)
	}()

	client.Connection.SetReadLimit(maxFrameBytes) // a bigger frame closes with 1009 (#40)
	limiter := newConnectionLimiter()
	client.Connection.SetPingHandler(limitedPingHandler(client.Connection, limiter))

	client.Connection.SetReadDeadline(time.Now().Add(60 * time.Second)) // Set read deadline and pong handler
	client.Connection.SetPongHandler(func(string) error {
		if client.IsClosed() {
			return nil // closing: pongs must not push back the close deadline (#76)
		}
		client.Connection.SetReadDeadline(time.Now().Add(60 * time.Second))
		client.Touch()
		return nil
	})

	for {
		_, message, err := client.Connection.ReadMessage() // Read message from client
		if err != nil {
			// Any read error means the connection is no longer usable.
			// Never re-read a dead socket — gorilla panics on repeated read.
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				wsLog().Printf("WebSocket error for client %s: %v", client.ID, err)
			} else {
				wsLog().Printf("WebSocket read failed for client %s: %v", client.ID, err)
			}
			break
		}

		// A connection being closed, for example after logout, may not act (#76).
		// Keep reading until the client answers the close frame (a read error), so
		// the frame isn't lost; see handleClientWrite.
		if client.IsClosed() {
			continue
		}

		// Every frame counts, unreadable ones too, so a flood costs no database work (#40).
		if !limiter.allow(time.Now()) {
			refuseRateLimited(client, message)
			continue
		}

		var request models.MessageRequest // Parse the message into a MessageRequest
		if err := json.Unmarshal(message, &request); err != nil {
			wsLog().Printf("Failed to unmarshal message from client %s: %v", client.ID, err)
			sendErrorResponse(client, "", "PARSE_ERROR", "Message is not valid JSON") // no decoder text (#41)
			continue                                                                  // SAFE: connection is still valid, just skip this bad message
		}

		client.Touch() // Update client activity

		handleClientRequest(ctx, client, request) // Process the request
	}
}

// handleClientRequest processes different types of client requests
func handleClientRequest(ctx context.Context, client *models.Client, request models.MessageRequest) {
	request.ChatID = normalizeChatID(request.ChatID)

	switch request.Type {
	case models.WSRequestTypeSendMessage:
		handleSendMessage(ctx, client, request)

	case models.WSRequestTypeJoinChat:
		handleJoinChat(ctx, client, request)

	case models.WSRequestTypeLeaveChat:
		handleLeaveChat(client, request)

	case models.WSRequestTypeSetTyping:
		handleSetTyping(ctx, client, request)

	case models.WSRequestTypeMarkRead:
		handleMarkRead(ctx, client, request)

	case models.WSRequestTypeEditMessage:
		handleEditMessage(client, request)

	case models.WSRequestTypeDeleteMessage:
		handleDeleteMessage(client, request)

	case models.WSRequestTypeAddReaction:
		handleAddReaction(ctx, client, request)

	case models.WSRequestTypeRemoveReaction:
		handleRemoveReaction(ctx, client, request)

	case models.WSRequestTypeInviteUser:
		handleInviteUser(ctx, client, request)

	case models.WSRequestTypeRemoveUser:
		handleRemoveUser(ctx, client, request)

	case models.WSRequestTypeUpdateChat:
		handleUpdateChat(client, request)

	default:
		sendErrorResponse(client, request.RequestID, "UNKNOWN_REQUEST", "Unknown request type: "+request.Type)
	}
}

// ============ REQUEST HANDLERS ============
// handleSendMessage processes message sending requests with comprehensive validation
func handleSendMessage(ctx context.Context, client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" || request.Content == "" { // Validate request
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID and content are required")
		return
	}

	chatID, err := bson.ObjectIDFromHex(request.ChatID) // Parse chat ID
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	// Membership first, so an unknown chat gets the same answer as one the
	// caller isn't in (#59). The same read returns the chat (#106).
	chat, err := services.GetChatForMember(ctx, chatID, client.UserID)
	if err != nil {
		if !errors.Is(err, services.ErrNotParticipant) {
			wsLog().Printf("Failed to check chat membership: %v", err)
		}
		sendChatError(client, request.RequestID, err, "PERMISSION_CHECK_FAILED", "Failed to validate permissions")
		return
	}

	canSend, err := validateMessagePermissions(ctx, &chat, client.UserID) // Check if user has permission to send messages in this chat
	if errors.Is(err, services.ErrBlocked) {
		sendErrorResponse(client, request.RequestID, "PERMISSION_DENIED", "User is blocked")
		return
	}
	if err != nil {
		wsLog().Printf("Failed to validate message permissions: %v", err)
		sendErrorResponse(client, request.RequestID, "PERMISSION_CHECK_FAILED", "Failed to validate permissions")
		return
	}

	if !canSend {
		sendErrorResponse(client, request.RequestID, "PERMISSION_DENIED", "You don't have permission to send messages in this chat")
		return
	}

	if problem := validateMessageContent(request); problem != "" { // Validate message content and type
		sendErrorResponse(client, request.RequestID, "INVALID_CONTENT", problem)
		return
	}

	messageType := request.MessageType // Set default message type if not provided
	if messageType == "" {
		messageType = "text"
	}

	message, err := services.SendMessage( // Send message through service layer
		ctx,
		chatID,
		client.UserID,
		request.Content,
		messageType,
		request.Attachments,
		request.Mentions,
	)

	if err != nil {
		wsLog().Printf("Failed to send message: %v", err)
		sendErrorResponse(client, request.RequestID, "MESSAGE_FAILED", "Failed to send message")
		return
	}

	response := models.MessageResponse{ // Send success response
		Success:   true,
		MessageID: message.ID.Hex(),
		Data: map[string]interface{}{
			"messageId":   message.ID.Hex(),
			"timestamp":   message.CreatedAt,
			"chatId":      request.ChatID,
			"messageType": messageType,
		},
		RequestID: request.RequestID,
	}

	wsMessage := models.WebSocketMessage{
		Type:      models.WSMessageTypeResponse,
		Data:      response,
		RequestID: request.RequestID, // at the top level too, like every other reply (#47)
		Timestamp: time.Now(),
	}

	if !client.TrySend(wsMessage) {
		wsLog().Printf("Client %s send channel is full, dropping message", client.ID)
		errorResponse := models.WebSocketMessage{ // Send error response about dropped message
			Type: models.WSMessageTypeError,
			Data: models.ErrorMessage{
				Code:    "CHANNEL_FULL",
				Message: "Message dropped due to full send channel",
			},
			RequestID: request.RequestID,
			Timestamp: time.Now(),
		}
		if !client.TrySend(errorResponse) { // if that fails too, the client is probably unresponsive
			wsLog().Printf("Client %s is unresponsive, marking for cleanup", client.ID)
		}
	}

	wsLog().Printf("Message sent by client %s in chat %s", client.ID, request.ChatID)
}

// joinChatRoom adds the connection to the chat's room. Tests replace it so they
// don't need a hub with Redis.
var joinChatRoom = func(ctx context.Context, client *models.Client, chatID string) error {
	return realtime.GetHubInstance().JoinChat(ctx, client, chatID)
}

// handleJoinChat starts following a chat on this connection: the chat named by
// chatId, if the caller is a participant (#48), or else the direct chat with
// the user named by senderId, created if it doesn't exist. Both answer with a
// response that names the chat.
func handleJoinChat(ctx context.Context, client *models.Client, request models.MessageRequest) {
	switch {
	case request.ChatID != "":
		joinChatByID(ctx, client, request)
	case request.SenderID != "":
		joinDirectChat(ctx, client, request)
	default:
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "chatId or senderId is required")
	}
}

// joinChatByID follows a chat the caller is a participant of. Membership is
// checked first, and an unknown chat gets the same answer as someone else's
// (#23, #59). A direct chat with a block either way is refused, as it is by
// senderId (#27).
func joinChatByID(ctx context.Context, client *models.Client, request models.MessageRequest) {
	chatID, err := bson.ObjectIDFromHex(request.ChatID)
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}
	if err := services.RequireChatAccess(ctx, chatID, client.UserID); err != nil {
		if !errors.Is(err, services.ErrNotParticipant) {
			wsLog().Printf("Failed to check chat membership for join: %v", err)
		}
		sendChatError(client, request.RequestID, err, "JOIN_FAILED", "Failed to join chat")
		return
	}
	followChat(ctx, client, request.RequestID, chatID.Hex())
}

// followChat joins the room and answers the request. A removal during the join
// undoes it and is refused; a failure to announce the join doesn't undo it, so
// it is logged and the join still succeeds.
func followChat(ctx context.Context, client *models.Client, requestID, chatID string) {
	if err := joinChatRoom(ctx, client, chatID); err != nil {
		if errors.Is(err, realtime.ErrNotInChat) {
			sendChatError(client, requestID, services.ErrNotParticipant, "JOIN_FAILED", "Failed to join chat")
			return
		}
		wsLog().Printf("Joined chat %s but announcing it failed: %v", chatID, err)
	}
	sendSuccessResponse(client, requestID, map[string]any{"chatId": chatID})
	wsLog().Printf("Client %s joined chat %s", client.ID, chatID)
}

// joinDirectChat opens the direct chat with the user named by senderId and
// follows it.
func joinDirectChat(ctx context.Context, client *models.Client, request models.MessageRequest) {
	log := logger.WithContext(ctx)
	if ctx.Value(constants.UserDataKey) == nil {
		log.WithError(errors.New("user not Found")).Error("User not authenticated")
		sendErrorResponse(client, request.RequestID, "USER_NOT_AUTHENTICATED", "User not authenticated")
		return
	}

	targetUserID, err := bson.ObjectIDFromHex(request.SenderID) // Parse sender user ID
	if err != nil {
		log.WithError(err).Error("Invalid sender user ID")
		sendErrorResponse(client, request.RequestID, "INVALID_USER_ID", "Invalid sender user ID")
		return
	}

	chat, err := services.CreateDirectChat(ctx, targetUserID) // create a new chat if it doesn't exist
	if errors.Is(err, services.ErrSelfDirectChat) {
		sendErrorResponse(client, request.RequestID, "INVALID_USER_ID", "Cannot open a direct chat with yourself")
		return
	}
	if errors.Is(err, services.ErrBlocked) {
		sendErrorResponse(client, request.RequestID, "PERMISSION_DENIED", "User is blocked")
		return
	}
	if errors.Is(err, services.ErrUserNotFound) {
		sendErrorResponse(client, request.RequestID, "INVALID_USER_ID", "User not found")
		return
	}
	if err != nil {
		log.WithError(err).Error("Failed to create chat")
		sendErrorResponse(client, request.RequestID, "CHAT_CREATION_FAILED", "Failed to create chat")
		return
	}

	followChat(ctx, client, request.RequestID, chat.ChatID.Hex())
}

// handleLeaveChat processes chat leaving requests
func handleLeaveChat(client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	chatID, err := bson.ObjectIDFromHex(request.ChatID) // Parse chat ID
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	// Hex() gives the lowercase form the hub's rooms are keyed by.
	if err := realtime.GetHubInstance().LeaveChat(client, chatID.Hex()); err != nil {
		wsLog().Printf("Failed to leave chat: %v", err)
		if errors.Is(err, realtime.ErrNotInChat) {
			sendErrorResponse(client, request.RequestID, "PERMISSION_DENIED", "You are not in this chat")
			return
		}
		sendErrorResponse(client, request.RequestID, "LEAVE_FAILED", "Failed to leave chat")
		return
	}

	wsLog().Printf("Client %s left chat %s", client.ID, request.ChatID)
}

// handleSetTyping processes typing indicator requests
func handleSetTyping(ctx context.Context, client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	chatID, err := bson.ObjectIDFromHex(request.ChatID) // Parse chat ID
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	isTyping := false // Extract typing status from metadata
	if request.Metadata != nil {
		if typing, ok := request.Metadata["isTyping"].(bool); ok {
			isTyping = typing
		}
	}

	err = services.UpdateTypingStatus(ctx, chatID, client.UserID, isTyping) // Update typing status
	if err != nil {
		wsLog().Printf("Failed to update typing status: %v", err)
		sendChatError(client, request.RequestID, err, "TYPING_FAILED", "Failed to update typing status")
		return
	}

	wsLog().Printf("Client %s typing status: %v in chat %s", client.ID, isTyping, request.ChatID)
}

// handleMarkRead processes message read status updates
func handleMarkRead(ctx context.Context, client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	chatID, err := bson.ObjectIDFromHex(request.ChatID) // Parse chat ID
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	messageIDs := []bson.ObjectID{} // Extract message IDs from metadata
	if request.Metadata != nil {
		if msgIDs, ok := request.Metadata["messageIds"].([]interface{}); ok {
			for _, id := range msgIDs {
				if idStr, ok := id.(string); ok {
					if msgID, err := bson.ObjectIDFromHex(idStr); err == nil {
						messageIDs = append(messageIDs, msgID)
					}
				}
			}
		}
	}

	if len(messageIDs) == 0 {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "Message IDs are required")
		return
	}

	err = services.MarkMessagesAsRead(ctx, chatID, client.UserID, messageIDs) // Mark messages as read
	if err != nil {
		wsLog().Printf("Failed to mark messages as read: %v", err)
		sendChatError(client, request.RequestID, err, "READ_FAILED", "Failed to mark messages as read")
		return
	}

	wsLog().Printf("Client %s marked %d messages as read in chat %s", client.ID, len(messageIDs), request.ChatID)
}

// handleAddReaction processes message reaction addition requests
func handleAddReaction(ctx context.Context, client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	messageID, emoji := "", "" // Extract messageID and emoji from metadata
	if request.Metadata != nil {
		if msgID, ok := request.Metadata["messageId"].(string); ok {
			messageID = msgID
		}
		if em, ok := request.Metadata["emoji"].(string); ok {
			emoji = em
		}
	}

	if messageID == "" || emoji == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "MessageID and emoji are required")
		return
	}

	chatID, err := bson.ObjectIDFromHex(request.ChatID) // Parse IDs
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	msgObjectID, err := bson.ObjectIDFromHex(messageID)
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_MESSAGE_ID", "Invalid message ID format")
		return
	}

	changed, err := services.AddMessageReaction(ctx, chatID, msgObjectID, client.UserID, emoji) // Add reaction through service layer
	if err != nil {
		wsLog().Printf("Failed to add reaction: %v", err)
		sendChatError(client, request.RequestID, err, "REACTION_FAILED", "Failed to add reaction")
		return
	}

	// A user who had already reacted changed nothing, so the chat isn't told (#25).
	if changed {
		reactionEvent := models.WebSocketMessage{ // Broadcast reaction to chat
			Type:   models.WSMessageTypeReaction,
			ChatID: request.ChatID,
			UserID: client.UserID.Hex(),
			Data: models.MessageReaction{
				MessageID: messageID,
				UserID:    client.UserID.Hex(),
				Username:  getUsernameFromClient(client),
				Emoji:     emoji,
				Action:    "add",
				Timestamp: time.Now(),
			},
			Timestamp: time.Now(),
		}

		err = services.BroadcastToChat(request.ChatID, reactionEvent) // Broadcast reaction to chat
		if err != nil {
			wsLog().Printf("Failed to broadcast reaction: %v", err)
		}
	}

	sendSuccessResponse(client, request.RequestID, map[string]interface{}{ // Send success response
		"action":    "reaction_added",
		"messageId": messageID,
		"emoji":     emoji,
		"changed":   changed,
	})

	wsLog().Printf("Client %s added reaction %s to message %s", client.ID, emoji, messageID)
}

// handleRemoveReaction processes message reaction removal requests
func handleRemoveReaction(ctx context.Context, client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	messageID, emoji := "", "" // Extract messageID and emoji from metadata
	if request.Metadata != nil {
		if msgID, ok := request.Metadata["messageId"].(string); ok {
			messageID = msgID
		}
		if em, ok := request.Metadata["emoji"].(string); ok {
			emoji = em
		}
	}

	if messageID == "" || emoji == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "MessageID and emoji are required")
		return
	}

	chatID, err := bson.ObjectIDFromHex(request.ChatID) // Parse IDs
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	msgObjectID, err := bson.ObjectIDFromHex(messageID)
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_MESSAGE_ID", "Invalid message ID format")
		return
	}

	changed, err := services.RemoveMessageReaction(ctx, chatID, msgObjectID, client.UserID, emoji) // Remove reaction through service layer
	if err != nil {
		wsLog().Printf("Failed to remove reaction: %v", err)
		sendChatError(client, request.RequestID, err, "REACTION_FAILED", "Failed to remove reaction")
		return
	}

	// A user who hadn't reacted changed nothing, so the chat isn't told (#25).
	if changed {
		reactionEvent := models.WebSocketMessage{ // Broadcast reaction removal to chat
			Type:   models.WSMessageTypeReaction,
			ChatID: request.ChatID,
			UserID: client.UserID.Hex(),
			Data: models.MessageReaction{
				MessageID: messageID,
				UserID:    client.UserID.Hex(),
				Username:  getUsernameFromClient(client),
				Emoji:     emoji,
				Action:    "remove",
				Timestamp: time.Now(),
			},
			Timestamp: time.Now(),
		}

		err = services.BroadcastToChat(request.ChatID, reactionEvent)
		if err != nil {
			wsLog().Printf("Failed to broadcast reaction removal: %v", err)
		}
	}

	sendSuccessResponse(client, request.RequestID, map[string]interface{}{ // Send success response
		"action":    "reaction_removed",
		"messageId": messageID,
		"emoji":     emoji,
		"changed":   changed,
	})

	wsLog().Printf("Client %s removed reaction %s from message %s", client.ID, emoji, messageID)
}

// handleEditMessage processes message editing requests
func handleEditMessage(client *models.Client, request models.MessageRequest) {
	// TODO: Implement message editing
	sendErrorResponse(client, request.RequestID, "NOT_IMPLEMENTED", "Message editing not yet implemented")
}

// handleDeleteMessage processes message deletion requests
func handleDeleteMessage(client *models.Client, request models.MessageRequest) {
	// TODO: Implement message deletion
	sendErrorResponse(client, request.RequestID, "NOT_IMPLEMENTED", "Message deletion not yet implemented")
}

// handleInviteUser processes user invitation requests for group chats.
// Requires: chatId (valid group), metadata.userIds ([]interface{} of hex user IDs).
// The acting user must be a participant with owner or admin role.
func handleInviteUser(ctx context.Context, client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	chatID, err := bson.ObjectIDFromHex(request.ChatID)
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	// Extract user IDs from metadata
	userIDs := []bson.ObjectID{}
	if request.Metadata != nil {
		if rawIDs, ok := request.Metadata["userIds"].([]interface{}); ok {
			for _, raw := range rawIDs {
				if idStr, ok := raw.(string); ok {
					if id, err := bson.ObjectIDFromHex(idStr); err == nil {
						userIDs = append(userIDs, id)
					}
				}
			}
		}
	}

	if len(userIDs) == 0 {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "At least one valid userId is required")
		return
	}

	// Membership before anything that can fail differently, so a non-member
	// can't tell whether the chat exists or what type it is (#59). The same
	// read returns the chat (#106).
	chat, err := services.GetChatForMember(ctx, chatID, client.UserID)
	if err != nil {
		if !errors.Is(err, services.ErrNotParticipant) {
			wsLog().Printf("Failed to check chat membership for invite: %v", err)
		}
		sendChatError(client, request.RequestID, err, "INVITE_FAILED", "Failed to invite users")
		return
	}

	if chat.ChatType != string(constants.ChatTypeGroup) {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "Can only invite users to group chats")
		return
	}

	// Only owners and admins can invite. GetChatForMember found the caller in
	// participants. This check gives the specific text; the service's own
	// role check catches a demotion during the add (see sendInviteError).
	actingParticipant := chat.Participants[client.UserID]
	if actingParticipant.Role != string(constants.ChatRoleOwner) && actingParticipant.Role != string(constants.ChatRoleAdmin) {
		sendErrorResponse(client, request.RequestID, "PERMISSION_DENIED", "Only owners and admins can invite users")
		return
	}

	// Add members through service layer
	err = services.AddGroupMemberToChat(ctx, client.UserID, chat, userIDs)
	if err != nil {
		wsLog().Printf("Failed to add group members: %v", err)
		sendInviteError(client, request.RequestID, err)
		return
	}

	// Build invited user ID strings for the event
	invitedIDStrings := make([]string, len(userIDs))
	for i, id := range userIDs {
		invitedIDStrings[i] = id.Hex()
	}

	// Broadcast invite event to the chat
	inviteEvent := models.WebSocketMessage{
		Type:   models.WSMessageTypeChatUpdated,
		ChatID: request.ChatID,
		UserID: client.UserID.Hex(),
		Data: models.UserInvite{
			ChatID:      request.ChatID,
			InviterID:   client.UserID.Hex(),
			InviterName: getUsernameFromClient(client),
			InviteeID:   "",
			InviteeName: "",
			Timestamp:   time.Now(),
		},
		Timestamp: time.Now(),
	}

	if err := services.BroadcastToChat(request.ChatID, inviteEvent); err != nil {
		wsLog().Printf("Failed to broadcast invite event: %v", err)
	}

	sendSuccessResponse(client, request.RequestID, map[string]interface{}{
		"action":       "users_invited",
		"chatId":       request.ChatID,
		"invitedUsers": invitedIDStrings,
	})

	wsLog().Printf("Client %s invited %d users to chat %s", client.ID, len(userIDs), request.ChatID)
}

// handleRemoveUser processes user removal requests from group chats.
// Requires: chatId (valid group), metadata.userIds ([]interface{} of hex user IDs).
// The acting user must be an owner or admin. Cannot remove self.
func handleRemoveUser(ctx context.Context, client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	chatID, err := bson.ObjectIDFromHex(request.ChatID)
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	// Extract user IDs from metadata
	userIDs := []bson.ObjectID{}
	if request.Metadata != nil {
		if rawIDs, ok := request.Metadata["userIds"].([]interface{}); ok {
			for _, raw := range rawIDs {
				if idStr, ok := raw.(string); ok {
					if id, err := bson.ObjectIDFromHex(idStr); err == nil {
						userIDs = append(userIDs, id)
					}
				}
			}
		}
	}

	if len(userIDs) == 0 {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "At least one valid userId is required")
		return
	}

	// Membership first, so a non-member can't tell whether the chat exists (#59).
	if err := services.RequireChatMember(ctx, chatID, client.UserID); err != nil {
		if !errors.Is(err, services.ErrNotParticipant) {
			wsLog().Printf("Failed to check chat membership for remove: %v", err)
		}
		sendChatError(client, request.RequestID, err, "REMOVE_FAILED", "Failed to remove users")
		return
	}

	// Remove members through service layer (validates permissions, chat type, etc.)
	removedIDs, err := services.RemoveGroupMember(ctx, client.UserID, chatID, userIDs)
	if err != nil {
		wsLog().Printf("Failed to remove group members: %v", err)

		errCode := "REMOVE_FAILED"
		clientMsg := "Failed to remove users"

		// The caller passed the membership check, so a missing group means the
		// chat isn't a group (or was just deleted), as invite_user reports it.
		if errors.Is(err, services.ErrChatNotFound) {
			errCode = "INVALID_REQUEST"
			clientMsg = "Can only remove users from group chats"
		} else if errors.Is(err, services.ErrNotParticipant) || errors.Is(err, services.ErrPermissionDenied) {
			errCode = "PERMISSION_DENIED"
			clientMsg = "You do not have permission to remove users"
		} else if errors.Is(err, services.ErrNoValidParticipants) {
			errCode = "INVALID_REQUEST"
			clientMsg = "No valid participants to remove"
		}

		sendErrorResponse(client, request.RequestID, errCode, clientMsg)
		return
	}

	// Report only the users actually removed, not skipped ones such as the
	// actor (#28).
	removedIDStrings := make([]string, len(removedIDs))
	for i, id := range removedIDs {
		removedIDStrings[i] = id.Hex()
	}

	// Broadcast removal event to the chat
	removeEvent := models.WebSocketMessage{
		Type:   models.WSMessageTypeChatUpdated,
		ChatID: request.ChatID,
		UserID: client.UserID.Hex(),
		Data: models.UserRemove{
			ChatID:      request.ChatID,
			RemovedByID: client.UserID.Hex(),
			RemovedBy:   getUsernameFromClient(client),
			RemovedID:   "",
			RemovedUser: "",
			Timestamp:   time.Now(),
		},
		Timestamp: time.Now(),
	}

	if err := services.BroadcastToChat(request.ChatID, removeEvent); err != nil {
		wsLog().Printf("Failed to broadcast remove event: %v", err)
	}

	// After the event, so the removed members still get it and nothing after
	// it (#14). The removal is saved either way, so a failure is only logged.
	if len(removedIDStrings) > 0 {
		if err := services.RemoveMembersFromLiveChat(request.ChatID, removedIDStrings); err != nil {
			wsLog().Printf("Failed to remove members from the live chat: %v", err)
		}
	}

	sendSuccessResponse(client, request.RequestID, map[string]interface{}{
		"action":       "users_removed",
		"chatId":       request.ChatID,
		"removedUsers": removedIDStrings,
	})

	wsLog().Printf("Client %s removed %d users from chat %s", client.ID, len(removedIDs), request.ChatID)
}

// handleUpdateChat processes chat update requests
func handleUpdateChat(client *models.Client, request models.MessageRequest) {
	// TODO: Implement chat updates
	sendErrorResponse(client, request.RequestID, "NOT_IMPLEMENTED", "Chat updates not yet implemented")
}

// ============ UTILITY FUNCTIONS ============

// validateMessagePermissions checks if a user can send messages in chat, which
// the caller has already read (services.GetChatForMember).
func validateMessagePermissions(ctx context.Context, chat *models.Chat, userID bson.ObjectID) (bool, error) {
	participant, isParticipant := chat.Participants[userID]
	if !isParticipant {
		return false, nil
	}

	if participant.IsBlocked || participant.IsMuted { // Check if user is blocked or muted

		return false, nil
	}

	switch chat.ChatType {
	case string(constants.ChatTypeDirect):
		// participant.IsBlocked is never set; the users' block lists decide (#27).
		if err := services.CheckDirectChatBlock(ctx, chat, userID); err != nil {
			return false, err
		}
		return true, nil

	case string(constants.ChatTypeGroup):
		permissions := participant.Permissions
		if len(permissions) > 0 {
			for _, perm := range permissions {
				// Group members are granted "write" (see services.CreateGroupChat).
				if perm == string(constants.ChatPermissionWrite) || perm == "send_message" || perm == "admin" || perm == "owner" {
					return true, nil
				}
			}
			return false, nil
		}
		return true, nil // Default: members can send

	case string(constants.ChatTypeChannel):
		return participant.Role == "admin" || participant.Role == "owner", nil

	default:
		return false, nil
	}
}

// validateMessageContent checks message content and attachments. It returns the
// problem as a fixed text for the client, or "" when there is none. The text
// never includes request values or internal errors (#41).
func validateMessageContent(request models.MessageRequest) string {
	// Characters, not bytes, as the error says (#51). 4000 of them are at most
	// 16000 bytes, which still fits maxFrameBytes.
	if utf8.RuneCountInString(request.Content) > maxContentChars {
		return "message content too long (max 4000 characters)"
	}

	validTypes := map[string]bool{
		"text":  true,
		"image": true,
		"file":  true,
		"audio": true,
		"video": true,
	}

	if request.MessageType != "" && !validTypes[request.MessageType] {
		return "invalid message type"
	}

	if len(request.Attachments) > 10 { // Max 10 attachments
		return "too many attachments (max 10)"
	}

	for _, attachment := range request.Attachments {
		if attachment.Size > 50*1024*1024 { // 50MB limit
			return "attachment too large (max 50MB)"
		}
		if attachment.URL == "" || attachment.OriginalName == "" {
			return "invalid attachment data"
		}
	}

	if len(request.Mentions) > 20 { // Max 20 mentions
		return "too many mentions (max 20)"
	}

	return ""
}

// sendSuccessResponse sends a success response to the client
func sendSuccessResponse(client *models.Client, requestID string, data map[string]interface{}) {
	response := models.MessageResponse{
		Success:   true,
		Data:      data,
		RequestID: requestID,
	}

	wsMessage := models.WebSocketMessage{
		Type:      models.WSMessageTypeResponse,
		Data:      response,
		RequestID: requestID,
		Timestamp: time.Now(),
	}

	if !client.TrySend(wsMessage) {
		wsLog().Printf("Failed to send success response to client %s: channel full", client.ID)
	}
}

// sendChatError reports a failed chat action. Known service errors get their own
// code; anything else gets the caller's code and message, never err's own text.
func sendChatError(client *models.Client, requestID string, err error, code, message string) {
	switch {
	case errors.Is(err, services.ErrNotParticipant):
		code, message = "PERMISSION_DENIED", "You are not a participant of this chat"
	case errors.Is(err, services.ErrBlocked):
		code, message = "PERMISSION_DENIED", "User is blocked"
	case errors.Is(err, services.ErrMessageNotFound):
		code, message = "MESSAGE_NOT_FOUND", "Message not found"
	case errors.Is(err, services.ErrInvalidEmoji):
		code, message = "INVALID_REQUEST", "Invalid emoji"
	}
	sendErrorResponse(client, requestID, code, message)
}

// sendInviteError answers a failed AddGroupMemberToChat with a fixed text (#41). The
// handler has already checked the caller's role, so ErrPermissionDenied here
// means the chat changed during the add: it keeps INVITE_FAILED, which the
// client may retry.
func sendInviteError(client *models.Client, requestID string, err error) {
	code, message := "INVITE_FAILED", "Failed to invite users"
	switch {
	case errors.Is(err, services.ErrChatNotFound):
		code, message = "INVALID_CHAT_ID", "Chat not found"
	case errors.Is(err, services.ErrNotParticipant):
		code, message = "PERMISSION_DENIED", "You are not a participant of this chat"
	case errors.Is(err, services.ErrAlreadyParticipant):
		code, message = "INVALID_REQUEST", "Users are already in the chat"
	case errors.Is(err, services.ErrGroupFull):
		code, message = "GROUP_FULL", "Group is full"
	case errors.Is(err, services.ErrUserNotFound):
		code, message = "INVALID_USER_ID", "User not found"
	}
	sendErrorResponse(client, requestID, code, message)
}

// sendErrorResponse sends an error response to the client
func sendErrorResponse(client *models.Client, requestID, code, message string) {
	errorMsg := models.WebSocketMessage{
		Type: models.WSMessageTypeError,
		Data: models.ErrorMessage{
			Code:    code,
			Message: message,
		},
		RequestID: requestID,
		Timestamp: time.Now(),
	}

	if !client.TrySend(errorMsg) {
		wsLog().Printf("Failed to send error response to client %s: channel full", client.ID)
	}
}
