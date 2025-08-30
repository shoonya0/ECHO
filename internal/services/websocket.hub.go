package services

import (
	"context"
	"fmt"
	"gin/internal/models"
	"gin/logger"
	"gin/objects"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ WEBSOCKET HUB FOR MANAGING REAL-TIME CONNECTIONS ============

var (
	// Singleton hub instance
	hubInstance *models.Hub
	hubOnce     sync.Once
)

// GetHubInstance returns the singleton hub instance
func GetHubInstance() *models.Hub {
	hubOnce.Do(func() {
		hubInstance = &models.Hub{
			Clients:       make(map[string]*models.Client),
			ChatClients:   make(map[string]map[string]*models.Client),
			UserClients:   make(map[string]map[string]*models.Client),
			UserInfoCache: make(map[string]*models.UserDisplayInfo),
			CacheExpiry:   make(map[string]time.Time),
		}
		// Initialize user lookup service
		InitUserLookupService()
	})
	return hubInstance
}

// RunHub starts the WebSocket hub and handles all real-time operations (simplified)
func RunHub() {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.Info("WebSocket Hub started - Managing real-time connections")

	// Start cleanup routine for inactive connections
	go cleanupInactiveConnections()

	// Hub now operates without complex channel management
	// Individual operations are called directly from API endpoints
	log.Info("WebSocket Hub is ready to handle connections")
}

// ============ CLIENT MANAGEMENT ============

// registerClient registers a new client connection
func registerClient(ctx context.Context, client *models.Client) {
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"client_id": client.ID,
		"user_id":   client.UserID.Hex(),
	}).Debug("Registering new WebSocket client")

	hub := GetHubInstance()

	// Add client to hub
	hub.Clients[client.ID] = client

	// Add client to user mapping
	userID := client.UserID.Hex()
	if hub.UserClients[userID] == nil {
		hub.UserClients[userID] = make(map[string]*models.Client)
	}
	hub.UserClients[userID][client.ID] = client

	// Update user presence to online
	if err := UpdateUserPresence(ctx, client.UserID, string(objects.UserStatusOnline)); err != nil {
		log.WithError(err).Error("Failed to update user presence")
	}

	// Send welcome message
	welcomeMsg := models.WebSocketMessage{
		Type:      models.WSMessageTypeResponse,
		Data:      map[string]interface{}{"status": "connected", "clientId": client.ID},
		Timestamp: time.Now(),
	}

	select {
	case client.Send <- welcomeMsg:
		log.Debug("Welcome message sent to client")
	default:
		log.Warn("Client send channel blocked, closing connection")
		close(client.Send)
		delete(hub.Clients, client.ID)
		return
	}

	// Get user display info for logging
	userInfo, err := GetUserDisplayInfo(client.UserID)
	if err != nil {
		log.WithError(err).Warn("Failed to get user display info")
		log.WithFields(map[string]interface{}{
			"client_id":     client.ID,
			"user_id":       client.UserID.Hex(),
			"total_clients": len(hub.Clients),
		}).Info("Client connected")
	} else {
		log.WithFields(map[string]interface{}{
			"client_id":     client.ID,
			"user_id":       client.UserID.Hex(),
			"username":      userInfo.Username,
			"total_clients": len(hub.Clients),
		}).Info("Client connected")
	}

	// Notify user's contacts about online status
	notifyUserPresence(client, string(objects.UserStatusOnline))
}

