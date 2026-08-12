package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gin/internal/models"
	"gin/internal/services"
	"gin/internal/utils"
	"gin/logger"
	"gin/objects"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ WEBSOCKET CONTROLLER FOR REAL-TIME CHAT ============
// JwtClaims represents JWT token claims
type JwtClaims struct {
	jwt.RegisteredClaims
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Exp    int64  `json:"exp"`
}

// getUsernameFromClient returns the username for a client
func getUsernameFromClient(client *models.Client) string {
	userInfo, err := services.GetHubInstance().GetUserInfo(client.UserID)
	if err != nil {
		return client.UserID.Hex() // Fallback to user ID
	}
	return userInfo.Username
}

// WebSocket upgrader with CORS support
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins (configure for production)
	},
}

// HandleWebSocketChat handles WebSocket connections for chat functionality
func HandleWebSocketChat(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user not found")
		return
	}

	user, exists := reqCtx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if !exists {
		log.Debug("user not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil) // Upgrade HTTP connection to WebSocket
	if err != nil {
		log.WithError(err).Error("Failed to upgrade WebSocket connection")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Failed to upgrade WebSocket connection", nil)
		return
	}

	client := &models.Client{ // Create new client with unique ID for each connection
		ID:         uuid.New().String(), // Generate unique ID for each WebSocket connection
		UserID:     user.ID,
		Connection: conn,
		Send:       make(chan models.WebSocketMessage, 10),
	}

	// Check if hub is still running, restart if needed
	hub := services.GetHubInstance()
	select {
	case <-hub.Ctx.Done():
		log.Warn("websocket.controller.go: Hub context cancelled, attempting restart")
		if err := hub.Restart(); err != nil {
			log.WithError(err).Error("websocket.controller.go: Failed to restart hub")
			utils.ErrorResponse(ctx, http.StatusInternalServerError, "WebSocket service temporarily unavailable", nil)
			return
		}
	default:
		// Hub is running normally
	}

	hub.UpdateUserInfoCache(user.ID.Hex(), models.UserDisplayInfo{
		Email:       user.Email,
		Username:    user.Username,
		DisplayName: user.Profile.DisplayName,
	})

	services.GetHubInstance().Register <- client // Register client with hub

	// Update presence
	services.GetPresenceInstance().Set(services.UserPresence{
		UserID:     user.ID,
		Status:     services.UserStatus(user.Presence.Status),
		LastSeen:   user.Presence.LastSeen,
		ClientID:   client.ID,
		DeviceInfo: user.Presence.DeviceInfo,
	})

	log.WithFields(map[string]interface{}{
		string(objects.UserIDKey):   user.ID.Hex(),
		string(objects.UsernameKey): user.Username,
		string(objects.ClientIDKey): client.ID,
	}).Info("New WebSocket connection established")

	go func() {
		handleClientWrite(client) // Handle writing in a simple goroutine
	}()

	handleClientRead(reqCtx, client) // Handle reading in the main routine (blocking)
}

// handleClientWrite handles writing messages to the WebSocket connection
// message to be sent to the client
func handleClientWrite(client *models.Client) {
	ticker := time.NewTicker(54 * time.Second) // Ping every 54 seconds
	defer func() {
		ticker.Stop()
		log.Printf("Write goroutine closed for client: %s", client.ID)
	}()

	for {
		select {
		case message, ok := <-client.Send:
			client.Connection.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				client.Connection.WriteMessage(websocket.CloseMessage, []byte{}) // Send close message
				return
			}

			if err := client.Connection.WriteJSON(message); err != nil { // Send message as JSON
				log.Printf("Failed to write message to client %s: %v", client.ID, err)
				return
			}

			client.LastActivity = time.Now() // Update client activity
		case <-ticker.C:
			client.Connection.SetWriteDeadline(time.Now().Add(10 * time.Second))

			if err := client.Connection.WriteMessage(websocket.PingMessage, nil); err != nil { // Send ping
				log.Printf("Failed to send ping to client %s: %v", client.ID, err)
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
		services.GetHubInstance().Unregister <- client
		client.Connection.Close()
		log.Printf("Read goroutine closed for client: %s", client.ID)
	}()

	client.Connection.SetReadDeadline(time.Now().Add(60 * time.Second)) // Set read deadline and pong handler
	client.Connection.SetPongHandler(func(string) error {
		client.Connection.SetReadDeadline(time.Now().Add(60 * time.Second))
		client.LastActivity = time.Now()
		return nil
	})

	for {
		_, message, err := client.Connection.ReadMessage() // Read message from client
		if err != nil {
			// Any read error means the connection is no longer usable.
			// Never re-read a dead socket — gorilla panics on repeated read.
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for client %s: %v", client.ID, err)
			} else {
				log.Printf("WebSocket read failed for client %s: %v", client.ID, err)
			}
			break
		}

		var request models.MessageRequest // Parse the message into a MessageRequest
		if err := json.Unmarshal(message, &request); err != nil {
			log.Printf("Failed to unmarshal message from client %s: %v", client.ID, err)
			errorMsg := models.WebSocketMessage{
				Type: models.WSMessageTypeError,
				Data: models.ErrorMessage{
					Code:    "PARSE_ERROR",
					Message: "Failed to parse message: " + err.Error(),
				},
				Timestamp: time.Now(),
			}
			select {
			case client.Send <- errorMsg:
			default:
				log.Printf("Cannot send error message to client %s, closing connection", client.ID)
			}
			continue // SAFE: connection is still valid, just skip this bad message
		}

		client.LastActivity = time.Now() // Update client activity

		handleClientRequest(ctx, client, request) // Process the request
	}
}

// handleClientRequest processes different types of client requests
func handleClientRequest(ctx context.Context, client *models.Client, request models.MessageRequest) {
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
		handleMarkRead(client, request)

	case models.WSRequestTypeEditMessage:
		handleEditMessage(client, request)

	case models.WSRequestTypeDeleteMessage:
		handleDeleteMessage(client, request)

	case models.WSRequestTypeAddReaction:
		handleAddReaction(client, request)

	case models.WSRequestTypeRemoveReaction:
		handleRemoveReaction(client, request)

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

	canSend, err := validateMessagePermissions(ctx, client.UserID, chatID) // Check if user has permission to send messages in this chat
	if err != nil {
		log.Printf("Failed to validate message permissions: %v", err)
		sendErrorResponse(client, request.RequestID, "PERMISSION_CHECK_FAILED", "Failed to validate permissions")
		return
	}

	if !canSend {
		sendErrorResponse(client, request.RequestID, "PERMISSION_DENIED", "You don't have permission to send messages in this chat")
		return
	}

	if err := validateMessageContent(request); err != nil { // Validate message content and type
		sendErrorResponse(client, request.RequestID, "INVALID_CONTENT", err.Error())
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
		log.Printf("Failed to send message: %v", err)
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
		Timestamp: time.Now(),
	}

	select {
	case client.Send <- wsMessage:
	default:
		log.Printf("Client %s send channel is full, dropping message", client.ID)
		errorResponse := models.WebSocketMessage{ // Send error response about dropped message
			Type: models.WSMessageTypeError,
			Data: models.ErrorMessage{
				Code:    "CHANNEL_FULL",
				Message: "Message dropped due to full send channel",
			},
			RequestID: request.RequestID,
			Timestamp: time.Now(),
		}
		select { // Try to send error, if that fails too, the client is probably unresponsive
		case client.Send <- errorResponse:
		default:
			log.Printf("Client %s is unresponsive, marking for cleanup", client.ID)
		}
	}

	log.Printf("Message sent by client %s in chat %s", client.ID, request.ChatID)
}

// handleJoinChat processes chat joining requests
func handleJoinChat(ctx context.Context, client *models.Client, request models.MessageRequest) {
	log := logger.WithContext(ctx)
	userInterface := ctx.Value(objects.UserDataKey) // Get user from context

	if userInterface == nil {
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
	if err != nil {
		log.WithError(err).Error("Failed to create chat")
		sendErrorResponse(client, request.RequestID, "CHAT_CREATION_FAILED", "Failed to create chat")
		return
	}

	chatID := chat.ChatID.Hex()

	services.GetHubInstance().JoinChat(ctx, client, chatID) // Join the chat room

	log.Printf("Client %s joined chat %s", client.ID, chatID)
}

// handleLeaveChat processes chat leaving requests
func handleLeaveChat(client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	services.GetHubInstance().LeaveChat(client, request.ChatID) // Leave the chat room

	log.Printf("Client %s left chat %s", client.ID, request.ChatID)
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
		log.Printf("Failed to update typing status: %v", err)
		sendErrorResponse(client, request.RequestID, "TYPING_FAILED", "Failed to update typing status")
		return
	}

	log.Printf("Client %s typing status: %v in chat %s", client.ID, isTyping, request.ChatID)
}

// handleMarkRead processes message read status updates
func handleMarkRead(client *models.Client, request models.MessageRequest) {
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

	err = services.MarkMessagesAsRead(chatID, client.UserID, messageIDs) // Mark messages as read
	if err != nil {
		log.Printf("Failed to mark messages as read: %v", err)
		sendErrorResponse(client, request.RequestID, "READ_FAILED", "Failed to mark messages as read")
		return
	}

	log.Printf("Client %s marked %d messages as read in chat %s", client.ID, len(messageIDs), request.ChatID)
}

// handleAddReaction processes message reaction addition requests
func handleAddReaction(client *models.Client, request models.MessageRequest) {
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

	err = services.AddMessageReaction(chatID, msgObjectID, client.UserID, emoji) // Add reaction through service layer
	if err != nil {
		log.Printf("Failed to add reaction: %v", err)
		errCode := "REACTION_FAILED"
		errMsg := "Failed to add reaction"
		if err.Error() == "message not found" {
			errCode = "MESSAGE_NOT_FOUND"
			errMsg = "Message not found"
		}
		sendErrorResponse(client, request.RequestID, errCode, errMsg)
		return
	}

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
		log.Printf("Failed to broadcast reaction: %v", err)
	}

	sendSuccessResponse(client, request.RequestID, map[string]interface{}{ // Send success response
		"action":    "reaction_added",
		"messageId": messageID,
		"emoji":     emoji,
	})

	log.Printf("Client %s added reaction %s to message %s", client.ID, emoji, messageID)
}

