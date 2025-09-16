package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gin/internal/models"
	"gin/objects"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

// ============ ENHANCED WEBSOCKET HUB WITH PUB/SUB ============

var (
	// Singleton hub instance with pub/sub support
	enhancedHubInstance *EnhancedHub
	enhancedHubOnce     sync.Once
)

// EnhancedHub represents the WebSocket hub with Redis pub/sub integration
type EnhancedHub struct {
	*models.Hub
	pubSubManager *PubSubManager
	redisClient   *redis.Client
	logger        *zap.Logger
	instanceID    string
}

// GetHubInstance returns the singleton enhanced hub instance
func GetHubInstance() *EnhancedHub {
	enhancedHubOnce.Do(func() {
		// Initialize Redis client
		rdb := redis.NewClient(&redis.Options{
			Addr:     objects.MainConfiguration.RedisUri,
			Password: objects.MainConfiguration.RedisPass,
			DB:       0,
		})

		if err := rdb.Ping(context.Background()).Err(); err != nil {
			panic(fmt.Sprintf("websocket_hub_enhanced.go: Failed to connect to Redis: %v", err))
		}

		// Generate unique instance ID for this server
		instanceID := fmt.Sprintf("instance-%s-%d", uuid.New().String()[:8], time.Now().Unix())

		// Create base hub
		baseHub := &models.Hub{
			Clients:         make(map[string]*models.Client),
			ChatClients:     make(map[string]map[string]*models.Client),
			UserClients:     make(map[string]map[string]*models.Client),
			UserInfoCache:   make(map[string]*models.UserDisplayInfo),
			CacheExpiry:     make(map[string]time.Time),
			UserActiveChats: make(map[string][]string), // Initialize centralized active chats
			Register:        make(chan *models.Client),
			Unregister:      make(chan *models.Client),
		}

		// Create context for hub
		ctx, cancel := context.WithCancel(context.Background())
		baseHub.Ctx = ctx
		baseHub.Cancel = cancel

		// Create logger
		zapLogger, _ := zap.NewProduction()

		// Create enhanced hub
		enhancedHubInstance = &EnhancedHub{
			Hub:         baseHub,
			redisClient: rdb,
			logger:      zapLogger,
			instanceID:  instanceID,
		}

		// Initialize pub/sub manager
		enhancedHubInstance.pubSubManager = NewPubSubManager(
			rdb,
			baseHub,
			zapLogger,
			instanceID,
		)

		// Initialize user lookup service
		InitUserLookupService()
	})
	return enhancedHubInstance
}

// Start initializes and runs the enhanced hub with pub/sub
func (eh *EnhancedHub) Start() error {
	eh.logger.Info("websocket_hub_enhanced.go: Starting enhanced WebSocket hub",
		zap.String("instanceID", eh.instanceID))

	// Start pub/sub manager
	if err := eh.pubSubManager.Start(); err != nil {
		return fmt.Errorf("websocket_hub_enhanced.go: failed to start pub/sub manager: %w", err)
	}

	// Start hub goroutines
	go eh.runHub()
	go eh.cleanupInactiveConnections()
	go eh.syncPresenceWithRedis()

	eh.logger.Info("websocket_hub_enhanced.go: Enhanced WebSocket hub started successfully")
	return nil
}

// Stop gracefully shuts down the enhanced hub
func (eh *EnhancedHub) Stop() {
	eh.logger.Info("websocket_hub_enhanced.go: Stopping enhanced WebSocket hub")

	// Cancel context
	eh.Cancel()

	// Stop pub/sub manager
	eh.pubSubManager.Stop()

	// Close all client connections
	eh.Mutex.Lock()
	for _, client := range eh.Clients {
		close(client.Send)
		client.Connection.Close()
	}
	eh.Mutex.Unlock()

	eh.logger.Info("websocket_hub_enhanced.go: Enhanced WebSocket hub stopped")
}

// runHub manages the main hub operations
func (eh *EnhancedHub) runHub() {
	for {
		select {
		case <-eh.Ctx.Done():
			return

		case client := <-eh.Register:
			eh.handleClientRegistration(client)

		case client := <-eh.Unregister:
			eh.handleClientUnregistration(client)
		}
	}
}