// unregisterClient removes a client connection
func unregisterClient(client *models.Client) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"client_id": client.ID,
		"user_id":   client.UserID.Hex(),
	}).Debug("Unregistering WebSocket client")

	hub := GetHubInstance()

	if _, ok := hub.Clients[client.ID]; ok {
		// Remove from all chats
		for _, chatID := range client.ActiveChats {
			log.WithField("chat_id", chatID).Debug("Removing client from chat")
			removeClientFromChat(client, chatID)
		}

		// Remove from hub
		delete(hub.Clients, client.ID)

		// Remove from user mapping
		userID := client.UserID.Hex()
		if userClients, exists := hub.UserClients[userID]; exists {
			delete(userClients, client.ID)

			// If no more clients for this user, update presence to offline
			if len(userClients) == 0 {
				log.WithField("user_id", userID).Debug("User has no more active clients, marking as offline")
				delete(hub.UserClients, userID)
				if err := UpdateUserPresence(ctx, client.UserID, string(objects.UserStatusOffline)); err != nil {
					log.WithError(err).Error("Failed to update user presence to offline")
				}
				notifyUserPresence(client, string(objects.UserStatusOffline))
			}
		}

		// Close send channel
		close(client.Send)

		// Get user display info for logging
		userInfo, err := GetUserDisplayInfo(client.UserID)
		if err != nil {
			log.WithError(err).Warn("Failed to get user display info")
			log.WithFields(map[string]interface{}{
				"client_id":     client.ID,
				"user_id":       client.UserID.Hex(),
				"total_clients": len(hub.Clients),
			}).Info("Client disconnected")
		} else {
			log.WithFields(map[string]interface{}{
				"client_id":     client.ID,
				"user_id":       client.UserID.Hex(),
				"username":      userInfo.Username,
				"total_clients": len(hub.Clients),
			}).Info("Client disconnected")
		}
	} else {
		log.WithField("client_id", client.ID).Warn("Attempted to unregister non-existent client")
	}
}

// ============ CHAT MANAGEMENT ============

