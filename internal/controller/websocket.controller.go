package controller

import (
	"gin/internal/models"
	"gin/internal/services"
	"gin/internal/utils"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ WEBSOCKET CONTROLLER FOR REAL-TIME CHAT ============

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
	// Get user from JWT token (should be set by auth middleware)
	userInterface, exists := ctx.Get("user")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// Extract user information
	user, ok := userInterface.(models.User)
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "Invalid user data", nil)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade WebSocket connection: %v", err)
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Failed to upgrade WebSocket connection", nil)
		return
	}

	// Create new client
	client := services.CreateClient(user.ID, user.Username, conn)

	log.Printf("New WebSocket connection established for user: %s (%s)", user.Username, user.ID.Hex())

	// Register client with hub
	services.RegisterClient(client)

	// Start goroutines for reading and writing
	go handleClientWrite(client)
	go handleClientRead(client)
}

// handleClientWrite handles writing messages to the WebSocket connection
func handleClientWrite(client *models.Client) {
	ticker := time.NewTicker(54 * time.Second) // Ping every 54 seconds
	defer func() {
		ticker.Stop()
		client.Connection.Close()
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
		err := client.Connection.ReadJSON(&request)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for client %s: %v", client.ID, err)
			}
			break
		}

		// Update client activity
		client.LastActivity = time.Now()

		// Process the request
		handleClientRequest(client, request)
	}
}

// handleClientRequest processes different types of client requests
func handleClientRequest(client *models.Client, request models.MessageRequest) {
	switch request.Type {
	case models.WSRequestTypeSendMessage:
		handleSendMessage(client, request)

	case models.WSRequestTypeJoinRoom:
		handleJoinRoom(client, request)

	case models.WSRequestTypeLeaveRoom:
		handleLeaveRoom(client, request)

	case models.WSRequestTypeSetTyping:
		handleSetTyping(client, request)

	case models.WSRequestTypeMarkRead:
		handleMarkRead(client, request)

	default:
		sendErrorResponse(client, request.RequestID, "UNKNOWN_REQUEST", "Unknown request type: "+request.Type)
	}
}

// ============ REQUEST HANDLERS ============

// handleSendMessage processes message sending requests
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
			"messageId": message.ID.Hex(),
			"timestamp": message.CreatedAt,
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
		log.Printf("Client %s send channel is full", client.ID)
	}
}

// handleJoinRoom processes room joining requests
func handleJoinRoom(client *models.Client, request models.MessageRequest) {
	if request.ChatID == "" {
		sendErrorResponse(client, request.RequestID, "INVALID_REQUEST", "ChatID is required")
		return
	}

	// Join the chat room
	services.JoinChatRoom(client, request.ChatID)

	log.Printf("Client %s requested to join room %s", client.ID, request.ChatID)
}

// handleLeaveRoom processes room leaving requests
func handleLeaveRoom(client *models.Client, request models.MessageRequest) {
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

// ============ UTILITY FUNCTIONS ============

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
