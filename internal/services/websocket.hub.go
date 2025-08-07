package services

import (
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
			Clients:     make(map[string]*models.Client),
			ChatRooms:   make(map[string]*models.ChatRoom),
			UserClients: make(map[string]map[string]*models.Client),
			Register:    make(chan *models.Client, 256),
			Unregister:  make(chan *models.Client, 256),
			Broadcast:   make(chan models.HubMessage, 1024),
			JoinRoom:    make(chan models.JoinRoomRequest, 256),
			LeaveRoom:   make(chan models.LeaveRoomRequest, 256),
		}
	})
	return hubInstance
}

// RunHub starts the WebSocket hub and handles all real-time operations
func RunHub() {
	hub := GetHubInstance()

	// Start cleanup goroutine for inactive connections
	go cleanupInactiveConnections()

	log.Println("WebSocket Hub started - Managing real-time connections")

	for {
		select {
		case client := <-hub.Register:
			registerClient(client)

		case client := <-hub.Unregister:
			unregisterClient(client)

		case message := <-hub.Broadcast:
			broadcastMessage(message)

		case joinReq := <-hub.JoinRoom:
			handleJoinRoom(joinReq)

		case leaveReq := <-hub.LeaveRoom:
			handleLeaveRoom(leaveReq)
		}
	}
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
	go UpdateUserPresence(client.UserID, string(objects.UserStatusOnline))

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

	log.Printf("Client %s (User: %s) connected. Total clients: %d",
		client.ID, client.Username, len(hub.Clients))

	// Notify user's contacts about online status
	go notifyUserPresence(client, string(objects.UserStatusOnline))
}

// unregisterClient removes a client connection
func unregisterClient(client *models.Client) {
	hub := GetHubInstance()

	if _, ok := hub.Clients[client.ID]; ok {
		// Remove from all chat rooms
		for chatID := range client.ChatRooms {
			removeClientFromRoom(client, chatID)
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
				go UpdateUserPresence(client.UserID, string(objects.UserStatusOffline))
				go notifyUserPresence(client, string(objects.UserStatusOffline))
			}
		}

		// Close send channel
		close(client.Send)

		log.Printf("Client %s (User: %s) disconnected. Total clients: %d",
			client.ID, client.Username, len(hub.Clients))
	}
}

// ============ CHAT ROOM MANAGEMENT ============

// handleJoinRoom adds a client to a chat room
func handleJoinRoom(req models.JoinRoomRequest) {
	hub := GetHubInstance()

	// Create room if it doesn't exist
	if _, exists := hub.ChatRooms[req.ChatID]; !exists {
		createChatRoom(req.ChatID)
	}

	room := hub.ChatRooms[req.ChatID]

	// Check if client can join (permissions, max clients, etc.)
	if !canJoinRoom(req.Client, room) {
		errorMsg := models.WebSocketMessage{
			Type: models.WSMessageTypeError,
			Data: models.ErrorMessage{
				Code:    "JOIN_DENIED",
				Message: "Permission denied or room is full",
			},
			Timestamp: time.Now(),
		}
		req.Client.Send <- errorMsg
		return
	}

	// Add client to room
	room.Clients[req.Client.ID] = req.Client
	req.Client.ChatRooms[req.ChatID] = true
	room.LastActivity = time.Now()

	// Notify other clients in the room
	joinMsg := models.WebSocketMessage{
		Type:   models.WSMessageTypeJoin,
		ChatID: req.ChatID,
		UserID: req.Client.UserID.Hex(),
		Data: models.UserJoinLeave{
			UserID:    req.Client.UserID.Hex(),
			Username:  req.Client.Username,
			Action:    "join",
			Timestamp: time.Now(),
		},
		Timestamp: time.Now(),
	}

	broadcastToRoom(req.ChatID, joinMsg, req.Client.ID)

	// Send success response to joining client
	successMsg := models.WebSocketMessage{
		Type:   models.WSMessageTypeResponse,
		ChatID: req.ChatID,
		Data: map[string]interface{}{
			"action":  "joined",
			"chatId":  req.ChatID,
			"members": len(room.Clients),
		},
		Timestamp: time.Now(),
	}
	req.Client.Send <- successMsg

	log.Printf("Client %s joined room %s. Room members: %d",
		req.Client.ID, req.ChatID, len(room.Clients))
}

// handleLeaveRoom removes a client from a chat room
func handleLeaveRoom(req models.LeaveRoomRequest) {
	hub := GetHubInstance()

	if room, exists := hub.ChatRooms[req.ChatID]; exists {
		removeClientFromRoom(req.Client, req.ChatID)

		// Notify other clients in the room
		leaveMsg := models.WebSocketMessage{
			Type:   models.WSMessageTypeLeave,
			ChatID: req.ChatID,
			UserID: req.Client.UserID.Hex(),
			Data: models.UserJoinLeave{
				UserID:    req.Client.UserID.Hex(),
				Username:  req.Client.Username,
				Action:    "leave",
				Timestamp: time.Now(),
			},
			Timestamp: time.Now(),
		}

		broadcastToRoom(req.ChatID, leaveMsg, req.Client.ID)

		log.Printf("Client %s left room %s. Room members: %d",
			req.Client.ID, req.ChatID, len(room.Clients))
	}
}