// handleJoinChat adds a client to a chat
func handleJoinChat(ctx context.Context, req models.JoinChatRequest) {
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"client_id": req.Client.ID,
		"user_id":   req.Client.UserID.Hex(),
		"chat_id":   req.ChatID,
	}).Debug("Processing chat join request")

	hub := GetHubInstance()

	// Create chat clients map if it doesn't exist
	if _, exists := hub.ChatClients[req.ChatID]; !exists {
		log.WithField("chat_id", req.ChatID).Debug("Creating new chat clients map")
		hub.ChatClients[req.ChatID] = make(map[string]*models.Client)
	}

	// Check if client can join (permissions, etc.)
	if !canJoinChat(req.Client, req.ChatID) {
		log.WithFields(map[string]interface{}{
			"client_id": req.Client.ID,
			"chat_id":   req.ChatID,
		}).Warn("Client denied permission to join chat")

		errorMsg := models.WebSocketMessage{
			Type: models.WSMessageTypeError,
			Data: models.ErrorMessage{
				Code:    "JOIN_DENIED",
				Message: "Permission denied to join chat",
			},
			Timestamp: time.Now(),
		}
		req.Client.Send <- errorMsg
		return
	}

	// Add client to chat
	hub.ChatClients[req.ChatID][req.Client.ID] = req.Client

	// Add chat to client's active chats
	alreadyInChat := false
	for _, chatID := range req.Client.ActiveChats {
		if chatID == req.ChatID {
			log.Debug("Client already in chat, skipping active chats update")
			alreadyInChat = true
			break
		}
	}
	if !alreadyInChat {
		req.Client.ActiveChats = append(req.Client.ActiveChats, req.ChatID)
	}

	// Get user display info for notification
	userInfo, err := GetUserDisplayInfo(req.Client.UserID)
	username := req.Client.UserID.Hex()
	if err != nil {
		log.WithError(err).Warn("Failed to get user display info for join notification")
	} else {
		username = userInfo.Username
	}

	// Notify other clients in the chat
	joinMsg := models.WebSocketMessage{
		Type:   models.WSMessageTypeJoin,
		ChatID: req.ChatID,
		UserID: req.Client.UserID.Hex(),
		Data: models.UserJoinLeave{
			UserID:    req.Client.UserID.Hex(),
			Username:  username,
			Action:    "join",
			Timestamp: time.Now(),
		},
		Timestamp: time.Now(),
	}

	broadcastToChat(req.ChatID, joinMsg, map[string]bool{req.Client.ID: true})

	// Get chat info for comprehensive response
	chatInfo, err := getChatInfoFromDB(req.ChatID)
	if err != nil {
		log.WithError(err).Error("Failed to get chat info from database")
	}

	responseData := map[string]interface{}{
		"action":  "joined",
		"chatId":  req.ChatID,
		"members": len(hub.ChatClients[req.ChatID]),
	}

	// Add chat information if available
	if chatInfo != nil {
		responseData["chatName"] = chatInfo.Name
		responseData["chatType"] = chatInfo.ChatType
		responseData["participantCount"] = chatInfo.Stats.ParticipantCount

		// Add participant list for group chats (with user lookup)
		if chatInfo.ChatType == "group" {
			participants := make([]map[string]interface{}, 0)
			for _, participant := range chatInfo.Participants {
				userInfo, err := GetUserDisplayInfo(participant.UserInfo.UserID)
				if err != nil {
					log.WithError(err).WithField("participant_id", participant.UserInfo.UserID.Hex()).
						Warn("Failed to get participant info")
					continue // Skip users we can't fetch info for
				}
				participants = append(participants, map[string]interface{}{
					"userId":      participant.UserInfo.UserID.Hex(),
					"username":    userInfo.Username,
					"displayName": userInfo.DisplayName,
					"avatar":      userInfo.Avatar,
					"role":        participant.Role,
					"isOnline":    IsUserOnline(participant.UserInfo.UserID.Hex()),
				})
			}
			responseData["participants"] = participants
			log.WithField("participant_count", len(participants)).Debug("Added group chat participants to response")
		}

		// For direct chats, add the other participant's info
		if chatInfo.ChatType == string(objects.ChatTypeDirect) {
			for userID, participant := range chatInfo.Participants {
				if userID != req.Client.UserID {
					userInfo, err := GetUserDisplayInfo(participant.UserInfo.UserID)
					if err != nil {
						log.WithError(err).WithField("other_user_id", participant.UserInfo.UserID.Hex()).
							Warn("Failed to get other user info for direct chat")
					} else {
						responseData["otherUser"] = map[string]interface{}{
							"userId":      participant.UserInfo.UserID.Hex(),
							"username":    userInfo.Username,
							"displayName": userInfo.DisplayName,
							"avatar":      userInfo.Avatar,
							"isOnline":    IsUserOnline(participant.UserInfo.UserID.Hex()),
						}
						log.Debug("Added other user info to direct chat response")
					}
					break
				}
			}
		}
	}

	// Send success response to joining client
	successMsg := models.WebSocketMessage{
		Type:      models.WSMessageTypeResponse,
		ChatID:    req.ChatID,
		Data:      responseData,
		Timestamp: time.Now(),
	}
	req.Client.Send <- successMsg

	log.WithFields(map[string]interface{}{
		"client_id":      req.Client.ID,
		"chat_id":        req.ChatID,
		"active_clients": len(hub.ChatClients[req.ChatID]),
		"chat_type":      chatInfo.ChatType,
	}).Info("Client joined chat successfully")
}

// handleLeaveChat removes a client from a chat
func handleLeaveChat(req models.LeaveChatRequest) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"client_id": req.Client.ID,
		"user_id":   req.Client.UserID.Hex(),
		"chat_id":   req.ChatID,
	}).Debug("Processing chat leave request")

	hub := GetHubInstance()

	if chatClients, exists := hub.ChatClients[req.ChatID]; exists {
		removeClientFromChat(req.Client, req.ChatID)

		// Get user display info for notification
		userInfo, err := GetUserDisplayInfo(req.Client.UserID)
		username := req.Client.UserID.Hex()
		if err != nil {
			log.WithError(err).Warn("Failed to get user display info for leave notification")
		} else {
			username = userInfo.Username
		}

		// Notify other clients in the chat
		leaveMsg := models.WebSocketMessage{
			Type:   models.WSMessageTypeLeave,
			ChatID: req.ChatID,
			UserID: req.Client.UserID.Hex(),
			Data: models.UserJoinLeave{
				UserID:    req.Client.UserID.Hex(),
				Username:  username,
				Action:    "leave",
				Timestamp: time.Now(),
			},
			Timestamp: time.Now(),
		}

		broadcastToChat(req.ChatID, leaveMsg, map[string]bool{req.Client.ID: true})

		log.WithFields(map[string]interface{}{
			"client_id":      req.Client.ID,
			"chat_id":        req.ChatID,
			"active_clients": len(chatClients) - 1, // -1 because client already removed
			"username":       username,
		}).Info("Client left chat successfully")
	} else {
		log.WithFields(map[string]interface{}{
			"client_id": req.Client.ID,
			"chat_id":   req.ChatID,
		}).Warn("Attempted to leave non-existent chat")
	}
}

