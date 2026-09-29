package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/models"
)

// PubSubManager handles Redis pub/sub for WebSocket communication across instances
type PubSubManager struct {
	redisClient   *redis.Client
	hub           *models.Hub
	logger        *logrus.Logger
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
func NewPubSubManager(redisClient *redis.Client, hub *models.Hub, logger *logrus.Logger, instanceID string) *PubSubManager {
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
	pm.logger.WithFields(logrus.Fields{"instanceID": pm.instanceID}).Info("Starting PubSub manager")

	// Subscribe to global system channels
	if err := pm.subscribeToChannel(PresenceChannel); err != nil {
		return fmt.Errorf("failed to subscribe to presence channel: %w", err)
	}

	if err := pm.subscribeToChannel(SystemChannel); err != nil {
		return fmt.Errorf("failed to subscribe to system channel: %w", err)
	}

	// Subscribe to instance-specific channel for targeted messages
	instanceChannel := InstanceChannelPrefix + pm.instanceID
	if err := pm.subscribeToChannel(instanceChannel); err != nil {
		return fmt.Errorf("failed to subscribe to instance channel: %w", err)
	}

	return nil
}

// SubscribeUserToChannels subscribes a user to their personal and chat channels
func (pm *PubSubManager) SubscribeUserToChannels(userID bson.ObjectID, chatIDs []string) error {
	// Subscribe to user's personal channel for direct messages
	userChannel := pm.getUserChannel(userID.Hex())
	if err := pm.subscribeToChannel(userChannel); err != nil {
		return fmt.Errorf("failed to subscribe to user channel: %w", err)
	}

	// Subscribe to all chat channels the user is part of
	for _, chatID := range chatIDs {
		chatChannel := pm.getChatChannel(chatID)
		if err := pm.subscribeToChannel(chatChannel); err != nil {
			pm.logger.WithFields(logrus.Fields{"chatID": chatID}).WithError(err).Error("Failed to subscribe to chat channel")
		}
	}

	return nil
}

// UnsubscribeUserFromChannels unsubscribes a user from their channels
func (pm *PubSubManager) UnsubscribeUserFromChannels(userID bson.ObjectID, chatIDs []string) error {
	// Unsubscribe from user's personal channel (always safe to unsubscribe personal channels)
	userChannel := pm.getUserChannel(userID.Hex())
	if err := pm.unsubscribeFromChannel(userChannel); err != nil {
		pm.logger.WithFields(logrus.Fields{"userID": userID.Hex()}).WithError(err).Error("Failed to unsubscribe from user channel")
	}

	// Unsubscribe from chat channels - but only if no other clients are in the chat
	for _, chatID := range chatIDs {
		chatChannel := pm.getChatChannel(chatID)

		// Check if there are other clients still in this chat on this instance
		pm.hub.Mutex.RLock()
		shouldUnsubscribe := true
		if chatClients, exists := pm.hub.ChatClients[chatID]; exists {
			// Count clients that are NOT from the disconnecting user
			otherClientsCount := 0
			for _, client := range chatClients {
				if client.UserID != userID {
					otherClientsCount++
				}
			}
			// Only unsubscribe if no other clients from different users are in this chat
			shouldUnsubscribe = (otherClientsCount == 0)
		}
		pm.hub.Mutex.RUnlock()

		// Only unsubscribe if safe to do so
		if shouldUnsubscribe {
			if err := pm.unsubscribeFromChannel(chatChannel); err != nil {
				pm.logger.WithFields(logrus.Fields{"chatID": chatID}).WithError(err).Error("Failed to unsubscribe from chat channel")
			} else {
				pm.logger.WithFields(logrus.Fields{"chatID": chatID, "userID": userID.Hex()}).Debug("Unsubscribed from chat channel (no other clients)")
			}
		} else {
			pm.logger.WithFields(logrus.Fields{"chatID": chatID, "userID": userID.Hex()}).Debug("Keeping chat channel subscription (other clients exist)")
		}
	}

	return nil
}

// PublishToChat publishes a message to a chat channel
func (pm *PubSubManager) PublishToChat(chatID string, message *models.WebSocketMessage) error {
	channel := pm.getChatChannel(chatID)
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
		return fmt.Errorf("failed to confirm subscription: %w", err)
	}

	pm.subscriptions[channel] = pubsub

	// One reader per subscription; it exits when the subscription is closed.
	go pm.processSubscriptionMessages(pubsub)
	pm.logger.WithFields(logrus.Fields{"channel": channel}).Debug("Subscribed to channel")

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
		return fmt.Errorf("failed to close subscription: %w", err)
	}

	delete(pm.subscriptions, channel)
	pm.logger.WithFields(logrus.Fields{"channel": channel}).Debug("Unsubscribed from channel")

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
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	if err := pm.redisClient.Publish(pm.ctx, channel, data).Err(); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	pm.logger.WithFields(logrus.Fields{"channel": channel, "type": message.Type}).Debug("Published message")

	return nil
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
			pm.logger.WithFields(logrus.Fields{"channel": msg.Channel}).WithError(err).Error("Failed to unmarshal message")
			continue
		}

		// Route message based on channel type
		pm.routeMessage(msg.Channel, &wsMessage)
	}
}

