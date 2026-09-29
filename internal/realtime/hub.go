package realtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	hubInstance *Hub
	hubOnce     sync.Once
)

// Hub tracks connected WebSocket clients on this server instance and relays
// chat events through Redis pub/sub so several instances can share rooms.
type Hub struct {
	*models.Hub
	pubSubManager *PubSubManager
	redisClient   *redis.Client
	logger        *logrus.Logger
	instanceID    string
}

// GetHubInstance returns the process-wide hub, creating it on first use.
// database.ConnectRedis must have succeeded before the first call.
func GetHubInstance() *Hub {
	hubOnce.Do(func() {
		instanceID := fmt.Sprintf("instance-%s-%d", uuid.New().String()[:8], time.Now().Unix())

		baseHub := &models.Hub{
			Clients:         make(map[string]*models.Client),
			ChatClients:     make(map[string]map[string]*models.Client),
			UserClients:     make(map[string]map[string]*models.Client),
			UserInfoCache:   make(map[string]*models.UserDisplayInfo),
			CacheExpiry:     make(map[string]time.Time),
			Register:        make(chan *models.Client, 1000),
			Unregister:      make(chan *models.Client, 1000),
			UserActiveChats: make(map[string][]string),
		}
		baseHub.Ctx, baseHub.Cancel = context.WithCancel(context.Background())

		hubInstance = &Hub{
			Hub:         baseHub,
			redisClient: database.Redis,
			logger:      logger.Base(),
			instanceID:  instanceID,
		}
		hubInstance.pubSubManager = NewPubSubManager(database.Redis, baseHub, logger.Base(), instanceID)
	})
	return hubInstance
}

// PublishToChat delivers a message to every client in a chat, on any instance.
func (eh *Hub) PublishToChat(chatID string, message *models.WebSocketMessage) error {
	return eh.pubSubManager.PublishToChat(chatID, message)
}

// Start initializes and runs the enhanced hub with pub/sub
func (eh *Hub) Start() error {
	eh.logger.WithFields(logrus.Fields{"instanceID": eh.instanceID}).Info("Starting enhanced WebSocket hub")

	// Start pub/sub manager
	if err := eh.pubSubManager.Start(); err != nil {
		return fmt.Errorf("failed to start pub/sub manager: %w", err)
	}

	// Start hub goroutines
	go eh.runHub()
	go eh.cleanupInactiveConnections()
	go eh.syncPresenceWithRedis()

	eh.logger.Info("Enhanced WebSocket hub started successfully")
	return nil
}

// Restart attempts to restart the hub if it has stopped
func (eh *Hub) Restart() error {
	eh.logger.Info("Attempting to restart hub")

	// Check if context is cancelled
	select {
	case <-eh.Ctx.Done():
		eh.logger.Warn("Hub context is cancelled, creating new context")
		// Create new context
		ctx, cancel := context.WithCancel(context.Background())
		eh.Hub.Ctx = ctx
		eh.Hub.Cancel = cancel
	default:
		eh.logger.Info("Hub context is still active")
	}

	// Restart goroutines
	go eh.runHub()
	go eh.cleanupInactiveConnections()
	go eh.syncPresenceWithRedis()

	eh.logger.Info("Hub restarted successfully")
	return nil
}

// runHub manages the main hub operations
func (eh *Hub) runHub() {
	for {
		select {
		case <-eh.Ctx.Done():
			eh.logger.Warn("Hub context cancelled, stopping main loop")
			return

		case client := <-eh.Register:
			eh.handleClientRegistration(client)

		case client := <-eh.Unregister:
			eh.handleClientUnregistration(client)
		}
	}
}

