package services

import (
	"fmt"
	"gin/internal/models"
	"gin/objects"
	"log"
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
	log.Println("WebSocket Hub started - Managing real-time connections")

	// Start cleanup routine for inactive connections
	go cleanupInactiveConnections()

	// Hub now operates without complex channel management
	// Individual operations are called directly from API endpoints
}

// ============ CLIENT MANAGEMENT ============

// registerClient registers a new client connection
func registerClient(client *models.Client) {
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
	UpdateUserPresence(client.UserID, string(objects.UserStatusOnline))

	// Send welcome message
	welcomeMsg := models.WebSocketMessage{
		Type:      models.WSMessageTypeResponse,
		Data:      map[string]interface{}{"status": "connected", "clientId": client.ID},
		Timestamp: time.Now(),
	}

	select {
	case client.Send <- welcomeMsg:
	default:
		close(client.Send)
		delete(hub.Clients, client.ID)
	}

	// Get user display info for logging
	userInfo, err := GetUserDisplayInfo(client.UserID)
	if err != nil {
		log.Printf("Client %s (User: %s) connected. Total clients: %d",
			client.ID, client.UserID.Hex(), len(hub.Clients))
	} else {
		log.Printf("Client %s (User: %s) connected. Total clients: %d",
			client.ID, userInfo.Username, len(hub.Clients))
	}

	// Notify user's contacts about online status
	notifyUserPresence(client, string(objects.UserStatusOnline))
}

// unregisterClient removes a client connection
func unregisterClient(client *models.Client) {
	hub := GetHubInstance()

	if _, ok := hub.Clients[client.ID]; ok {
		// Remove from all chats
		for _, chatID := range client.ActiveChats {
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
				delete(hub.UserClients, userID)
				UpdateUserPresence(client.UserID, string(objects.UserStatusOffline))
				notifyUserPresence(client, string(objects.UserStatusOffline))
			}
		}

		// Close send channel
		close(client.Send)

		// Get user display info for logging
		userInfo, err := GetUserDisplayInfo(client.UserID)
		if err != nil {
			log.Printf("Client %s (User: %s) disconnected. Total clients: %d",
				client.ID, client.UserID.Hex(), len(hub.Clients))
		} else {
			log.Printf("Client %s (User: %s) disconnected. Total clients: %d",
				client.ID, userInfo.Username, len(hub.Clients))
		}
	}
}

// ============ CHAT MANAGEMENT ============