// removeClientFromChat removes a client from a specific chat
func removeClientFromChat(client *models.Client, chatID string) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"client_id": client.ID,
		"user_id":   client.UserID.Hex(),
		"chat_id":   chatID,
	}).Debug("Removing client from chat")

	hub := GetHubInstance()
	hub.Mutex.Lock()
	defer hub.Mutex.Unlock()

	if chatClients, exists := hub.ChatClients[chatID]; exists {
		delete(chatClients, client.ID)
		log.Debug("Removed client from chat clients map")

		// Remove chat from client's active chats
		for i, activeChat := range client.ActiveChats {
			if activeChat == chatID {
				client.ActiveChats = append(client.ActiveChats[:i], client.ActiveChats[i+1:]...)
				log.Debug("Removed chat from client's active chats")
				break
			}
		}

		// Remove empty chat clients map after a delay
		if len(chatClients) == 0 {
			log.WithField("chat_id", chatID).Info("Chat is now empty, scheduling cleanup")
			go scheduleChatCleanup(chatID)
		} else {
			log.WithFields(map[string]interface{}{
				"chat_id":        chatID,
				"active_clients": len(chatClients),
			}).Debug("Updated chat clients count")
		}
	} else {
		log.WithField("chat_id", chatID).Warn("Attempted to remove client from non-existent chat")
	}
}

// ============ MESSAGE BROADCASTING ============

// broadcastMessage broadcasts a message to appropriate clients
func broadcastMessage(hubMsg models.HubMessage) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"message_type": hubMsg.Message.Type,
		"chat_id":      hubMsg.ChatID,
		"exclude":      len(hubMsg.Exclude),
	}).Debug("Broadcasting message")

	if hubMsg.ChatID != "" {
		// Broadcast to specific chat
		broadcastToChat(hubMsg.ChatID, hubMsg.Message, hubMsg.Exclude)
	} else {
		// Global broadcast (rare case)
		log.Info("Performing global broadcast")
		broadcastToAll(hubMsg.Message, hubMsg.Exclude)
	}
}

// broadcastToChat broadcasts a message to all clients in a chat
func broadcastToChat(chatID string, message models.WebSocketMessage, exclude map[string]bool) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"chat_id":      chatID,
		"message_type": message.Type,
		"exclude":      len(exclude),
	}).Debug("Broadcasting message to chat")

	hub := GetHubInstance()
	hub.Mutex.RLock()
	defer hub.Mutex.RUnlock()

	chatClients, exists := hub.ChatClients[chatID]
	if !exists {
		log.WithField("chat_id", chatID).Warn("Attempted to broadcast to non-existent chat")
		return
	}

	successCount := 0
	failureCount := 0

	for clientID, client := range chatClients {
		// Skip excluded clients
		if exclude != nil && exclude[clientID] {
			log.WithField("client_id", clientID).Debug("Skipping excluded client")
			continue
		}

		select {
		case client.Send <- message:
			successCount++
		default:
			// Client's send channel is full or closed, remove client
			log.WithFields(map[string]interface{}{
				"client_id": clientID,
				"chat_id":   chatID,
			}).Warn("Client unresponsive, removing from chat")
			unregisterClient(client)
			failureCount++
		}
	}

	log.WithFields(map[string]interface{}{
		"chat_id":       chatID,
		"total_clients": len(chatClients),
		"success_count": successCount,
		"failure_count": failureCount,
		"excluded":      len(exclude),
	}).Info("Message broadcast completed")
}