// handleClientRegistration handles new client connections with pub/sub
func (eh *EnhancedHub) handleClientRegistration(client *models.Client) {
	eh.logger.Info("websocket_hub_enhanced.go: Registering new client",
		zap.String("clientID", client.ID),
		zap.String("userID", client.UserID.Hex()))

	eh.Mutex.Lock()
	defer eh.Mutex.Unlock()

	// Add client to hub
	eh.Clients[client.ID] = client

	// Add to user mapping
	userID := client.UserID.Hex()
	if eh.UserClients[userID] == nil {
		eh.UserClients[userID] = make(map[string]*models.Client)
	}
	eh.UserClients[userID][client.ID] = client

	// Get user's chats from database
	ctx := context.Background()
	userChats, err := eh.getUserChats(ctx, client.UserID)
	if err != nil {
		eh.logger.Error("websocket_hub_enhanced.go: Failed to get user chats",
			zap.Error(err))
		userChats = []string{}
	}

	// Subscribe to user and chat channels via pub/sub
	if err := eh.pubSubManager.SubscribeUserToChannels(client.UserID, userChats); err != nil {
		eh.logger.Error("websocket_hub_enhanced.go: Failed to subscribe to channels",
			zap.Error(err))
	}

	// updated part
	// Add client to all their chat rooms (this was missing!)
	for _, chatID := range userChats {
		if eh.ChatClients[chatID] == nil {
			eh.ChatClients[chatID] = make(map[string]*models.Client)
		}
		eh.ChatClients[chatID][client.ID] = client
		eh.logger.Debug("websocket_hub_enhanced.go: Added client to chat",
			zap.String("clientID", client.ID),
			zap.String("chatID", chatID))
	}

	// Update user's active chats
	if len(userChats) > 0 {
		eh.UserActiveChats[userID] = userChats
	}

	// Update presence
	eh.updateUserPresence(client.UserID, "online")

	// Publish presence update to Redis
	presenceStatus := &models.WSPresenceStatus{
		UserID:   userID,
		Status:   "online",
		LastSeen: time.Now(),
	}
	if err := eh.pubSubManager.PublishPresenceUpdate(presenceStatus); err != nil {
		eh.logger.Error("websocket_hub_enhanced.go: Failed to publish presence update",
			zap.Error(err))
	}

	// Send welcome message
	welcomeMsg := models.WebSocketMessage{
		Type:      models.WSMessageTypeResponse,
		UserID:    userID,
		Data:      map[string]interface{}{"status": "connected", "clientId": client.ID, "instanceId": eh.instanceID},
		Timestamp: time.Now(),
	}

	select {
	case client.Send <- welcomeMsg:
	default:
		eh.logger.Warn("websocket_hub_enhanced.go: Failed to send welcome message")
	}
}

// handleClientUnregistration handles client disconnections with pub/sub cleanup
func (eh *EnhancedHub) handleClientUnregistration(client *models.Client) {
	eh.logger.Info("websocket_hub_enhanced.go: Unregistering client",
		zap.String("clientID", client.ID),
		zap.String("userID", client.UserID.Hex()))

	eh.Mutex.Lock()
	defer eh.Mutex.Unlock()

	if _, ok := eh.Clients[client.ID]; !ok {
		return
	}

	// Remove from all chats
	userID := client.UserID.Hex()
	if activeChats, exists := eh.UserActiveChats[userID]; exists {
		for _, chatID := range activeChats {
			eh.removeClientFromChat(client, chatID)
		}
	}

	// Remove from hub
	delete(eh.Clients, client.ID)

	// Remove from user mapping
	if userClients, exists := eh.UserClients[userID]; exists {
		delete(userClients, client.ID)

		// If no more clients for this user
		if len(userClients) == 0 {
			delete(eh.UserClients, userID)

			// Get user's active chats for unsubscription
			if activeChats, exists := eh.UserActiveChats[userID]; exists {
				// Unsubscribe from channels
				if err := eh.pubSubManager.UnsubscribeUserFromChannels(client.UserID, activeChats); err != nil {
					eh.logger.Error("websocket_hub_enhanced.go: Failed to unsubscribe from channels",
						zap.Error(err))
				}
			}

			// Update presence to offline
			eh.updateUserPresence(client.UserID, "offline")

			// Publish presence update
			presenceStatus := &models.WSPresenceStatus{
				UserID:   userID,
				Status:   "offline",
				LastSeen: time.Now(),
			}
			if err := eh.pubSubManager.PublishPresenceUpdate(presenceStatus); err != nil {
				eh.logger.Error("websocket_hub_enhanced.go: Failed to publish presence update",
					zap.Error(err))
			}
		}
	}

	// Close send channel
	close(client.Send)
}

