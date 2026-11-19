package realtime

import (
	"context"
	"encoding/json"
	"errors"
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

// PubSubManager handles Redis pub/sub for WebSocket communication across
// instances. Every channel of an instance shares one Redis connection, read by
// one goroutine (#21): a connection per channel grew with online users and
// chats until Redis refused new ones.
type PubSubManager struct {
	redisClient *redis.Client
	hub         *models.Hub
	logger      *logrus.Logger
	// pubsub is the shared subscription, created by the first subscribe.
	pubsub *redis.PubSub
	// subscriptions holds the channels subscribed on pubsub. Each value is the
	// waiter of the subscribe that sent the command, closed once Redis confirmed
	// it (nil for channels tests mark without Redis).
	subscriptions map[string]chan struct{}
	mu            sync.RWMutex // guards pubsub and subscriptions; held while a command is written
	// pending holds, per channel, the waiters of subscribe commands Redis hasn't
	// confirmed yet, in the order the commands were sent; Redis confirms them in
	// that order. It has its own lock, never held across a Redis call, so the
	// reader never waits behind a slow write (#21).
	pending    map[string][]chan struct{}
	pendingMu  sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	instanceID string // Unique identifier for this server instance
	// confirmTimeout bounds the wait for a subscribe's confirmation:
	// redisSubscribeTimeout, shorter in tests.
	confirmTimeout time.Duration

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
	beforePresencePublish func(status string)
}

// redisSubscribeTimeout bounds the wait for Redis to confirm a subscribe (#22),
// and the write of a subscribe or unsubscribe command.
const redisSubscribeTimeout = 5 * time.Second

// pubsubBufferSize is how many messages of the shared connection go-redis
// queues for the reader. A message that waits more than a minute for room is
// dropped, and every channel shares this queue, so it is larger than
// go-redis's default of 100.
const pubsubBufferSize = 1000

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
		redisClient:    redisClient,
		hub:            hub,
		logger:         logger,
		subscriptions:  make(map[string]chan struct{}),
		pending:        make(map[string][]chan struct{}),
		confirmTimeout: redisSubscribeTimeout,
		ctx:            ctx,
		cancel:         cancel,
		instanceID:     instanceID,
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

	if err := pm.subscribeToChannel(ProfileControlChannel); err != nil {
		return fmt.Errorf("failed to subscribe to profile control channel: %w", err)
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

// userInUse says whether the user has a connection registered, or queued for
// registration (#22).
func (pm *PubSubManager) userInUse(userID string) bool {
	pm.hub.Mutex.RLock()
	defer pm.hub.Mutex.RUnlock()
	return userInUseLocked(pm.hub, userID)
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

// PublishPresenceUpdate publishes a presence change on presence:global. Every
// server delivers it to its own connections in status.Chats (#31).
func (pm *PubSubManager) PublishPresenceUpdate(status *models.WSPresenceStatus) error {
	if pm.beforePresencePublish != nil {
		pm.beforePresencePublish(status.Status)
	}
	message := &models.WebSocketMessage{
		Type:      models.WSMessageTypePresence,
		UserID:    status.UserID,
		Data:      status,
		Timestamp: time.Now(),
	}

	return pm.publishMessage(PresenceChannel, message)
}

// subscribeToChannel subscribes the shared connection to a Redis channel and
// returns once Redis has confirmed it, so a message published afterwards is
// delivered. The command is written under pm.mu, which keeps the subscribes and
// unsubscribes of a channel in order. The wait for the confirmation runs after
// the lock is released, so one slow reply no longer holds up every other
// subscribe (#21).
func (pm *PubSubManager) subscribeToChannel(channel string) error {
	pm.mu.Lock()
	if confirmed, exists := pm.subscriptions[channel]; exists {
		// Already sent: wait for its confirmation if it hasn't come yet. If it
		// doesn't come, the subscribe that sent it releases the channel.
		pm.mu.Unlock()
		if confirmed == nil {
			return nil
		}
		return pm.awaitConfirmation(confirmed)
	}

	ps := pm.sharedPubSubLocked()
	confirmed := make(chan struct{})
	pm.addPending(channel, confirmed)

	ctx, cancel := context.WithTimeout(pm.ctx, redisSubscribeTimeout)
	err := ps.Subscribe(ctx, channel)
	cancel()
	if err != nil {
		// go-redis keeps the channel for its next reconnect even when the write
		// fails. Drop it, so Redis and pm.subscriptions agree.
		pm.unsubscribeLocked(channel)
		pm.dropPending(channel, confirmed)
	} else {
		pm.subscriptions[channel] = confirmed
	}
	pm.mu.Unlock()

	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}
	if err := pm.awaitConfirmation(confirmed); err != nil {
		pm.abandonSubscribe(channel, confirmed)
		return err
	}
	pm.logger.WithFields(logrus.Fields{"channel": channel}).Debug("Subscribed to channel")
	return nil
}

// abandonSubscribe undoes a subscribe Redis didn't confirm in time (lost on a
// reconnect, dropped by a stuck reader, or refused by Redis): its waiter
// leaves the queue, so it can't swallow a later confirmation, and the channel
// is released unless another subscribe has replaced it. The next subscribe
// sends the command again.
func (pm *PubSubManager) abandonSubscribe(channel string, confirmed chan struct{}) {
	pm.dropPending(channel, confirmed)
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.subscriptions[channel] == confirmed {
		delete(pm.subscriptions, channel)
		pm.unsubscribeLocked(channel)
	}
}

// unsubscribeLocked sends an unsubscribe with a fresh timeout and logs a
// failure. go-redis forgets the channel even if the write fails, so it isn't
// subscribed again on the next reconnect. Call it with pm.mu held.
func (pm *PubSubManager) unsubscribeLocked(channel string) {
	ctx, cancel := context.WithTimeout(pm.ctx, redisSubscribeTimeout)
	defer cancel()
	if err := pm.pubsub.Unsubscribe(ctx, channel); err != nil {
		pm.logger.WithFields(logrus.Fields{"channel": channel}).WithError(err).Warn("Failed to send unsubscribe")
	}
}

// awaitConfirmation waits at most pm.confirmTimeout for Redis to confirm a
// subscribe.
func (pm *PubSubManager) awaitConfirmation(confirmed <-chan struct{}) error {
	timer := time.NewTimer(pm.confirmTimeout)
	defer timer.Stop()
	select {
	case <-confirmed:
		return nil
	case <-timer.C:
		return errors.New("failed to confirm subscription: timed out")
	case <-pm.ctx.Done():
		return fmt.Errorf("failed to confirm subscription: %w", pm.ctx.Err())
	}
}

// sharedPubSubLocked returns the shared subscription, creating it and its
// reader on first use. Call it with pm.mu held.
func (pm *PubSubManager) sharedPubSubLocked() *redis.PubSub {
	if pm.pubsub != nil {
		return pm.pubsub
	}
	ps := pm.redisClient.Subscribe(pm.ctx)
	pm.pubsub = ps
	go pm.processSubscriptionMessages(ps.ChannelWithSubscriptions(redis.WithChannelSize(pubsubBufferSize)))
	// Tests cancel pm.ctx to close the connection; the server closes it by
	// exiting.
	go func() {
		<-pm.ctx.Done()
		_ = ps.Close() // ends the reader
	}()
	return ps
}

// addPending queues a waiter for a subscribe command about to be sent.
func (pm *PubSubManager) addPending(channel string, confirmed chan struct{}) {
	pm.pendingMu.Lock()
	defer pm.pendingMu.Unlock()
	pm.pending[channel] = append(pm.pending[channel], confirmed)
}

// dropPending removes one waiter from a channel's queue.
func (pm *PubSubManager) dropPending(channel string, confirmed chan struct{}) {
	pm.pendingMu.Lock()
	defer pm.pendingMu.Unlock()
	queue := slices.DeleteFunc(pm.pending[channel], func(c chan struct{}) bool { return c == confirmed })
	if len(queue) == 0 {
		delete(pm.pending, channel)
		return
	}
	pm.pending[channel] = queue
}

// confirmSubscribe releases the oldest waiter of a channel. A confirmation
// with no waiter comes from go-redis subscribing again after a reconnect.
// After a reconnect, such a confirmation can also release a subscribe sent
// just after it before Redis has run that command; a message published in
// that moment is missed, like the other short gaps of #43.
func (pm *PubSubManager) confirmSubscribe(channel string) {
	pm.pendingMu.Lock()
	defer pm.pendingMu.Unlock()
	queue := pm.pending[channel]
	if len(queue) == 0 {
		return
	}
	close(queue[0])
	if len(queue) == 1 {
		delete(pm.pending, channel)
		return
	}
	pm.pending[channel] = queue[1:]
}

// unsubscribeFromChannel unsubscribes the shared connection from a Redis
// channel. It doesn't wait for Redis to confirm: a message of the channel that
// is already on its way still arrives, and goes to whoever is in the room.
func (pm *PubSubManager) unsubscribeFromChannel(channel string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.subscriptions[channel]; !exists {
		return nil
	}
	delete(pm.subscriptions, channel)
	if pm.pubsub == nil {
		// Nothing was ever sent to Redis: a subscribe creates pubsub before it
		// records a channel. Only tests mark channels without one.
		return nil
	}

	ctx, cancel := context.WithTimeout(pm.ctx, redisSubscribeTimeout)
	defer cancel()
	// go-redis forgets the channel even if the write fails, so it isn't
	// subscribed again on the next reconnect.
	if err := pm.pubsub.Unsubscribe(ctx, channel); err != nil {
		return fmt.Errorf("failed to unsubscribe: %w", err)
	}
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

// processSubscriptionMessages reads the shared connection: subscribe
// confirmations and the messages of every channel, in the order Redis sent
// them. It runs until the subscription is closed. Handlers must not block: a
// slow one holds up every channel.
func (pm *PubSubManager) processSubscriptionMessages(items <-chan any) {
	for item := range items {
		switch v := item.(type) {
		case *redis.Subscription:
			if v.Kind == "subscribe" {
				pm.confirmSubscribe(v.Channel)
			}
		case *redis.Message:
			pm.handleRedisMessage(v)
		}
	}
}

// handleRedisMessage decodes one message and routes it by its channel.
func (pm *PubSubManager) handleRedisMessage(msg *redis.Message) {
	if msg.Payload == "" {
		return
	}

	// Not a WebSocketMessage, and never forwarded to clients.
	if msg.Channel == SessionControlChannel {
		pm.handleSessionControl(msg.Payload)
		return
	}
	if msg.Channel == ProfileControlChannel {
		pm.handleProfileControl(msg.Payload)
		return
	}

	var wsMessage models.WebSocketMessage
	if err := json.Unmarshal([]byte(msg.Payload), &wsMessage); err != nil {
		pm.logger.WithFields(logrus.Fields{"channel": msg.Channel}).WithError(err).Error("Failed to unmarshal message")
		return
	}

	// Route message based on channel type
	pm.routeMessage(msg.Channel, &wsMessage)
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

// handlePresenceMessage delivers a presence change to this server's
// connections in the chats it lists, once per connection, without the list
// (#31). An event without chats is ignored: only a server from before #31
// publishes those, during a deploy.
func (pm *PubSubManager) handlePresenceMessage(message *models.WebSocketMessage) {
	data, ok := message.Data.(map[string]any)
	if !ok {
		return
	}
	rawChats, _ := data["chats"].([]any)
	if len(rawChats) == 0 {
		return
	}
	// A user who shows their status to contacts only: deliver to those users
	// alone. An empty audience reaches nobody (#32).
	contactsOnly, _ := data["contactsOnly"].(bool)
	audience := make(map[string]bool)
	if rawAudience, ok := data["audience"].([]any); ok {
		for _, item := range rawAudience {
			if id, ok := item.(string); ok {
				audience[id] = true
			}
		}
	}
	out := make(map[string]any, len(data))
	for key, value := range data {
		if key != "chats" && key != "contactsOnly" && key != "audience" {
			out[key] = value
		}
	}
	event := *message
	event.Data = out

	recipients := make(map[string]*models.Client)
	pm.hub.Mutex.RLock()
	for _, item := range rawChats {
		if chatID, ok := item.(string); ok {
			maps.Copy(recipients, pm.hub.ChatClients[chatID])
		}
	}
	pm.hub.Mutex.RUnlock()

	for _, client := range recipients {
		// The user's own other connections get it too: they aren't in their
		// own contacts.
		if contactsOnly && !audience[client.UserID.Hex()] && client.UserID.Hex() != data["userId"] {
			continue
		}
		if !client.TrySend(event) {
			pm.dropSlowClient(client)
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
