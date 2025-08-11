package controller

import (
	"fmt"
	"gin/internal/models"
	"gin/internal/services"
	"gin/internal/utils"
	"gin/objects"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
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

// authenticateWebSocketToken verifies JWT token for WebSocket connections
func authenticateWebSocketToken(tokenString string) (models.User, error) {
	if !strings.HasPrefix(tokenString, "Bearer ") {
		return models.User{}, fmt.Errorf("authorization header must be in 'Bearer <token>' format")
	}

	// Extract token from header
	token := strings.TrimPrefix(tokenString, "Bearer ")
	if token == "" {
		return models.User{}, fmt.Errorf("token cannot be empty")
	}
	claims := JwtClaims{}

	// Parse and validate the token
	parsedToken, err := jwt.ParseWithClaims(token, &claims, func(token *jwt.Token) (interface{}, error) {
		// Verify the signing method
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(objects.MainConfiguration.JwtSecret), nil
	})

	if err != nil {
		return models.User{}, fmt.Errorf("token parsing failed: %w", err)
	}

	// Check if token is valid
	if !parsedToken.Valid {
		return models.User{}, fmt.Errorf("invalid token")
	}

	// Validate required claims
	if claims.UserID == "" {
		return models.User{}, fmt.Errorf("missing user_id in token claims")
	}

	if claims.Email == "" {
		return models.User{}, fmt.Errorf("missing email in token claims")
	}

	// Convert user ID to ObjectID
	objectID, err := bson.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return models.User{}, fmt.Errorf("invalid user ID format: %w", err)
	}

	// Get user details from database
	user, err := services.GetUserByID(objectID)
	if err != nil {
		return models.User{}, fmt.Errorf("%w", err)
	}

	return *user, nil
}

// Helper function to get username from client
func getUsernameFromClient(client *models.Client) string {
	userInfo, err := services.GetUserDisplayInfo(client.UserID)
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
		// Allow connections from any origin (configure appropriately for production)
		return true
	},
}

// HandleWebSocketChat handles WebSocket connections for chat functionality
func HandleWebSocketChat(ctx *gin.Context) {
	// For WebSocket connections, authenticate via query parameter
	token := ctx.GetHeader("Authorization")
	if token == "" {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "Token parameter required", nil)
		return
	}

	// Verify the token using our auth logic
	user, err := authenticateWebSocketToken(token)
	if err != nil {
		log.Printf("WebSocket authentication failed: %v", err)
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "Authentication failed", err.Error())
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket connection: %v", err)
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Failed to upgrade WebSocket connection", nil)
		return
	}

	// Create new client (simplified - no username needed)
	client := services.CreateClient(user.ID, conn)

	log.Printf("New WebSocket connection established for user: %s (%s)", user.Username, user.ID.Hex())

	// Register client with hub
	services.RegisterClient(client)

	// Auto-join user to their active chats
	err = autoJoinUserChats(client)
	if err != nil {
		log.Printf("Failed to auto-join user to chats: %v", err)
	}

	// Start sequential handlers for reading and writing
	go func() {
		// Handle writing in a simple goroutine
		handleClientWrite(client)
	}()

	// // Handle reading in the main routine (blocking)
	handleClientRead(client)
}

// handleClientWrite handles writing messages to the WebSocket connection
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
				// Channel was closed
				client.Connection.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Send message as JSON
			// utils.PrintColored("response to the client")
			// utils.PrintColored(message)

			if err := client.Connection.WriteJSON(message); err != nil {
				log.Printf("Failed to write message to client %s: %v", client.ID, err)
				return
			}

			// Update client activity
			client.LastActivity = time.Now()

		case <-ticker.C:
			client.Connection.SetWriteDeadline(time.Now().Add(10 * time.Second))

			// Send ping
			if err := client.Connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Failed to send ping to client %s: %v", client.ID, err)
				return
			}
		}
	}
}

// handleClientRead handles reading messages from the WebSocket connection
func handleClientRead(client *models.Client) {
	defer func() {
		services.UnregisterClient(client)
		client.Connection.Close()
		log.Printf("Read goroutine closed for client: %s", client.ID)
	}()

	// Set read deadline and pong handler
	client.Connection.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.Connection.SetPongHandler(func(string) error {
		client.Connection.SetReadDeadline(time.Now().Add(60 * time.Second))
		client.LastActivity = time.Now()
		return nil
	})

	for {
		// Read message from client
		var request models.MessageRequest
		_, message, err := client.Connection.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for client %s: %v", client.ID, err)
			} else {
				// Log other errors (like JSON parsing errors) but don't immediately close
				log.Printf("Failed to parse message from client %s: %v", client.ID, err)

				// Send error response to client instead of closing connection
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
					// If can't send error, then close
					log.Printf("Cannot send error message to client %s, closing connection", client.ID)
					break
				}
				continue // Don't close, try to read next message
			}
			break
		}

		// Update client activity
		client.LastActivity = time.Now()

		// Log received request for debugging
		// utils.PrintColored("message from the client")
		// utils.PrintColored(string(message))
		contentPreview := request.Content
		if len(contentPreview) > 50 {
			contentPreview = contentPreview[:50] + "..."
		}
		log.Printf("Received WebSocket request from client %s: Type=%s, ChatID=%s, Content=%s",
			client.ID, request.Type, request.ChatID, contentPreview+" "+string(message))

		// Process the request
		handleClientRequest(client, request)
	}
}