// handleJoinChat adds a client to a chat
func handleJoinChat(req models.JoinChatRequest) {
	hub := GetHubInstance()

	// Create chat clients map if it doesn't exist
	if _, exists := hub.ChatClients[req.ChatID]; !exists {
		hub.ChatClients[req.ChatID] = make(map[string]*models.Client)
	}

	// Check if client can join (permissions, etc.)
	if !canJoinChat(req.Client, req.ChatID) {
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
	for _, chatID := range req.Client.ActiveChats {
		if chatID == req.ChatID {
			// Already in chat, don't add again
			goto skipAdd
		}
	}
	req.Client.ActiveChats = append(req.Client.ActiveChats, req.ChatID)

skipAdd:

	// Get user display info for notification
	userInfo, err := GetUserDisplayInfo(req.Client.UserID)
	username := req.Client.UserID.Hex()
	if err == nil {
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
	responseData := map[string]interface{}{
		"action":  "joined",
		"chatId":  req.ChatID,
		"members": len(hub.ChatClients[req.ChatID]),
	}

	// Add chat information if available
	if err == nil {
		responseData["chatName"] = chatInfo.Name
		responseData["chatType"] = chatInfo.ChatType
		responseData["participantCount"] = chatInfo.Stats.ParticipantCount

		// Add participant list for group chats (with user lookup)
		if chatInfo.ChatType == "group" {
			participants := make([]map[string]interface{}, 0)
			for _, participant := range chatInfo.Participants {
				userInfo, err := GetUserDisplayInfo(participant.UserInfo.UserID)
				if err != nil {
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
		}

		// For direct chats, add the other participant's info
		if chatInfo.ChatType == string(objects.ChatTypeDirect) {
			for userID, participant := range chatInfo.Participants {
				if userID != req.Client.UserID {
					userInfo, err := GetUserDisplayInfo(participant.UserInfo.UserID)
					if err == nil {
						responseData["otherUser"] = map[string]interface{}{
							"userId":      participant.UserInfo.UserID.Hex(),
							"username":    userInfo.Username,
							"displayName": userInfo.DisplayName,
							"avatar":      userInfo.Avatar,
							"isOnline":    IsUserOnline(participant.UserInfo.UserID.Hex()),
						}
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

	log.Printf("Client %s joined chat %s. Active clients: %d",
		req.Client.ID, req.ChatID, len(hub.ChatClients[req.ChatID]))
}

// handleLeaveChat removes a client from a chat
func handleLeaveChat(req models.LeaveChatRequest) {
	hub := GetHubInstance()

	if chatClients, exists := hub.ChatClients[req.ChatID]; exists {
		removeClientFromChat(req.Client, req.ChatID)

		// Get user display info for notification
		userInfo, err := GetUserDisplayInfo(req.Client.UserID)
		username := req.Client.UserID.Hex()
		if err == nil {
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

		log.Printf("Client %s left chat %s. Chat clients: %d",
			req.Client.ID, req.ChatID, len(chatClients)-1) // -1 because client already removed
	}
}

// removeClientFromChat removes a client from a specific chat
func removeClientFromChat(client *models.Client, chatID string) {
	hub := GetHubInstance()
	hub.Mutex.Lock()
	defer hub.Mutex.Unlock()

	if chatClients, exists := hub.ChatClients[chatID]; exists {
		delete(chatClients, client.ID)

		// Remove chat from client's active chats
		for i, activeChat := range client.ActiveChats {
			if activeChat == chatID {
				client.ActiveChats = append(client.ActiveChats[:i], client.ActiveChats[i+1:]...)
				break
			}
		}

		// Remove empty chat clients map after a delay
		if len(chatClients) == 0 {
			go scheduleChatCleanup(chatID)
		}
	}
}

// ============ MESSAGE BROADCASTING ============

// broadcastMessage broadcasts a message to appropriate clients
func broadcastMessage(hubMsg models.HubMessage) {
	if hubMsg.ChatID != "" {
		// Broadcast to specific chat
		broadcastToChat(hubMsg.ChatID, hubMsg.Message, hubMsg.Exclude)
	} else {
		// Global broadcast (rare case)
		broadcastToAll(hubMsg.Message, hubMsg.Exclude)
	}
}

// broadcastToChat broadcasts a message to all clients in a chat
func broadcastToChat(chatID string, message models.WebSocketMessage, exclude map[string]bool) {
	hub := GetHubInstance()
	hub.Mutex.RLock()
	defer hub.Mutex.RUnlock()

	chatClients, exists := hub.ChatClients[chatID]
	if !exists {
		log.Printf("Attempted to broadcast to non-existent chat: %s", chatID)
		return
	}

	for clientID, client := range chatClients {
		// Skip excluded clients
		if exclude != nil && exclude[clientID] {
			continue
		}

		select {
		case client.Send <- message:
		default:
			// Client's send channel is full or closed, remove client
			unregisterClient(client)
			log.Printf("Removed unresponsive client %s from chat %s", clientID, chatID)
		}
	}

	log.Printf("Broadcasted message to chat %s (%d clients)", chatID, len(chatClients))
}

// broadcastToAll broadcasts a message to all connected clients
func broadcastToAll(message models.WebSocketMessage, exclude map[string]bool) {
	hub := GetHubInstance()

	for clientID, client := range hub.Clients {
		if exclude != nil && exclude[clientID] {
			continue
		}

		select {
		case client.Send <- message:
		default:
			unregisterClient(client)
			log.Printf("Removed unresponsive client %s from global broadcast", clientID)
		}
	}
}

// ============ UTILITY FUNCTIONS ============

// Chat clients are now managed directly in ChatClients map
// No separate room creation needed since Chat model handles both persistence and real-time

// getChatInfoFromDB retrieves chat information from database
func getChatInfoFromDB(chatID string) (*models.Chat, error) {
	chatObjectID, err := bson.ObjectIDFromHex(chatID)
	if err != nil {
		return nil, fmt.Errorf("invalid chat ID: %w", err)
	}

	chat, err := GetChat(models.ContactInfo{ChatID: chatObjectID})
	if err != nil {
		return nil, fmt.Errorf("failed to get chat: %w", err)
	}

	return &chat, nil
}

// canJoinChat checks if a client can join a specific chat with proper permissions
func canJoinChat(client *models.Client, chatID string) bool {
	// Get chat info to check if user is a participant
	chatInfo, err := getChatInfoFromDB(chatID)
	if err != nil {
		log.Printf("Failed to get chat info for permission check: %v", err)
		return false
	}

	// Check if user is a participant in the chat
	userID := client.UserID.Hex()
	participant, isParticipant := chatInfo.Participants[client.UserID]

	if !isParticipant {
		log.Printf("User %s is not a participant in chat %s", userID, chatID)
		return false
	}

	// Check if user is blocked
	if participant.IsBlocked {
		log.Printf("User %s is blocked from chat %s", userID, chatID)
		return false
	}

	// For private chats, additional checks
	if chatInfo.Settings.IsPrivate {
		// User must be explicitly invited (already checked by being a participant)
		log.Printf("User %s joining private chat %s", userID, chatID)
	}

	return true
}

// scheduleChatCleanup schedules cleanup of empty rooms (simplified)
func scheduleChatCleanup(chatID string) {
	// Immediate cleanup instead of delayed
	hub := GetHubInstance()
	hub.Mutex.Lock()
	defer hub.Mutex.Unlock()

	if chatClients, exists := hub.ChatClients[chatID]; exists && len(chatClients) == 0 {
		delete(hub.ChatClients, chatID)
		log.Printf("Cleaned up empty chat: %s", chatID)
	}
}

// cleanupInactiveConnections periodically removes inactive connections
func cleanupInactiveConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		hub := GetHubInstance()
		now := time.Now()
		inactiveClients := make([]*models.Client, 0)

		for _, client := range hub.Clients {
			if now.Sub(client.LastActivity) > 5*time.Minute {
				inactiveClients = append(inactiveClients, client)
			}
		}

		for _, client := range inactiveClients {
			log.Printf("Removing inactive client: %s", client.ID)
			unregisterClient(client)
		}
	}
}

// notifyUserPresence notifies user's contacts about presence changes
func notifyUserPresence(client *models.Client, status string) {
	// TODO: Get user's contacts and notify them about presence change
	// This would involve querying the contacts collection and sending presence updates

	presenceMsg := models.WebSocketMessage{
		Type:   models.WSMessageTypePresence,
		UserID: client.UserID.Hex(),
		Data: models.WSPresenceStatus{
			UserID:   client.UserID.Hex(),
			Status:   status,
			LastSeen: time.Now(),
		},
		Timestamp: time.Now(),
	}

	// For now, we'll broadcast to all clients
	// In a production system, this should be optimized to only notify contacts
	broadcastToAll(presenceMsg, map[string]bool{client.ID: true})
}

// ============ PUBLIC API FUNCTIONS ============

// CreateClient creates a new WebSocket client
func CreateClient(userID bson.ObjectID, conn *websocket.Conn) *models.Client {
	return &models.Client{
		ID:           uuid.New().String(),
		UserID:       userID,
		Connection:   conn,
		Send:         make(chan models.WebSocketMessage, 256),
		ActiveChats:  make([]string, 0),
		LastActivity: time.Now(),
		Metadata:     make(map[string]interface{}),
	}
}

// RegisterClient registers a client with the hub (direct call)
func RegisterClient(client *models.Client) {
	registerClient(client)
}

// UnregisterClient unregisters a client from the hub (direct call)
func UnregisterClient(client *models.Client) {
	unregisterClient(client)
}

// BroadcastToChat broadcasts a message to a specific chat (direct call)
func BroadcastToChat(chatID string, message models.WebSocketMessage) {
	broadcastToChat(chatID, message, nil)
}

// JoinChatRoom adds a client to a chat (direct call)
func JoinChatRoom(client *models.Client, chatID string) {
	joinReq := models.JoinChatRequest{
		Client: client,
		ChatID: chatID,
	}
	handleJoinChat(joinReq)
}

// LeaveChatRoom removes a client from a chat (direct call)
func LeaveChatRoom(client *models.Client, chatID string) {
	leaveReq := models.LeaveChatRequest{
		Client: client,
		ChatID: chatID,
	}
	handleLeaveChat(leaveReq)
}

// GetChatClients returns clients connected to a chat
func GetChatClients(chatID string) (map[string]*models.Client, bool) {
	hub := GetHubInstance()
	hub.Mutex.RLock()
	defer hub.Mutex.RUnlock()
	clients, exists := hub.ChatClients[chatID]
	return clients, exists
}

// GetUserClients returns all clients for a specific user
func GetUserClients(userID string) map[string]*models.Client {
	hub := GetHubInstance()
	if clients, exists := hub.UserClients[userID]; exists {
		return clients
	}
	return nil
}

// IsUserOnline checks if a user has any active connections
func IsUserOnline(userID string) bool {
	hub := GetHubInstance()
	clients, exists := hub.UserClients[userID]
	return exists && len(clients) > 0
}

// GetHubStats returns current hub statistics
func GetHubStats() map[string]interface{} {
	hub := GetHubInstance()
	hub.Mutex.RLock()
	defer hub.Mutex.RUnlock()
	return map[string]interface{}{
		"totalClients": len(hub.Clients),
		"totalChats":   len(hub.ChatClients),
		"totalUsers":   len(hub.UserClients),
		"cacheSize":    len(hub.UserInfoCache),
		"timestamp":    time.Now(),
	}
}