// broadcastToAll broadcasts a message to all connected clients
func broadcastToAll(message models.WebSocketMessage, exclude map[string]bool) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"message_type": message.Type,
		"exclude":      len(exclude),
	}).Debug("Broadcasting message to all clients")

	hub := GetHubInstance()
	successCount := 0
	failureCount := 0

	for clientID, client := range hub.Clients {
		if exclude != nil && exclude[clientID] {
			log.WithField("client_id", clientID).Debug("Skipping excluded client")
			continue
		}

		select {
		case client.Send <- message:
			successCount++
		default:
			log.WithField("client_id", clientID).Warn("Client unresponsive, removing from hub")
			unregisterClient(client)
			failureCount++
		}
	}

	log.WithFields(map[string]interface{}{
		"total_clients": len(hub.Clients),
		"success_count": successCount,
		"failure_count": failureCount,
		"excluded":      len(exclude),
	}).Info("Global broadcast completed")
}

// ============ UTILITY FUNCTIONS ============

// Chat clients are now managed directly in ChatClients map
// No separate room creation needed since Chat model handles both persistence and real-time

// getChatInfoFromDB retrieves chat information from database
func getChatInfoFromDB(chatID string) (*models.Chat, error) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithField("chat_id", chatID).Debug("Retrieving chat info from database")

	chatObjectID, err := bson.ObjectIDFromHex(chatID)
	if err != nil {
		log.WithError(err).Error("Invalid chat ID format")
		return nil, fmt.Errorf("invalid chat ID: %w", err)
	}

	chat, err := GetChat(ctx, models.ChatInfo{ChatID: chatObjectID})
	if err != nil {
		log.WithError(err).Warn("Failed to get chat from database")
		return nil, fmt.Errorf("failed to get chat: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"chat_name":         chat.Name,
		"chat_type":         chat.ChatType,
		"participant_count": chat.Stats.ParticipantCount,
	}).Debug("Chat info retrieved successfully")

	return &chat, nil
}

// canJoinChat checks if a client can join a specific chat with proper permissions
func canJoinChat(client *models.Client, chatID string) bool {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"client_id": client.ID,
		"user_id":   client.UserID.Hex(),
		"chat_id":   chatID,
	}).Debug("Checking chat join permissions")

	// Get chat info to check if user is a participant
	chatInfo, err := getChatInfoFromDB(chatID)
	if err != nil {
		log.WithError(err).Error("Failed to get chat info for permission check")
		return false
	}

	// Check if user is a participant in the chat
	userID := client.UserID.Hex()
	participant, isParticipant := chatInfo.Participants[client.UserID]

	if !isParticipant {
		log.WithFields(map[string]interface{}{
			"user_id": userID,
			"chat_id": chatID,
		}).Warn("User is not a participant in chat")
		return false
	}

	// Check if user is blocked
	if participant.IsBlocked {
		log.WithFields(map[string]interface{}{
			"user_id": userID,
			"chat_id": chatID,
		}).Warn("User is blocked from chat")
		return false
	}

	// For private chats, additional checks
	if chatInfo.Settings.IsPrivate {
		// User must be explicitly invited (already checked by being a participant)
		log.WithFields(map[string]interface{}{
			"user_id": userID,
			"chat_id": chatID,
		}).Info("User joining private chat")
	}

	log.WithFields(map[string]interface{}{
		"user_id": userID,
		"chat_id": chatID,
		"role":    participant.Role,
	}).Debug("User has permission to join chat")
	return true
}