// removeClientFromRoom removes a client from a specific room
func removeClientFromRoom(client *models.Client, chatID string) {
	hub := GetHubInstance()

	if room, exists := hub.ChatRooms[chatID]; exists {
		delete(room.Clients, client.ID)
		delete(client.ChatRooms, chatID)

		// Remove empty rooms after a delay
		if len(room.Clients) == 0 {
			go scheduleRoomCleanup(chatID)
		}
	}
}

// ============ MESSAGE BROADCASTING ============

// broadcastMessage broadcasts a message to appropriate clients
func broadcastMessage(hubMsg models.HubMessage) {
	if hubMsg.ChatID != "" {
		// Broadcast to specific chat room
		broadcastToRoom(hubMsg.ChatID, hubMsg.Message, "")
	} else {
		// Global broadcast (rare case)
		broadcastToAll(hubMsg.Message, hubMsg.Exclude)
	}
}

// broadcastToRoom broadcasts a message to all clients in a chat room
func broadcastToRoom(chatID string, message models.WebSocketMessage, excludeClientID string) {
	hub := GetHubInstance()

	room, exists := hub.ChatRooms[chatID]
	if !exists {
		log.Printf("Attempted to broadcast to non-existent room: %s", chatID)
		return
	}

	room.LastActivity = time.Now()

	for clientID, client := range room.Clients {
		if clientID == excludeClientID {
			continue
		}

		select {
		case client.Send <- message:
		default:
			// Client's send channel is full or closed, remove client
			removeClientFromRoom(client, chatID)
			log.Printf("Removed unresponsive client %s from room %s", clientID, chatID)
		}
	}

	log.Printf("Broadcasted message to room %s (%d clients)", chatID, len(room.Clients))
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

// createChatRoom creates a new chat room
func createChatRoom(chatID string) {
	hub := GetHubInstance()

	room := &models.ChatRoom{
		ID:           chatID,
		Type:         "group", // Default type
		Clients:      make(map[string]*models.Client),
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		Settings: models.ChatRoomSettings{
			MaxClients:      100, // Default max clients
			AllowAnonymous:  false,
			MessageHistory:  true,
			TypingIndicator: true,
			ReadReceipts:    true,
		},
	}

	hub.ChatRooms[chatID] = room
	log.Printf("Created new chat room: %s", chatID)
}

// canJoinRoom checks if a client can join a specific room
func canJoinRoom(client *models.Client, room *models.ChatRoom) bool {
	// Check max clients limit
	if len(room.Clients) >= room.Settings.MaxClients {
		return false
	}

	// TODO: Add more permission checks here
	// - Check if user is banned from room
	// - Check if room is private and user has invite
	// - Check user role permissions

	return true
}

// scheduleRoomCleanup schedules cleanup of empty rooms
func scheduleRoomCleanup(chatID string) {
	time.Sleep(5 * time.Minute) // Wait 5 minutes before cleanup

	hub := GetHubInstance()
	if room, exists := hub.ChatRooms[chatID]; exists && len(room.Clients) == 0 {
		delete(hub.ChatRooms, chatID)
		log.Printf("Cleaned up empty room: %s", chatID)
	}
}

// cleanupInactiveConnections periodically removes inactive connections
func cleanupInactiveConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
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
func CreateClient(userID bson.ObjectID, username string, conn *websocket.Conn) *models.Client {
	return &models.Client{
		ID:           uuid.New().String(),
		UserID:       userID,
		Username:     username,
		Connection:   conn,
		Send:         make(chan models.WebSocketMessage, 256),
		ChatRooms:    make(map[string]bool),
		LastActivity: time.Now(),
		Metadata:     make(map[string]interface{}),
	}
}

// RegisterClient registers a client with the hub
func RegisterClient(client *models.Client) {
	hub := GetHubInstance()
	hub.Register <- client
}

// UnregisterClient unregisters a client from the hub
func UnregisterClient(client *models.Client) {
	hub := GetHubInstance()
	hub.Unregister <- client
}

// BroadcastToChat broadcasts a message to a specific chat
func BroadcastToChat(chatID string, message models.WebSocketMessage) {
	hub := GetHubInstance()
	hubMsg := models.HubMessage{
		ChatID:  chatID,
		Message: message,
	}
	hub.Broadcast <- hubMsg
}

// JoinChatRoom adds a client to a chat room
func JoinChatRoom(client *models.Client, chatID string) {
	hub := GetHubInstance()
	joinReq := models.JoinRoomRequest{
		Client: client,
		ChatID: chatID,
	}
	hub.JoinRoom <- joinReq
}

// LeaveChatRoom removes a client from a chat room
func LeaveChatRoom(client *models.Client, chatID string) {
	hub := GetHubInstance()
	leaveReq := models.LeaveRoomRequest{
		Client: client,
		ChatID: chatID,
	}
	hub.LeaveRoom <- leaveReq
}

// GetRoomInfo returns information about a chat room
func GetRoomInfo(chatID string) (*models.ChatRoom, bool) {
	hub := GetHubInstance()
	room, exists := hub.ChatRooms[chatID]
	return room, exists
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
	return map[string]interface{}{
		"totalClients": len(hub.Clients),
		"totalRooms":   len(hub.ChatRooms),
		"totalUsers":   len(hub.UserClients),
		"timestamp":    time.Now(),
	}
}
