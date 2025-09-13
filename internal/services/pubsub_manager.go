package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"

	"gin/internal/models"
)

// PubSubManager handles Redis pub/sub for WebSocket communication across instances
type PubSubManager struct {
	redisClient   *redis.Client
	hub           *models.Hub
	logger        *zap.Logger
	subscriptions map[string]*redis.PubSub // channel -> subscription
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	instanceID    string // Unique identifier for this server instance
}

// Channel naming conventions for pub/sub
const (
	// User channels for direct messages and notifications
	UserChannelPrefix = "user:"

	// Chat channels for group messages
	ChatChannelPrefix = "chat:"

	// System channels for presence and metadata
	PresenceChannel = "presence:global"
	SystemChannel   = "system:global"

	// Instance-specific channels for internal coordination
	InstanceChannelPrefix = "instance:"
)

// NewPubSubManager creates a new pub/sub manager
func NewPubSubManager(redisClient *redis.Client, hub *models.Hub, logger *zap.Logger, instanceID string) *PubSubManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &PubSubManager{
		redisClient:   redisClient,
		hub:           hub,
		logger:        logger,
		subscriptions: make(map[string]*redis.PubSub),
		ctx:           ctx,
		cancel:        cancel,
		instanceID:    instanceID,
	}
}

// Start initializes the pub/sub manager and subscribes to system channels
func (pm *PubSubManager) Start() error {
	pm.logger.Info("pubsub_manager.go: Starting PubSub manager", zap.String("instanceID", pm.instanceID))

	// Subscribe to global system channels
	if err := pm.subscribeToChannel(PresenceChannel); err != nil {
		return fmt.Errorf("pubsub_manager.go: failed to subscribe to presence channel: %w", err)
	}

	if err := pm.subscribeToChannel(SystemChannel); err != nil {
		return fmt.Errorf("pubsub_manager.go: failed to subscribe to system channel: %w", err)
	}

	// Subscribe to instance-specific channel for targeted messages
	instanceChannel := InstanceChannelPrefix + pm.instanceID
	if err := pm.subscribeToChannel(instanceChannel); err != nil {
		return fmt.Errorf("pubsub_manager.go: failed to subscribe to instance channel: %w", err)
	}

	// Start message processing goroutines
	go pm.processMessages()

	return nil
}

// Stop gracefully shuts down the pub/sub manager
func (pm *PubSubManager) Stop() {
	pm.logger.Info("pubsub_manager.go: Stopping PubSub manager")

	pm.cancel()

	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Close all subscriptions
	for channel, sub := range pm.subscriptions {
		if err := sub.Close(); err != nil {
			pm.logger.Error("pubsub_manager.go: Failed to close subscription",
				zap.String("channel", channel),
				zap.Error(err))
		}
	}

	pm.subscriptions = make(map[string]*redis.PubSub)
}

// SubscribeUserToChannels subscribes a user to their personal and chat channels
func (pm *PubSubManager) SubscribeUserToChannels(userID bson.ObjectID, chatIDs []string) error {
	// Subscribe to user's personal channel for direct messages
	userChannel := pm.getUserChannel(userID.Hex())
	if err := pm.subscribeToChannel(userChannel); err != nil {
		return fmt.Errorf("pubsub_manager.go: failed to subscribe to user channel: %w", err)
	}

	// Subscribe to all chat channels the user is part of
	for _, chatID := range chatIDs {
		chatChannel := pm.getChatChannel(chatID)
		if err := pm.subscribeToChannel(chatChannel); err != nil {
			pm.logger.Error("pubsub_manager.go: Failed to subscribe to chat channel",
				zap.String("chatID", chatID),
				zap.Error(err))
		}
	}

	return nil
}