// scheduleChatCleanup schedules cleanup of empty rooms (simplified)
func scheduleChatCleanup(chatID string) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithField("chat_id", chatID).Debug("Scheduling chat cleanup")

	// Immediate cleanup instead of delayed
	hub := GetHubInstance()
	hub.Mutex.Lock()
	defer hub.Mutex.Unlock()

	if chatClients, exists := hub.ChatClients[chatID]; exists {
		if len(chatClients) == 0 {
			delete(hub.ChatClients, chatID)
			log.WithField("chat_id", chatID).Info("Empty chat cleaned up")
		} else {
			log.WithFields(map[string]interface{}{
				"chat_id":        chatID,
				"active_clients": len(chatClients),
			}).Debug("Chat still has active clients, skipping cleanup")
		}
	} else {
		log.WithField("chat_id", chatID).Warn("Attempted to clean up non-existent chat")
	}
}

// cleanupInactiveConnections periodically removes inactive connections
func cleanupInactiveConnections() {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.Info("Starting inactive connections cleanup routine")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		hub := GetHubInstance()
		now := time.Now()
		inactiveClients := make([]*models.Client, 0)
		inactivityThreshold := 5 * time.Minute

		log.WithFields(map[string]interface{}{
			"total_clients":        len(hub.Clients),
			"inactivity_threshold": inactivityThreshold.String(),
		}).Debug("Checking for inactive clients")

		for _, client := range hub.Clients {
			inactiveDuration := now.Sub(client.LastActivity)
			if inactiveDuration > inactivityThreshold {
				log.WithFields(map[string]interface{}{
					"client_id":         client.ID,
					"user_id":           client.UserID.Hex(),
					"inactive_duration": inactiveDuration.String(),
				}).Debug("Found inactive client")
				inactiveClients = append(inactiveClients, client)
			}
		}

		if len(inactiveClients) > 0 {
			log.WithField("inactive_count", len(inactiveClients)).Info("Removing inactive clients")
			for _, client := range inactiveClients {
				log.WithFields(map[string]interface{}{
					"client_id": client.ID,
					"user_id":   client.UserID.Hex(),
				}).Info("Removing inactive client")
				unregisterClient(client)
			}
		} else {
			log.Debug("No inactive clients found")
		}
	}
}

// notifyUserPresence notifies user's contacts about presence changes
func notifyUserPresence(client *models.Client, status string) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"client_id": client.ID,
		"user_id":   client.UserID.Hex(),
		"status":    status,
	}).Debug("Notifying contacts about user presence change")

	// TODO: Get user's contacts and notify them about presence change
	// This would involve querying the contacts collection and sending presence updates

	now := time.Now()
	presenceMsg := models.WebSocketMessage{
		Type:   models.WSMessageTypePresence,
		UserID: client.UserID.Hex(),
		Data: models.WSPresenceStatus{
			UserID:   client.UserID.Hex(),
			Status:   status,
			LastSeen: now,
		},
		Timestamp: now,
	}

	// For now, we'll broadcast to all clients
	// In a production system, this should be optimized to only notify contacts
	log.Info("Broadcasting presence update to all clients (TODO: optimize to only notify contacts)")
	broadcastToAll(presenceMsg, map[string]bool{client.ID: true})
}

// ============ PUBLIC API FUNCTIONS ============

// CreateClient creates a new WebSocket client
func CreateClient(ctx context.Context, userID bson.ObjectID, conn *websocket.Conn) *models.Client {
	log := logger.WithContext(ctx)

	clientID := uuid.New().String()
	log.WithFields(map[string]interface{}{
		"client_id": clientID,
		"user_id":   userID.Hex(),
	}).Info("Creating new WebSocket client")

	client := &models.Client{
		ID:           clientID,
		UserID:       userID,
		Connection:   conn,
		Send:         make(chan models.WebSocketMessage, 256),
		ActiveChats:  make([]string, 0),
		LastActivity: time.Now(),
		Metadata:     make(map[string]interface{}),
	}

	log.WithFields(map[string]interface{}{
		"client_id":   client.ID,
		"user_id":     client.UserID.Hex(),
		"buffer_size": 256,
		"created_at":  client.LastActivity,
	}).Debug("WebSocket client created successfully")

	return client
}