// handleClientRequest processes different types of client requests
func handleClientRequest(client *models.Client, request models.MessageRequest) {
	switch request.Type {
	case models.WSRequestTypeSendMessage:
		handleSendMessage(client, request)

	case models.WSRequestTypeJoinChat:
		handleJoinChat(client, request)

	case models.WSRequestTypeLeaveChat:
		handleLeaveChat(client, request)

	case models.WSRequestTypeSetTyping:
		handleSetTyping(client, request)

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
		handleInviteUser(client, request)

	case models.WSRequestTypeRemoveUser:
		handleRemoveUser(client, request)

	case models.WSRequestTypeUpdateChat:
		handleUpdateChat(client, request)

	default:
		sendErrorResponse(client, request.RequestID, "UNKNOWN_REQUEST", "Unknown request type: "+request.Type)
	}
}

// ============ REQUEST HANDLERS ============

// handleSendMessage processes message sending requests with comprehensive validation
func handleSendMessage(client *models.Client, request models.MessageRequest) {
	// Validate request
	if request.ChatID == "" || request.Content == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID and content are required")
		return
	}

	// Parse chat ID
	chatID, err := bson.ObjectIDFromHex(request.ChatID)
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	// Check if user has permission to send messages in this chat
	canSend, err := validateMessagePermissions(client.UserID, chatID, request)
	if err != nil {
		log.Printf("Failed to validate message permissions: %v", err)
		sendErrorResponse(client, request.RequestID, "PERMISSION_CHECK_FAILED", "Failed to validate permissions")
		return
	}

	if !canSend {
		sendErrorResponse(client, request.RequestID, "PERMISSION_DENIED", "You don't have permission to send messages in this chat")
		return
	}

	// Validate message content and type
	if err := validateMessageContent(request); err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CONTENT", err.Error())
		return
	}

	// Set default message type if not provided
	messageType := request.MessageType
	if messageType == "" {
		messageType = "text"
	}

	// Send message through service layer
	message, err := services.SendMessage(
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

	// Send success response
	response := models.MessageResponse{
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
		// Send error response about dropped message
		errorResponse := models.WebSocketMessage{
			Type: models.WSMessageTypeError,
			Data: models.ErrorMessage{
				Code:    "CHANNEL_FULL",
				Message: "Message dropped due to full send channel",
			},
			RequestID: request.RequestID,
			Timestamp: time.Now(),
		}
		// Try to send error, if that fails too, the client is probably unresponsive
		select {
		case client.Send <- errorResponse:
		default:
			log.Printf("Client %s is unresponsive, marking for cleanup", client.ID)
		}
	}

	log.Printf("Message sent by client %s in chat %s", client.ID, request.ChatID)
}

// handleJoinRoom processes room joining requests
func handleJoinChat(client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	// Join the chat room
	services.JoinChatRoom(client, request.ChatID)

	log.Printf("Client %s requested to join room %s", client.ID, request.ChatID)
}

// handleLeaveRoom processes room leaving requests
func handleLeaveChat(client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	// Leave the chat room
	services.LeaveChatRoom(client, request.ChatID)

	log.Printf("Client %s requested to leave room %s", client.ID, request.ChatID)
}

// handleSetTyping processes typing indicator requests
func handleSetTyping(client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	// Parse chat ID
	chatID, err := bson.ObjectIDFromHex(request.ChatID)
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	// Extract typing status from metadata
	isTyping := false
	if request.Metadata != nil {
		if typing, ok := request.Metadata["isTyping"].(bool); ok {
			isTyping = typing
		}
	}

	// Update typing status
	err = services.UpdateTypingStatus(chatID, client.UserID, isTyping)
	if err != nil {
		log.Printf("Failed to update typing status: %v", err)
		sendErrorResponse(client, request.RequestID, "TYPING_FAILED", "Failed to update typing status")
		return
	}

	log.Printf("Client %s set typing status to %v in room %s", client.ID, isTyping, request.ChatID)
}

