package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
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

	// Test hooks, always nil in production. Fields, not package variables, so
	// a goroutine left over from one test can't race the next test (#124).
	// Tests set them before anything runs and never change them.
	//
	// afterEmptyCheck runs between a release's "room is empty" check and its
	// unsubscribe; afterRoomJoin runs between a join's room entry and its
	// subscribe.
	afterEmptyCheck func(chatID string)
	afterRoomJoin   func(chatID string)
	// beforeUserRelease runs between UnsubscribeUser's "no connection" check and
	// its unsubscribe; beforeUserSubscribe runs before SubscribeUserToChannels
	// subscribes the user channel; beforePresencePublish runs at the start of
	// PublishPresenceUpdate (#22).
	beforeUserRelease     func(userID bson.ObjectID)
	beforeUserSubscribe   func(userID bson.ObjectID)
	beforePresencePublish func()
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

	if err := pm.subscribeToChannel(SessionControlChannel); err != nil {
		return fmt.Errorf("failed to subscribe to session control channel: %w", err)
	}

	// Subscribe to instance-specific channel for targeted messages
	instanceChannel := InstanceChannelPrefix + pm.instanceID
	if err := pm.subscribeToChannel(instanceChannel); err != nil {
		return fmt.Errorf("failed to subscribe to instance channel: %w", err)
	}

	return nil
}

// SubscribeUserToChannels subscribes a new connection's user channel and chat
// channels. Call it after the connection is in the hub's maps. Disconnects run
// concurrently (#22): if the user's last connection closed before the subscribe,
// its release had nothing to release, so the user channel is checked again
// afterwards and released if unused.
func (pm *PubSubManager) SubscribeUserToChannels(userID bson.ObjectID, chatIDs []string) error {
	if pm.beforeUserSubscribe != nil {
		pm.beforeUserSubscribe(userID)
	}
	userChannel := pm.getUserChannel(userID.Hex())
	if err := pm.subscribeToChannel(userChannel); err != nil {
		return fmt.Errorf("failed to subscribe to user channel: %w", err)
	}
	if !pm.userInUse(userID.Hex()) {
		pm.UnsubscribeUser(userID)
	}

	pm.subscribeChats(chatIDs)
	return nil
}

// subscribeChats subscribes the channels of chats a connection has just
// entered the rooms of. Call it after the room entry, never before (#55). A
// removal that emptied a room before the subscribe had nothing to release, so
// the channels are checked again afterwards and released if unused (#123).
func (pm *PubSubManager) subscribeChats(chatIDs []string) {
	for _, chatID := range chatIDs {
		if pm.afterRoomJoin != nil {
			pm.afterRoomJoin(chatID)
		}
		if err := pm.subscribeToChannel(pm.getChatChannel(chatID)); err != nil {
			pm.logger.WithFields(logrus.Fields{"chatID": chatID}).WithError(err).Error("Failed to subscribe to chat channel")
		}
	}
	pm.releaseEmptyChats(chatIDs)
}

// UnsubscribeUser releases a user's personal channel. The hub calls it when
// the user's last connection on this instance closes, whether or not they
// followed any chat (#20). Registrations run concurrently (#22), so a user with
// an open connection keeps the channel: one that registers between the check and
// the unsubscribe is seen by the second check, which subscribes again. Messages
// published in that gap are missed (#43). No lock is held across a Redis call.
func (pm *PubSubManager) UnsubscribeUser(userID bson.ObjectID) {
	if pm.userInUse(userID.Hex()) {
		return
	}
	if pm.beforeUserRelease != nil {
		pm.beforeUserRelease(userID)
	}
	channel := pm.getUserChannel(userID.Hex())
	if err := pm.unsubscribeFromChannel(channel); err != nil {
		pm.logger.WithFields(logrus.Fields{"userID": userID.Hex()}).WithError(err).Error("Failed to unsubscribe from user channel")
	}
	if pm.userInUse(userID.Hex()) {
		if err := pm.subscribeToChannel(channel); err != nil {
			pm.logger.WithFields(logrus.Fields{"userID": userID.Hex()}).WithError(err).Error("Failed to subscribe again to user channel")
		}
	}
}