// UnsubscribeUserFromChannels unsubscribes a user from their channels
func (pm *PubSubManager) UnsubscribeUserFromChannels(userID bson.ObjectID, chatIDs []string) error {
	// Unsubscribe from user's personal channel
	userChannel := pm.getUserChannel(userID.Hex())
	if err := pm.unsubscribeFromChannel(userChannel); err != nil {
		pm.logger.Error("pubsub_manager.go: Failed to unsubscribe from user channel",
			zap.String("userID", userID.Hex()),
			zap.Error(err))
	}

	// Unsubscribe from chat channels
	for _, chatID := range chatIDs {
		chatChannel := pm.getChatChannel(chatID)
		if err := pm.unsubscribeFromChannel(chatChannel); err != nil {
			pm.logger.Error("pubsub_manager.go: Failed to unsubscribe from chat channel",
				zap.String("chatID", chatID),
				zap.Error(err))
		}
	}

	return nil
}

// PublishToChat publishes a message to a chat channel
func (pm *PubSubManager) PublishToChat(chatID string, message *models.WebSocketMessage) error {
	channel := pm.getChatChannel(chatID)
	return pm.publishMessage(channel, message)
}

// PublishToUser publishes a message to a specific user's channel
func (pm *PubSubManager) PublishToUser(userID string, message *models.WebSocketMessage) error {
	channel := pm.getUserChannel(userID)
	return pm.publishMessage(channel, message)
}

// PublishPresenceUpdate publishes presence updates to the global presence channel
func (pm *PubSubManager) PublishPresenceUpdate(status *models.WSPresenceStatus) error {
	message := &models.WebSocketMessage{
		Type:      models.WSMessageTypePresence,
		UserID:    status.UserID,
		Data:      status,
		Timestamp: time.Now(),
	}

	return pm.publishMessage(PresenceChannel, message)
}

// BroadcastSystemMessage broadcasts a system message to all instances
func (pm *PubSubManager) BroadcastSystemMessage(message *models.WebSocketMessage) error {
	return pm.publishMessage(SystemChannel, message)
}

// subscribeToChannel creates or reuses a subscription to a Redis channel
func (pm *PubSubManager) subscribeToChannel(channel string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if already subscribed
	if _, exists := pm.subscriptions[channel]; exists {
		return nil
	}

	// Create new subscription
	pubsub := pm.redisClient.Subscribe(pm.ctx, channel)

	// Wait for subscription confirmation
	_, err := pubsub.Receive(pm.ctx)
	if err != nil {
		return fmt.Errorf("pubsub_manager.go: failed to confirm subscription: %w", err)
	}

	pm.subscriptions[channel] = pubsub
	pm.logger.Debug("pubsub_manager.go: Subscribed to channel", zap.String("channel", channel))

	return nil
}

// unsubscribeFromChannel removes a subscription from a Redis channel
func (pm *PubSubManager) unsubscribeFromChannel(channel string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pubsub, exists := pm.subscriptions[channel]
	if !exists {
		return nil
	}

	if err := pubsub.Close(); err != nil {
		return fmt.Errorf("pubsub_manager.go: failed to close subscription: %w", err)
	}

	delete(pm.subscriptions, channel)
	pm.logger.Debug("pubsub_manager.go: Unsubscribed from channel", zap.String("channel", channel))

	return nil
}

// publishMessage publishes a message to a Redis channel
func (pm *PubSubManager) publishMessage(channel string, message *models.WebSocketMessage) error {
	// Add instance ID to message metadata for tracking
	if message.Data == nil {
		message.Data = make(map[string]interface{})
	}

	if metadata, ok := message.Data.(map[string]interface{}); ok {
		metadata["instanceID"] = pm.instanceID
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("pubsub_manager.go: failed to marshal message: %w", err)
	}

	if err := pm.redisClient.Publish(pm.ctx, channel, data).Err(); err != nil {
		return fmt.Errorf("pubsub_manager.go: failed to publish message: %w", err)
	}

	pm.logger.Debug("pubsub_manager.go: Published message",
		zap.String("channel", channel),
		zap.String("type", message.Type))

	return nil
}