// handleRemoveReaction processes message reaction removal requests
func handleRemoveReaction(client *models.Client, request models.MessageRequest) {
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

	err = services.RemoveMessageReaction(chatID, msgObjectID, client.UserID, emoji) // Remove reaction through service layer
	if err != nil {
		log.Printf("Failed to remove reaction: %v", err)
		errCode := "REACTION_FAILED"
		errMsg := "Failed to remove reaction"
		if err.Error() == "message not found" {
			errCode = "MESSAGE_NOT_FOUND"
			errMsg = "Message not found"
		}
		sendErrorResponse(client, request.RequestID, errCode, errMsg)
		return
	}

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
		log.Printf("Failed to broadcast reaction removal: %v", err)
	}

	sendSuccessResponse(client, request.RequestID, map[string]interface{}{ // Send success response
		"action":    "reaction_removed",
		"messageId": messageID,
		"emoji":     emoji,
	})

	log.Printf("Client %s removed reaction %s from message %s", client.ID, emoji, messageID)
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

	// Verify chat exists and is a group where the user is a participant
	chat, err := services.GetChat(ctx, models.ChatInfo{ChatID: chatID})
	if err != nil {
		log.Printf("Failed to get chat for invite: %v", err)
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Chat not found")
		return
	}

	if chat.ChatType != string(objects.ChatTypeGroup) {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "Can only invite users to group chats")
		return
	}

	actingParticipant, isParticipant := chat.Participants[client.UserID]
	if !isParticipant {
		sendErrorResponse(client, request.RequestID, "PERMISSION_DENIED", "You are not a participant of this chat")
		return
	}

	// Only owners and admins can invite
	if actingParticipant.Role != string(objects.ChatRoleOwner) && actingParticipant.Role != string(objects.ChatRoleAdmin) {
		sendErrorResponse(client, request.RequestID, "PERMISSION_DENIED", "Only owners and admins can invite users")
		return
	}

	// Add members through service layer
	err = services.AddGroupMember(ctx, client.UserID, chatID, userIDs)
	if err != nil {
		log.Printf("Failed to add group members: %v", err)
		sendErrorResponse(client, request.RequestID, "INVITE_FAILED", "Failed to invite users: "+err.Error())
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
		log.Printf("Failed to broadcast invite event: %v", err)
	}

	sendSuccessResponse(client, request.RequestID, map[string]interface{}{
		"action":       "users_invited",
		"chatId":       request.ChatID,
		"invitedUsers": invitedIDStrings,
	})

	log.Printf("Client %s invited %d users to chat %s", client.ID, len(userIDs), request.ChatID)
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

	// Remove members through service layer (validates permissions, chat type, etc.)
	err = services.RemoveGroupMember(ctx, client.UserID, chatID, userIDs)
	if err != nil {
		log.Printf("Failed to remove group members: %v", err)

		errCode := "REMOVE_FAILED"
		clientMsg := "Failed to remove users"

		if errors.Is(err, services.ErrChatNotFound) {
			errCode = "INVALID_CHAT_ID"
			clientMsg = "Chat not found"
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

	// Build removed user ID strings
	removedIDStrings := make([]string, len(userIDs))
	for i, id := range userIDs {
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
		log.Printf("Failed to broadcast remove event: %v", err)
	}

	sendSuccessResponse(client, request.RequestID, map[string]interface{}{
		"action":       "users_removed",
		"chatId":       request.ChatID,
		"removedUsers": removedIDStrings,
	})

	log.Printf("Client %s removed %d users from chat %s", client.ID, len(userIDs), request.ChatID)
}

// handleUpdateChat processes chat update requests
func handleUpdateChat(client *models.Client, request models.MessageRequest) {
	// TODO: Implement chat updates
	sendErrorResponse(client, request.RequestID, "NOT_IMPLEMENTED", "Chat updates not yet implemented")
}

// ============ UTILITY FUNCTIONS ============

// validateMessagePermissions checks if a user can send messages in a chat
func validateMessagePermissions(ctx context.Context, userID bson.ObjectID, chatID bson.ObjectID) (bool, error) {
	chat, err := services.GetChat(ctx, models.ChatInfo{ChatID: chatID})
	if err != nil {
		return false, fmt.Errorf("failed to get chat: %w", err)
	}

	participant, isParticipant := chat.Participants[userID]
	if !isParticipant {
		return false, nil
	}

	if participant.IsBlocked || participant.IsMuted { // Check if user is blocked or muted

		return false, nil
	}

	switch chat.ChatType {
	case string(objects.ChatTypeDirect):
		return true, nil

	case string(objects.ChatTypeGroup):
		permissions := participant.Permissions
		if len(permissions) > 0 {
			for _, perm := range permissions {
				if perm == "send_message" || perm == "admin" || perm == "owner" {
					return true, nil
				}
			}
			return false, nil
		}
		return true, nil // Default: members can send

	case string(objects.ChatTypeChannel):
		return participant.Role == "admin" || participant.Role == "owner", nil

	default:
		return false, nil
	}
}

// validateMessageContent validates message content and attachments
func validateMessageContent(request models.MessageRequest) error {
	if len(request.Content) > 4000 { // 4KB limit
		return fmt.Errorf("message content too long (max 4000 characters)")
	}

	validTypes := map[string]bool{
		"text":  true,
		"image": true,
		"file":  true,
		"audio": true,
		"video": true,
	}

	if request.MessageType != "" && !validTypes[request.MessageType] {
		return fmt.Errorf("invalid message type: %s", request.MessageType)
	}

	if len(request.Attachments) > 10 { // Max 10 attachments
		return fmt.Errorf("too many attachments (max 10)")
	}

	for _, attachment := range request.Attachments {
		if attachment.Size > 50*1024*1024 { // 50MB limit
			return fmt.Errorf("attachment too large (max 50MB)")
		}
		if attachment.URL == "" || attachment.OriginalName == "" {
			return fmt.Errorf("invalid attachment data")
		}
	}

	if len(request.Mentions) > 20 { // Max 20 mentions
		return fmt.Errorf("too many mentions (max 20)")
	}

	return nil
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

	select {
	case client.Send <- wsMessage:
	default:
		log.Printf("Failed to send success response to client %s: channel full", client.ID)
	}
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

	select {
	case client.Send <- errorMsg:
	default:
		log.Printf("Failed to send error response to client %s: channel full", client.ID)
	}
}