// handleClientRegistration handles new client connections with pub/sub
func (eh *Hub) handleClientRegistration(client *models.Client) {
	eh.logger.WithFields(logrus.Fields{"clientID": client.ID, "userID": client.UserID.Hex()}).Info("Registering new client")

	// Get user's chats from database BEFORE acquiring mutex (avoid deadlock!)
	ctx := context.Background()
	userChats, err := eh.getUserChats(ctx, client.UserID)
	if err != nil {
		eh.logger.WithError(err).Error("Failed to get user chats")
		userChats = []string{}
	}

	// Subscribe to user and chat channels via pub/sub BEFORE acquiring mutex
	if err := eh.pubSubManager.SubscribeUserToChannels(client.UserID, userChats); err != nil {
		eh.logger.WithError(err).Error("Failed to subscribe to channels")
	}

	eh.Mutex.Lock()

	// Add client to hub
	eh.Clients[client.ID] = client

	// Add to user mapping
	userID := client.UserID.Hex()
	if eh.UserClients[userID] == nil {
		eh.UserClients[userID] = make(map[string]*models.Client)
	}
	eh.UserClients[userID][client.ID] = client

	// Add client to all their chat rooms
	for _, chatID := range userChats {
		if eh.ChatClients[chatID] == nil {
			eh.ChatClients[chatID] = make(map[string]*models.Client)
		}
		eh.ChatClients[chatID][client.ID] = client
		eh.logger.WithFields(logrus.Fields{"clientID": client.ID, "chatID": chatID}).Debug("Added client to chat")
	}

	// Update user's active chats
	if len(userChats) > 0 {
		eh.UserActiveChats[userID] = userChats
	}

	presence, ok := GetPresenceInstance().Get(client.UserID)
	if ok {
		if presence.Status == UserStatus(constants.UserStatusOnline) {
			if err := eh.pubSubManager.PublishPresenceUpdate(&models.WSPresenceStatus{
				UserID:   userID,
				Status:   string(constants.UserStatusOnline),
				LastSeen: time.Now(),
			}); err != nil {
				eh.logger.WithError(err).Error("Failed to publish presence update")
			}
		}
	}

	eh.Mutex.Unlock()

	// Send welcome message
	welcomeMsg := models.WebSocketMessage{
		Type:      models.WSMessageTypeResponse,
		UserID:    userID,
		Data:      map[string]interface{}{"status": "connected", "clientId": client.ID, "instanceId": eh.instanceID},
		Timestamp: time.Now(),
	}

	if !client.TrySend(welcomeMsg) {
		eh.logger.Warn("Failed to send welcome message")
	}
}

// handleClientUnregistration handles client disconnections with pub/sub cleanup
func (eh *Hub) handleClientUnregistration(client *models.Client) {
	eh.logger.WithFields(logrus.Fields{"clientID": client.ID, "userID": client.UserID.Hex()}).Info("Unregistering client")

	if _, ok := eh.Clients[client.ID]; !ok {
		return
	}
	eh.Mutex.Lock()

	clientID := client.ID
	userID := client.UserID.Hex()

	for _, chatID := range eh.UserActiveChats[userID] {
		if _, ok := eh.ChatClients[chatID][clientID]; ok {
			delete(eh.ChatClients[chatID], clientID)

			// If their is no other client of same user in the chat, delete the chat
			if len(eh.ChatClients[chatID]) == 0 {
				delete(eh.ChatClients, chatID)
			}
		}
	}

	delete(eh.Clients, clientID)

	// Remove from hub
	eh.Mutex.Unlock()

	// Remove from user mapping
	if userClients, exists := eh.UserClients[userID]; exists {
		eh.Mutex.Lock()
		delete(userClients, client.ID)
		eh.Mutex.Unlock()

		// If no more clients for this user
		if len(eh.UserClients[userID]) == 0 {
			eh.Mutex.Lock()
			delete(eh.UserClients, userID)
			eh.Mutex.Unlock()

			// Remove from active chats
			if activeChats, exists := eh.UserActiveChats[userID]; exists {
				for _, chatID := range activeChats {
					eh.removeClientFromChat(client, chatID)
				}
			}

			// Get user's active chats for unsubscription
			if activeChats, exists := eh.UserActiveChats[userID]; exists {
				// Unsubscribe from channels
				if err := eh.pubSubManager.UnsubscribeUserFromChannels(client.UserID, activeChats); err != nil {
					eh.logger.WithError(err).Error("Failed to unsubscribe from channels")
				}
			}

			// delete from presence
			GetPresenceInstance().Delete(client.UserID)

			// Publish presence update
			presenceStatus := &models.WSPresenceStatus{
				UserID:   userID,
				Status:   string(constants.UserStatusOffline),
				LastSeen: time.Now(),
			}
			if err := eh.pubSubManager.PublishPresenceUpdate(presenceStatus); err != nil {
				eh.logger.WithError(err).Error("Failed to publish presence update")
			}
		}
	}

	// Close send channel
	client.CloseSend()
}