// releaseEmptyChats unsubscribes the channels of chats no connection on this
// instance is in. JoinChat adds a client to the room before it subscribes, so
// a join racing this release is either seen by the first check (the channel
// is kept) or subscribes again after the close; the second check re-subscribes
// for a join that slipped in between (#55). Messages published during that
// gap are missed (#43). No lock is held across a Redis call.
func (pm *PubSubManager) releaseEmptyChats(chatIDs []string) {
	for _, chatID := range chatIDs {
		if pm.roomInUse(chatID) {
			continue
		}
		if pm.afterEmptyCheck != nil {
			pm.afterEmptyCheck(chatID)
		}

		channel := pm.getChatChannel(chatID)
		if err := pm.unsubscribeFromChannel(channel); err != nil {
			pm.logger.WithFields(logrus.Fields{"chatID": chatID}).WithError(err).Error("Failed to unsubscribe from chat channel")
		}

		if pm.roomInUse(chatID) {
			if err := pm.subscribeToChannel(channel); err != nil {
				pm.logger.WithFields(logrus.Fields{"chatID": chatID}).WithError(err).Error("Failed to subscribe again to chat channel")
			}
		}
	}
}

// userInUse says whether the user has a connection in the hub's maps.
func (pm *PubSubManager) userInUse(userID string) bool {
	pm.hub.Mutex.RLock()
	defer pm.hub.Mutex.RUnlock()
	return len(pm.hub.UserClients[userID]) > 0
}

func (pm *PubSubManager) roomInUse(chatID string) bool {
	pm.hub.Mutex.RLock()
	defer pm.hub.Mutex.RUnlock()
	_, inUse := pm.hub.ChatClients[chatID]
	return inUse
}

// PublishToChat publishes a message to a chat channel
func (pm *PubSubManager) PublishToChat(chatID string, message *models.WebSocketMessage) error {
	channel := pm.getChatChannel(chatID)
	return pm.publishMessage(channel, message)
}

// PublishPresenceUpdate publishes presence updates to the global presence channel
func (pm *PubSubManager) PublishPresenceUpdate(status *models.WSPresenceStatus) error {
	if pm.beforePresencePublish != nil {
		pm.beforePresencePublish()
	}
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

		// Not a WebSocketMessage, and never forwarded to clients.
		if msg.Channel == SessionControlChannel {
			pm.handleSessionControl(msg.Payload)
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
	// Hub-internal, never forwarded to clients (#14).
	if message.Type == membersRemovedType {
		pm.handleMembersRemoved(chatID, message)
		return
	}

	// Broadcast to all clients in this chat on this instance
	pm.hub.Mutex.RLock()
	clients := slices.Collect(maps.Values(pm.hub.ChatClients[chatID]))
	pm.hub.Mutex.RUnlock()

	// Send to all connected clients in this chat
	for _, client := range clients {
		if !client.TrySend(*message) {
			pm.dropSlowClient(client)
		}
	}
}

// handleUserMessage handles direct messages for specific users
func (pm *PubSubManager) handleUserMessage(userID string, message *models.WebSocketMessage) {
	// Hub-internal, never forwarded to clients (#49).
	if message.Type == chatAddedType {
		pm.handleChatAdded(userID, message)
		return
	}

	// Send to all connections of this user on this instance
	pm.hub.Mutex.RLock()
	clients := slices.Collect(maps.Values(pm.hub.UserClients[userID]))
	pm.hub.Mutex.RUnlock()

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
	activeChats := userActiveChats(pm.hub, userID)
	pm.hub.Mutex.RUnlock()

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

	// Replace the entry, never change it: GetUserInfo callers read the old
	// pointer without the lock.
	if _, exists := pm.hub.UserInfoCache[userID]; exists {
		pm.hub.UserInfoCache[userID] = &user
	}
}

// broadcastToAllClients sends a message to all connected clients
func (pm *PubSubManager) broadcastToAllClients(message *models.WebSocketMessage) {
	pm.hub.Mutex.RLock()
	clients := slices.Collect(maps.Values(pm.hub.Clients))
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