// routeMessage routes messages to appropriate handlers based on channel type
func (pm *PubSubManager) routeMessage(channel string, message *models.WebSocketMessage) {
	pm.logger.WithFields(logrus.Fields{"channel": channel, "type": message.Type}).Debug("Routing message")

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
		pm.logger.WithFields(logrus.Fields{"channel": channel}).Warn("Unknown channel type")
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
		if !client.TrySend(*message) {
			pm.dropSlowClient(client)
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
		if client.TrySend(*message) {
			pm.logger.WithFields(logrus.Fields{"clientID": client.ID, "userID": userID}).Debug("Sent user message to client")
		} else {
			pm.dropSlowClient(client)
		}
	}
}

// dropSlowClient disconnects a client whose send buffer is full. Closing the
// channel is left to the hub's unregister path so it happens exactly once.
func (pm *PubSubManager) dropSlowClient(client *models.Client) {
	pm.logger.WithFields(logrus.Fields{"clientID": client.ID}).Warn("Client send channel full, disconnecting")
	client.CloseSend()
	select {
	case pm.hub.Unregister <- client:
	default:
		pm.logger.WithFields(logrus.Fields{"clientID": client.ID}).Warn("Unregister queue full; client will be removed by cleanup")
	}
}

// handlePresenceMessage handles global presence updates
func (pm *PubSubManager) handlePresenceMessage(message *models.WebSocketMessage) {
	userID := message.UserID

	pm.hub.Mutex.RLock()
	activeChats, exists := pm.hub.UserActiveChats[userID]
	pm.hub.Mutex.RUnlock()

	if !exists {
		return
	}

	for _, chatID := range activeChats {
		// we have to publish to the chat channel
		if err := pm.publishMessage(pm.getChatChannel(chatID), message); err != nil {
			pm.logger.WithFields(logrus.Fields{"chatID": chatID}).WithError(err).Error("Failed to publish message to chat channel")
		}
	}
}

// handleSystemMessage handles system-wide messages
func (pm *PubSubManager) handleSystemMessage(message *models.WebSocketMessage) {
	pm.logger.WithFields(logrus.Fields{"type": message.Type, "data": message.Data}).Info("Received system message")

	// Handle different system message types
	switch message.Type {
	case "shutdown":
		// Handle graceful shutdown
		pm.logger.Info("Received shutdown signal")

	case "reload_config":
		// Handle configuration reload
		pm.logger.Info("Reloading configuration")

	default:
		// Broadcast to all clients if needed
		pm.broadcastToAllClients(message)
	}
}

// handleInstanceMessage handles instance-specific messages
func (pm *PubSubManager) handleInstanceMessage(message *models.WebSocketMessage) {
	pm.logger.WithFields(logrus.Fields{"type": message.Type}).Debug("Received instance message")

	// Handle instance-specific operations
	// This could be used for targeted operations like connection migration
}

// UpdateUserInfoCache updates the user info cache
func (eh *Hub) UpdateUserInfoCache(userID string, user models.UserDisplayInfo) {
	eh.pubSubManager.updateUserInfoCache(userID, user)
}

func (pm *PubSubManager) updateUserInfoCache(userID string, user models.UserDisplayInfo) {
	// Update hub's user info cache
	pm.hub.Mutex.Lock()
	defer pm.hub.Mutex.Unlock()

	if userInfo, exists := pm.hub.UserInfoCache[userID]; exists {
		userInfo.Email = user.Email
		userInfo.Username = user.Username
		userInfo.DisplayName = user.DisplayName
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
		client.TrySend(*message) // skip if the channel is full
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