// RegisterClient registers a client with the hub (direct call)
func RegisterClient(ctx context.Context, client *models.Client) {
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"client_id": client.ID,
		"user_id":   client.UserID.Hex(),
	}).Info("Registering client with hub")
	registerClient(ctx, client)
}

// UnregisterClient unregisters a client from the hub (direct call)
func UnregisterClient(client *models.Client) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"client_id": client.ID,
		"user_id":   client.UserID.Hex(),
	}).Info("Unregistering client from hub")
	unregisterClient(client)
}

// BroadcastToChat broadcasts a message to a specific chat (direct call)
func BroadcastToChat(chatID string, message models.WebSocketMessage) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"chat_id":      chatID,
		"message_type": message.Type,
	}).Debug("Broadcasting message to chat")
	broadcastToChat(chatID, message, nil)
}

// JoinChatRoom adds a client to a chat (direct call)
func JoinChatRoom(ctx context.Context, client *models.Client, chatID string) {
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"client_id": client.ID,
		"user_id":   client.UserID.Hex(),
		"chat_id":   chatID,
	}).Info("Adding client to chat room")

	joinReq := models.JoinChatRequest{
		Client: client,
		ChatID: chatID,
	}
	handleJoinChat(ctx, joinReq)
}

// LeaveChatRoom removes a client from a chat (direct call)
func LeaveChatRoom(client *models.Client, chatID string) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"client_id": client.ID,
		"user_id":   client.UserID.Hex(),
		"chat_id":   chatID,
	}).Info("Removing client from chat room")

	leaveReq := models.LeaveChatRequest{
		Client: client,
		ChatID: chatID,
	}
	handleLeaveChat(leaveReq)
}

// GetChatClients returns clients connected to a chat
func GetChatClients(chatID string) (map[string]*models.Client, bool) {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithField("chat_id", chatID).Debug("Getting chat clients")

	hub := GetHubInstance()
	hub.Mutex.RLock()
	defer hub.Mutex.RUnlock()
	clients, exists := hub.ChatClients[chatID]

	log.WithFields(map[string]interface{}{
		"chat_id":      chatID,
		"exists":       exists,
		"client_count": len(clients),
	}).Debug("Retrieved chat clients")

	return clients, exists
}

// GetUserClients returns all clients for a specific user
func GetUserClients(userID string) map[string]*models.Client {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithField("user_id", userID).Debug("Getting user clients")

	hub := GetHubInstance()
	if clients, exists := hub.UserClients[userID]; exists {
		log.WithFields(map[string]interface{}{
			"user_id":      userID,
			"client_count": len(clients),
		}).Debug("Retrieved user clients")
		return clients
	}

	log.WithField("user_id", userID).Debug("No clients found for user")
	return nil
}

// IsUserOnline checks if a user has any active connections
func IsUserOnline(userID string) bool {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.WithField("user_id", userID).Debug("Checking user online status")

	hub := GetHubInstance()
	clients, exists := hub.UserClients[userID]
	isOnline := exists && len(clients) > 0

	log.WithFields(map[string]interface{}{
		"user_id":      userID,
		"is_online":    isOnline,
		"client_count": len(clients),
	}).Debug("User online status checked")

	return isOnline
}

// GetHubStats returns current hub statistics
func GetHubStats() map[string]interface{} {
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.Debug("Getting hub statistics")

	hub := GetHubInstance()
	hub.Mutex.RLock()
	defer hub.Mutex.RUnlock()

	stats := map[string]interface{}{
		"totalClients": len(hub.Clients),
		"totalChats":   len(hub.ChatClients),
		"totalUsers":   len(hub.UserClients),
		"cacheSize":    len(hub.UserInfoCache),
		"timestamp":    time.Now(),
	}

	log.WithFields(map[string]interface{}{
		"total_clients": stats["totalClients"],
		"total_chats":   stats["totalChats"],
		"total_users":   stats["totalUsers"],
		"cache_size":    stats["cacheSize"],
	}).Info("Hub statistics retrieved")

	return stats
}