// JoinChat handles client joining a chat with pub/sub subscription
func (eh *Hub) JoinChat(ctx context.Context, client *models.Client, chatID string) error {
	eh.logger.WithFields(logrus.Fields{"clientID": client.ID, "chatID": chatID}).Info("Client joining chat")

	// Check if pub/sub subscription is needed BEFORE acquiring mutex
	chatChannel := "chat:" + chatID
	needsSubscription := false
	if !eh.pubSubManager.IsSubscribed(chatChannel) {
		needsSubscription = true
	}

	// Subscribe to chat channel BEFORE acquiring mutex (if needed)
	if needsSubscription {
		if err := eh.pubSubManager.subscribeToChannel(chatChannel); err != nil {
			eh.logger.WithFields(logrus.Fields{"chatID": chatID}).WithError(err).Error("Failed to subscribe to chat channel")
		}
	}

	eh.Mutex.Lock()
	defer eh.Mutex.Unlock()

	// Add to chat clients
	if eh.ChatClients[chatID] == nil {
		eh.ChatClients[chatID] = make(map[string]*models.Client)
	}
	eh.ChatClients[chatID][client.ID] = client

	// Add to user's active chats (centralized in hub)
	userID := client.UserID.Hex()
	alreadyInChat := false

	if activeChats, exists := eh.UserActiveChats[userID]; exists {
		for _, activeChat := range activeChats {
			if activeChat == chatID {
				alreadyInChat = true
				break
			}
		}
		if !alreadyInChat {
			eh.UserActiveChats[userID] = append(activeChats, chatID)
		}
	} else {
		// First time this user has active chats
		eh.UserActiveChats[userID] = []string{chatID}
	}

	// Publish join event to chat
	joinMsg := &models.WebSocketMessage{
		Type:   models.WSMessageTypeJoin,
		ChatID: chatID,
		UserID: client.UserID.Hex(),
		Data: models.UserJoinLeave{
			UserID:    client.UserID.Hex(),
			Action:    "join",
			Timestamp: time.Now(),
		},
		Timestamp: time.Now(),
	}

	return eh.pubSubManager.PublishToChat(chatID, joinMsg)
}

// LeaveChat handles client leaving a chat with pub/sub cleanup
func (eh *Hub) LeaveChat(client *models.Client, chatID string) error {
	eh.logger.WithFields(logrus.Fields{"clientID": client.ID, "chatID": chatID}).Info("Client leaving chat")

	eh.Mutex.Lock()
	defer eh.Mutex.Unlock()

	// Remove from chat
	eh.removeClientFromChat(client, chatID)

	// Publish leave event
	leaveMsg := &models.WebSocketMessage{
		Type:   models.WSMessageTypeLeave,
		ChatID: chatID,
		UserID: client.UserID.Hex(),
		Data: models.UserJoinLeave{
			UserID:    client.UserID.Hex(),
			Action:    "leave",
			Timestamp: time.Now(),
		},
		Timestamp: time.Now(),
	}

	return eh.pubSubManager.PublishToChat(chatID, leaveMsg)
}

// removeClientFromChat removes a client from a chat (internal helper)
func (eh *Hub) removeClientFromChat(client *models.Client, chatID string) {
	if chatClients, exists := eh.ChatClients[chatID]; exists {
		delete(chatClients, client.ID)

		// Remove empty chat map
		if len(chatClients) == 0 {
			delete(eh.ChatClients, chatID)

			// Chat channel unsubscription is now handled safely in UnsubscribeUserFromChannels
			// with proper reference counting to avoid breaking other users in the chat
		}
	}

	// Remove from user's active chats (centralized in hub)
	userID := client.UserID.Hex()
	if activeChats, exists := eh.UserActiveChats[userID]; exists {
		for i, activeChat := range activeChats {
			if activeChat == chatID {
				eh.UserActiveChats[userID] = append(activeChats[:i], activeChats[i+1:]...)
				break
			}
		}
		// If no more active chats for this user, clean up
		if len(eh.UserActiveChats[userID]) == 0 {
			delete(eh.UserActiveChats, userID)
		}
	}
}