// handleMarkRead processes message read status updates
func handleMarkRead(client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	// Parse chat ID
	chatID, err := bson.ObjectIDFromHex(request.ChatID)
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	// Extract message IDs from metadata
	var messageIDs []bson.ObjectID
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

	// Mark messages as read
	err = services.MarkMessagesAsRead(chatID, client.UserID, messageIDs)
	if err != nil {
		log.Printf("Failed to mark messages as read: %v", err)
		sendErrorResponse(client, request.RequestID, "READ_FAILED", "Failed to mark messages as read")
		return
	}

	log.Printf("Client %s marked %d messages as read in room %s", client.ID, len(messageIDs), request.ChatID)
}

// handleAddReaction processes message reaction addition requests
func handleAddReaction(client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	// Extract messageID and emoji from metadata
	var messageID, emoji string
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

	// Parse IDs
	chatID, err := bson.ObjectIDFromHex(request.ChatID)
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	msgObjectID, err := bson.ObjectIDFromHex(messageID)
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_MESSAGE_ID", "Invalid message ID format")
		return
	}

	// Add reaction through service layer
	err = services.AddMessageReaction(chatID, msgObjectID, client.UserID, emoji)
	if err != nil {
		log.Printf("Failed to add reaction: %v", err)
		sendErrorResponse(client, request.RequestID, "REACTION_FAILED", "Failed to add reaction")
		return
	}

	// Broadcast reaction to chat
	reactionEvent := models.WebSocketMessage{
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

	services.BroadcastToChat(request.ChatID, reactionEvent)

	// Send success response
	sendSuccessResponse(client, request.RequestID, map[string]interface{}{
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

	// Extract messageID and emoji from metadata
	var messageID, emoji string
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

	// Parse IDs
	chatID, err := bson.ObjectIDFromHex(request.ChatID)
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_CHAT_ID", "Invalid chat ID format")
		return
	}

	msgObjectID, err := bson.ObjectIDFromHex(messageID)
	if err != nil {
		sendErrorResponse(client, request.RequestID, "INVALID_MESSAGE_ID", "Invalid message ID format")
		return
	}

	// Remove reaction through service layer
	err = services.RemoveMessageReaction(chatID, msgObjectID, client.UserID, emoji)
	if err != nil {
		log.Printf("Failed to remove reaction: %v", err)
		sendErrorResponse(client, request.RequestID, "REACTION_FAILED", "Failed to remove reaction")
		return
	}

	// Broadcast reaction removal to chat
	reactionEvent := models.WebSocketMessage{
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

	services.BroadcastToChat(request.ChatID, reactionEvent)

	// Send success response
	sendSuccessResponse(client, request.RequestID, map[string]interface{}{
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

// handleInviteUser processes user invitation requests
func handleInviteUser(client *models.Client, request models.MessageRequest) {
	// TODO: Implement user invitation
	sendErrorResponse(client, request.RequestID, "NOT_IMPLEMENTED", "User invitation not yet implemented")
}

// handleRemoveUser processes user removal requests
func handleRemoveUser(client *models.Client, request models.MessageRequest) {
	// TODO: Implement user removal
	sendErrorResponse(client, request.RequestID, "NOT_IMPLEMENTED", "User removal not yet implemented")
}

// handleUpdateChat processes chat update requests
func handleUpdateChat(client *models.Client, request models.MessageRequest) {
	// TODO: Implement chat updates
	sendErrorResponse(client, request.RequestID, "NOT_IMPLEMENTED", "Chat updates not yet implemented")
}

// ============ UTILITY FUNCTIONS ============

// autoJoinUserChats automatically joins the user to their active chats (sequential)
func autoJoinUserChats(client *models.Client) error {
	// Get user's chats
	userChats, err := services.GetUserChats(client.UserID, []string{"chatId", "status", "username", "displayName", "avatar", "isFavorite"})
	if err != nil {
		log.Printf("Failed to get user chats for auto-join: %v", err)
		return err
	}

	if len(userChats) == 0 {
		return fmt.Errorf("no chats found for auto-join for client %s", client.ID)
	}

	// Join each chat sequentially
	for _, chat := range userChats {
		chatID := chat.ChatID.Hex()

		// Check if user is still a participant and not blocked
		if chat.Status == string(objects.StatusAccepted) {
			services.JoinChatRoom(client, chatID)
			log.Printf("Auto-joined client %s to chat %s (%s)", client.ID, chatID, chat.Status)
		}
	}

	log.Printf("Auto-join completed for client %s (%d chats)", client.ID, len(userChats))
	return nil
}

// validateMessagePermissions checks if a user can send messages in a chat
func validateMessagePermissions(userID bson.ObjectID, chatID bson.ObjectID, request models.MessageRequest) (bool, error) {
	// Get chat details
	chat, err := services.GetChat(models.GetContactInfo{ChatID: chatID})
	if err != nil {
		return false, fmt.Errorf("failed to get chat: %w", err)
	}

	// Check if user is a participant
	participant, isParticipant := chat.Participants[userID.Hex()]
	if !isParticipant {
		return false, nil
	}

	// Check if user is blocked or muted
	if participant.IsBlocked || participant.IsMuted {
		return false, nil
	}

	// Check chat-specific permissions
	switch chat.Type {
	case "direct":
		// In direct chats, both participants can send messages
		return true, nil

	case "group":
		// Check if user has message permission (default: all members can send)
		permissions := participant.Permissions
		if len(permissions) > 0 {
			// If permissions are explicitly set, check for send_message permission
			hasPermission := false
			for _, perm := range permissions {
				if perm == "send_message" || perm == "admin" || perm == "owner" {
					hasPermission = true
					break
				}
			}
			return hasPermission, nil
		}
		// Default: members can send messages
		return true, nil

	case "channel":
		// Only admins and owners can send messages in channels by default
		return participant.Role == "admin" || participant.Role == "owner", nil

	default:
		return false, nil
	}
}

// validateMessageContent validates message content and attachments
func validateMessageContent(request models.MessageRequest) error {
	// Check content length
	if len(request.Content) > 4000 { // 4KB limit
		return fmt.Errorf("message content too long (max 4000 characters)")
	}

	// Validate message type
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

	// Validate attachments if present
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

	// Validate mentions
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

// ============ HTTP ENDPOINTS FOR CHAT MANAGEMENT ============

// CreateDirectChatHTTP creates a direct chat via HTTP endpoint
func CreateDirectChatHTTP(ctx *gin.Context) {
	// Get current user
	userInterface, exists := ctx.Get("user")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	user, ok := userInterface.(models.User)
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "Invalid user data", nil)
		return
	}

	// Parse request
	var request struct {
		UserID string `json:"userId" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid request data", err.Error())
		return
	}

	// Parse target user ID
	targetUserID, err := bson.ObjectIDFromHex(request.UserID)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid user ID format", nil)
		return
	}

	// Create direct chat
	chat, err := services.CreateDirectChat(user.ID, targetUserID)
	if err != nil {
		log.Printf("Failed to create direct chat: %v", err)
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to create chat", nil)
		return
	}

	utils.SuccessResponse(ctx, "Direct chat created successfully", chat)
}

// CreateGroupChatHTTP creates a group chat via HTTP endpoint
func CreateGroupChatHTTP(ctx *gin.Context) {
	// Get current user
	userInterface, exists := ctx.Get("user")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	user, ok := userInterface.(models.User)
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "Invalid user data", nil)
		return
	}

	// Parse request
	var request struct {
		Name         string   `json:"name" binding:"required"`
		Description  string   `json:"description"`
		Participants []string `json:"participants" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid request data", err.Error())
		return
	}

	// Parse participant IDs
	var participantIDs []bson.ObjectID
	for _, idStr := range request.Participants {
		if id, err := bson.ObjectIDFromHex(idStr); err == nil {
			participantIDs = append(participantIDs, id)
		}
	}

	if len(participantIDs) == 0 {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "At least one valid participant is required", nil)
		return
	}

	// Create group chat
	chat, err := services.CreateGroupChat(user.ID, request.Name, request.Description, participantIDs)
	if err != nil {
		log.Printf("Failed to create group chat: %v", err)
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to create group chat", nil)
		return
	}

	utils.SuccessResponse(ctx, "Group chat created successfully", chat)
}

// GetChatMessagesHTTP retrieves chat messages via HTTP endpoint
func GetChatMessagesHTTP(ctx *gin.Context) {
	// Get current user
	_, exists := ctx.Get("user")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// Parse query parameters
	chatIDStr := ctx.Query("chatId")
	if chatIDStr == "" {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Chat ID is required", nil)
		return
	}

	chatID, err := bson.ObjectIDFromHex(chatIDStr)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid chat ID format", nil)
		return
	}

	// Parse pagination parameters
	limit := 50 // Default limit
	offset := 0 // Default offset

	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsedLimit := utils.ParseInt(limitStr, 50); parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if parsedOffset := utils.ParseInt(offsetStr, 0); parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Get chat with messages
	chat, messages, err := services.GetChatWithMessages(chatID, limit, offset)
	if err != nil {
		log.Printf("Failed to get chat messages: %v", err)
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get chat messages", nil)
		return
	}

	// Prepare response
	response := map[string]interface{}{
		"chat":     chat,
		"messages": messages,
		"pagination": map[string]interface{}{
			"limit":  limit,
			"offset": offset,
			"count":  len(messages),
		},
	}

	utils.SuccessResponse(ctx, "Chat messages retrieved successfully", response)
}

// GetHubStatsHTTP returns WebSocket hub statistics (for monitoring)
func GetHubStatsHTTP(ctx *gin.Context) {
	stats := services.GetHubStats()
	utils.SuccessResponse(ctx, "Hub statistics retrieved successfully", stats)
}