// processMessages processes incoming messages from all subscriptions
func (pm *PubSubManager) processMessages() {
	for {
		select {
		case <-pm.ctx.Done():
			return
		default:
			pm.mu.RLock()
			subscriptions := make([]*redis.PubSub, 0, len(pm.subscriptions))
			for _, sub := range pm.subscriptions {
				subscriptions = append(subscriptions, sub)
			}
			pm.mu.RUnlock()

			for _, sub := range subscriptions {
				go pm.processSubscriptionMessages(sub)
			}

			// Small delay to prevent tight loop
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// processSubscriptionMessages handles messages from a specific subscription
func (pm *PubSubManager) processSubscriptionMessages(pubsub *redis.PubSub) {
	ch := pubsub.Channel()

	for msg := range ch {
		if msg.Payload == "" {
			continue
		}

		var wsMessage models.WebSocketMessage
		if err := json.Unmarshal([]byte(msg.Payload), &wsMessage); err != nil {
			pm.logger.Error("pubsub_manager.go: Failed to unmarshal message",
				zap.String("channel", msg.Channel),
				zap.Error(err))
			continue
		}

		// Route message based on channel type
		pm.routeMessage(msg.Channel, &wsMessage)
	}
}

// routeMessage routes messages to appropriate handlers based on channel type
func (pm *PubSubManager) routeMessage(channel string, message *models.WebSocketMessage) {
	pm.logger.Debug("pubsub_manager.go: Routing message",
		zap.String("channel", channel),
		zap.String("type", message.Type))

	switch {
	case channel == PresenceChannel:
		pm.handlePresenceMessage(message)

	case channel == SystemChannel:
		pm.handleSystemMessage(message)

	case len(channel) > len(ChatChannelPrefix) && channel[:len(ChatChannelPrefix)] == ChatChannelPrefix:
		chatID := channel[len(ChatChannelPrefix):]
		pm.handleChatMessage(chatID, message)

	case len(channel) > len(UserChannelPrefix) && channel[:len(UserChannelPrefix)] == UserChannelPrefix:
		userID := channel[len(UserChannelPrefix):]
		pm.handleUserMessage(userID, message)

	case len(channel) > len(InstanceChannelPrefix) && channel[:len(InstanceChannelPrefix)] == InstanceChannelPrefix:
		pm.handleInstanceMessage(message)

	default:
		pm.logger.Warn("pubsub_manager.go: Unknown channel type", zap.String("channel", channel))
	}
}

// handleChatMessage handles messages for chat channels
func (pm *PubSubManager) handleChatMessage(chatID string, message *models.WebSocketMessage) {
	// Broadcast to all clients in this chat on this instance
	pm.hub.Mutex.RLock()
	clients, exists := pm.hub.ChatClients[chatID]
	pm.hub.Mutex.RUnlock()

	if !exists || len(clients) == 0 {
		return
	}

	// Send to all connected clients in this chat
	for _, client := range clients {
		select {
		case client.Send <- *message:
			pm.logger.Debug("pubsub_manager.go: Sent chat message to client",
				zap.String("clientID", client.ID),
				zap.String("chatID", chatID))
		default:
			// Client's send channel is full, close it
			pm.logger.Warn("pubsub_manager.go: Client send channel full, closing",
				zap.String("clientID", client.ID))
			close(client.Send)
		}
	}
}

// handleUserMessage handles direct messages for specific users
func (pm *PubSubManager) handleUserMessage(userID string, message *models.WebSocketMessage) {
	// Send to all connections of this user on this instance
	pm.hub.Mutex.RLock()
	clients, exists := pm.hub.UserClients[userID]
	pm.hub.Mutex.RUnlock()

	if !exists || len(clients) == 0 {
		return
	}

	for _, client := range clients {
		select {
		case client.Send <- *message:
			pm.logger.Debug("pubsub_manager.go: Sent user message to client",
				zap.String("clientID", client.ID),
				zap.String("userID", userID))
		default:
			pm.logger.Warn("pubsub_manager.go: Client send channel full, closing",
				zap.String("clientID", client.ID))
			close(client.Send)
		}
	}
}

// handlePresenceMessage handles global presence updates
func (pm *PubSubManager) handlePresenceMessage(message *models.WebSocketMessage) {
	// Update local presence cache
	if presenceData, ok := message.Data.(*models.WSPresenceStatus); ok {
		pm.updateLocalPresenceCache(presenceData)
	}

	// Broadcast to all connected clients on this instance
	pm.hub.Mutex.RLock()
	allClients := make([]*models.Client, 0)
	for _, clients := range pm.hub.Clients {
		allClients = append(allClients, clients)
	}
	pm.hub.Mutex.RUnlock()

	for _, client := range allClients {
		select {
		case client.Send <- *message:
		default:
			// Skip if channel is full
		}
	}
}

// handleSystemMessage handles system-wide messages
func (pm *PubSubManager) handleSystemMessage(message *models.WebSocketMessage) {
	pm.logger.Info("pubsub_manager.go: Received system message",
		zap.String("type", message.Type),
		zap.Any("data", message.Data))

	// Handle different system message types
	switch message.Type {
	case "shutdown":
		// Handle graceful shutdown
		pm.logger.Info("pubsub_manager.go: Received shutdown signal")

	case "reload_config":
		// Handle configuration reload
		pm.logger.Info("pubsub_manager.go: Reloading configuration")

	default:
		// Broadcast to all clients if needed
		pm.broadcastToAllClients(message)
	}
}

// handleInstanceMessage handles instance-specific messages
func (pm *PubSubManager) handleInstanceMessage(message *models.WebSocketMessage) {
	pm.logger.Debug("pubsub_manager.go: Received instance message",
		zap.String("type", message.Type))

	// Handle instance-specific operations
	// This could be used for targeted operations like connection migration
}

// updateLocalPresenceCache updates the local presence cache
func (pm *PubSubManager) updateLocalPresenceCache(presence *models.WSPresenceStatus) {
	// Update hub's user info cache
	pm.hub.Mutex.Lock()
	defer pm.hub.Mutex.Unlock()

	if userInfo, exists := pm.hub.UserInfoCache[presence.UserID]; exists {
		userInfo.Status = presence.Status
		userInfo.IsOnline = presence.Status != "offline"
		userInfo.LastSeen = presence.LastSeen
		pm.hub.CacheExpiry[presence.UserID] = time.Now().Add(5 * time.Minute)
	}
}

// broadcastToAllClients sends a message to all connected clients
func (pm *PubSubManager) broadcastToAllClients(message *models.WebSocketMessage) {
	pm.hub.Mutex.RLock()
	clients := make([]*models.Client, 0, len(pm.hub.Clients))
	for _, client := range pm.hub.Clients {
		clients = append(clients, client)
	}
	pm.hub.Mutex.RUnlock()

	for _, client := range clients {
		select {
		case client.Send <- *message:
		default:
			// Skip if channel is full
		}
	}
}

// Helper methods for channel naming
func (pm *PubSubManager) getUserChannel(userID string) string {
	return UserChannelPrefix + userID
}

func (pm *PubSubManager) getChatChannel(chatID string) string {
	return ChatChannelPrefix + chatID
}

// GetSubscribedChannels returns list of currently subscribed channels
func (pm *PubSubManager) GetSubscribedChannels() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	channels := make([]string, 0, len(pm.subscriptions))
	for channel := range pm.subscriptions {
		channels = append(channels, channel)
	}

	return channels
}

// IsSubscribed checks if subscribed to a specific channel
func (pm *PubSubManager) IsSubscribed(channel string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	_, exists := pm.subscriptions[channel]
	return exists
}