// JoinChat handles client joining a chat with pub/sub subscription
func (eh *EnhancedHub) JoinChat(ctx context.Context, client *models.Client, chatID string) error {
	eh.logger.Info("websocket_hub_enhanced.go: Client joining chat",
		zap.String("clientID", client.ID),
		zap.String("chatID", chatID))

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

	// Subscribe to chat channel if not already subscribed
	chatChannel := "chat:" + chatID
	if !eh.pubSubManager.IsSubscribed(chatChannel) {
		if err := eh.pubSubManager.subscribeToChannel(chatChannel); err != nil {
			eh.logger.Error("websocket_hub_enhanced.go: Failed to subscribe to chat channel",
				zap.String("chatID", chatID),
				zap.Error(err))
		}
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
func (eh *EnhancedHub) LeaveChat(client *models.Client, chatID string) error {
	eh.logger.Info("websocket_hub_enhanced.go: Client leaving chat",
		zap.String("clientID", client.ID),
		zap.String("chatID", chatID))

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

// SendChatMessage sends a message to a chat via pub/sub
func (eh *EnhancedHub) SendChatMessage(ctx context.Context, chatID string, message *models.ChatMessage) error {
	eh.logger.Info("websocket_hub_enhanced.go: Sending chat message",
		zap.String("chatID", chatID),
		zap.String("messageID", message.ID))

	// Create WebSocket message wrapper
	wsMessage := &models.WebSocketMessage{
		Type:      models.WSMessageTypeChat,
		ChatID:    chatID,
		UserID:    message.SenderID,
		Data:      message,
		Timestamp: message.CreatedAt,
	}

	// Publish to chat channel for cross-instance delivery
	return eh.pubSubManager.PublishToChat(chatID, wsMessage)
}

// SendDirectMessage sends a direct message to a specific user via pub/sub
func (eh *EnhancedHub) SendDirectMessage(ctx context.Context, targetUserID string, message *models.ChatMessage) error {
	eh.logger.Info("websocket_hub_enhanced.go: Sending direct message",
		zap.String("targetUserID", targetUserID),
		zap.String("messageID", message.ID))

	// Create WebSocket message wrapper
	wsMessage := &models.WebSocketMessage{
		Type:      models.WSMessageTypeChat,
		UserID:    message.SenderID,
		Data:      message,
		Timestamp: message.CreatedAt,
	}

	// Publish to user channel for cross-instance delivery
	return eh.pubSubManager.PublishToUser(targetUserID, wsMessage)
}

// BroadcastTypingIndicator broadcasts typing status via pub/sub
func (eh *EnhancedHub) BroadcastTypingIndicator(chatID string, userID string, isTyping bool) error {
	eh.logger.Debug("websocket_hub_enhanced.go: Broadcasting typing indicator",
		zap.String("chatID", chatID),
		zap.String("userID", userID),
		zap.Bool("isTyping", isTyping))

	// Get user display info
	userObjectID, _ := bson.ObjectIDFromHex(userID)
	userInfo, _ := GetUserDisplayInfo(userObjectID)
	username := "Unknown"
	if userInfo != nil {
		username = userInfo.Username
	}

	typingMsg := &models.WebSocketMessage{
		Type:   models.WSMessageTypeTyping,
		ChatID: chatID,
		UserID: userID,
		Data: models.TypingIndicator{
			UserID:    userID,
			Username:  username,
			IsTyping:  isTyping,
			Timestamp: time.Now(),
		},
		Timestamp: time.Now(),
	}

	return eh.pubSubManager.PublishToChat(chatID, typingMsg)
}

// removeClientFromChat removes a client from a chat (internal helper)
func (eh *EnhancedHub) removeClientFromChat(client *models.Client, chatID string) {
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
func (eh *EnhancedHub) getUserChats(ctx context.Context, userID bson.ObjectID) ([]string, error) {
	// Query database for user's chats
	chats, err := GetUserChats(ctx, userID)
	if err != nil {
		return nil, err
	}

	chatIDs := make([]string, 0, len(chats))
	for _, chat := range chats {
		chatIDs = append(chatIDs, chat.Hex())
	}

	return chatIDs, nil
}

// updateUserPresence updates user presence in cache and Redis
func (eh *EnhancedHub) updateUserPresence(userID bson.ObjectID, status string) {
	eh.Mutex.Lock()
	defer eh.Mutex.Unlock()

	userIDStr := userID.Hex()

	// Update local cache
	if userInfo, exists := eh.UserInfoCache[userIDStr]; exists {
		userInfo.Status = status
		userInfo.IsOnline = status != "offline"
		userInfo.LastSeen = time.Now()
		eh.CacheExpiry[userIDStr] = time.Now().Add(5 * time.Minute)
	} else {
		// Create new cache entry - use lookup service directly since we already checked cache
		userInfo, _ := GetUserDisplayInfo(userID)
		if userInfo != nil {
			userInfo.Status = status
			userInfo.IsOnline = status != "offline"
			userInfo.LastSeen = time.Now()
			eh.UserInfoCache[userIDStr] = userInfo
			eh.CacheExpiry[userIDStr] = time.Now().Add(5 * time.Minute)
		}
	}

	// Update presence in Redis for persistence
	ctx := context.Background()
	presenceKey := fmt.Sprintf("presence:%s", userIDStr)
	presenceData := map[string]interface{}{
		"status":   status,
		"lastSeen": time.Now().Unix(),
	}

	eh.redisClient.HSet(ctx, presenceKey, presenceData)
	eh.redisClient.Expire(ctx, presenceKey, 30*time.Minute)
}

// syncPresenceWithRedis periodically syncs presence data with Redis
func (eh *EnhancedHub) syncPresenceWithRedis() {
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
func (eh *EnhancedHub) performPresenceSync() {
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
		eh.logger.Error("websocket_hub_enhanced.go: Failed to sync presence with Redis",
			zap.Error(err))
	}
}

// cleanupInactiveConnections removes inactive connections
func (eh *EnhancedHub) cleanupInactiveConnections() {
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
func (eh *EnhancedHub) performCleanup() {
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
		eh.logger.Info("websocket_hub_enhanced.go: Removing inactive client",
			zap.String("clientID", client.ID))
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
func (eh *EnhancedHub) GetUserInfo(userID bson.ObjectID) (*models.UserDisplayInfo, error) {
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
	userInfo, err := GetUserDisplayInfo(userID)
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
func (eh *EnhancedHub) GetStats() map[string]interface{} {
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