// getUserChats retrieves all chat IDs for a user from the database
func (eh *Hub) getUserChats(ctx context.Context, userID bson.ObjectID) ([]string, error) {
	// Query database for user's chats
	chats, err := services.GetUserChats(ctx, userID)
	if err != nil {
		return nil, err
	}

	chatIDs := make([]string, 0, len(chats))
	for _, chat := range chats {
		chatIDs = append(chatIDs, chat.Hex())
	}

	return chatIDs, nil
}

// syncPresenceWithRedis periodically syncs presence data with Redis
func (eh *Hub) syncPresenceWithRedis() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-eh.Ctx.Done():
			return
		case <-ticker.C:
			eh.performPresenceSync()
		}
	}
}

// performPresenceSync syncs local presence data with Redis
func (eh *Hub) performPresenceSync() {
	eh.Mutex.RLock()
	userIDs := make([]string, 0, len(eh.UserClients))
	for userID := range eh.UserClients {
		userIDs = append(userIDs, userID)
	}
	eh.Mutex.RUnlock()

	ctx := context.Background()
	pipe := eh.redisClient.Pipeline()

	for _, userID := range userIDs {
		presenceKey := fmt.Sprintf("presence:%s", userID)
		presenceData := map[string]interface{}{
			"status":     "online",
			"lastSeen":   time.Now().Unix(),
			"instanceId": eh.instanceID,
		}
		pipe.HSet(ctx, presenceKey, presenceData)
		pipe.Expire(ctx, presenceKey, 2*time.Minute)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		eh.logger.WithError(err).Error("Failed to sync presence with Redis")
	}
}

// cleanupInactiveConnections removes inactive connections
func (eh *Hub) cleanupInactiveConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-eh.Ctx.Done():
			return
		case <-ticker.C:
			eh.performCleanup()
		}
	}
}

// performCleanup removes inactive clients
func (eh *Hub) performCleanup() {
	eh.Mutex.Lock()
	defer eh.Mutex.Unlock()

	now := time.Now()
	inactiveThreshold := 5 * time.Minute
	toRemove := make([]*models.Client, 0)

	for _, client := range eh.Clients {
		if now.Sub(client.LastActivity) > inactiveThreshold {
			toRemove = append(toRemove, client)
		}
	}

	for _, client := range toRemove {
		eh.logger.WithFields(logrus.Fields{"clientID": client.ID}).Info("Removing inactive client")
		eh.Unregister <- client
	}

	// Clean expired cache entries
	for userID, expiry := range eh.CacheExpiry {
		if now.After(expiry) {
			delete(eh.UserInfoCache, userID)
			delete(eh.CacheExpiry, userID)
		}
	}
}

// GetUserInfo gets user display information from hub cache first, then lookup service
func (eh *Hub) GetUserInfo(userID bson.ObjectID) (*models.UserDisplayInfo, error) {
	eh.Mutex.RLock()
	userIDStr := userID.Hex()

	// Check hub cache first
	if userInfo, exists := eh.UserInfoCache[userIDStr]; exists {
		// Check if cache entry is still valid
		if expiry, hasExpiry := eh.CacheExpiry[userIDStr]; hasExpiry && time.Now().Before(expiry) {
			eh.Mutex.RUnlock()
			return userInfo, nil
		}
	}
	eh.Mutex.RUnlock()

	// Not in cache or expired, get from lookup service
	userInfo, err := services.GetUserDisplayInfoFromDB(userID)
	if err != nil {
		return nil, err
	}

	// Cache the result in hub for future use
	eh.Mutex.Lock()
	eh.UserInfoCache[userIDStr] = userInfo
	eh.CacheExpiry[userIDStr] = time.Now().Add(5 * time.Minute)
	eh.Mutex.Unlock()

	return userInfo, nil
}

// GetStats returns hub statistics including pub/sub info
func (eh *Hub) GetStats() map[string]interface{} {
	eh.Mutex.RLock()
	defer eh.Mutex.RUnlock()

	subscribedChannels := eh.pubSubManager.GetSubscribedChannels()

	return map[string]interface{}{
		"instanceId":         eh.instanceID,
		"totalClients":       len(eh.Clients),
		"totalChats":         len(eh.ChatClients),
		"totalUsers":         len(eh.UserClients),
		"cacheSize":          len(eh.UserInfoCache),
		"subscribedChannels": len(subscribedChannels),
		"channels":           subscribedChannels,
		"timestamp":          time.Now(),
	}
}
