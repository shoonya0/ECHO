package realtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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
	// checkSession checks open connections; nil means services.CheckSession.
	checkSession SessionChecker
	// loadChats lists the chats a new connection joins; nil means getUserChats.
	loadChats func(ctx context.Context, userID bson.ObjectID) ([]string, error)
	// loadMembers reads chat participants for the membership re-check; nil
	// means services.ChatMembers.
	loadMembers MembersLoader

	// restartMu serializes Restart, so connections that all find the context
	// cancelled start the background loops once (#79).
	restartMu sync.Mutex
	// loopsRunning counts the background loops startLoops started that
	// haven't returned; tests read it.
	loopsRunning atomic.Int32
}

// GetHubInstance returns the process-wide hub, creating it on first use.
// database.ConnectRedis must have succeeded before the first call.
func GetHubInstance() *Hub {
	hubOnce.Do(func() {
		instanceID := fmt.Sprintf("instance-%s-%d", uuid.New().String()[:8], time.Now().Unix())

		baseHub := &models.Hub{
			Clients:       make(map[string]*models.Client),
			ChatClients:   make(map[string]map[string]*models.Client),
			UserClients:   make(map[string]map[string]*models.Client),
			UserInfoCache: make(map[string]*models.UserDisplayInfo),
			CacheExpiry:   make(map[string]time.Time),
			Register:      make(chan *models.Client, 1000),
			Unregister:    make(chan *models.Client, 1000),
			ClientChats:   make(map[string]map[string]struct{}),
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

	eh.startLoops()

	eh.logger.Info("Enhanced WebSocket hub started successfully")
	return nil
}

// Restart starts the background loops again on a new context once the hub's
// context has been cancelled. While the context is live the loops are still
// running, so it does nothing (#79).
func (eh *Hub) Restart() error {
	eh.restartMu.Lock()
	defer eh.restartMu.Unlock()

	select {
	case <-eh.Ctx.Done():
	default:
		eh.logger.Info("Hub context is still active, not restarting")
		return nil
	}

	eh.logger.Warn("Hub context is cancelled, restarting with a new context")
	eh.Ctx, eh.Cancel = context.WithCancel(context.Background())
	eh.startLoops()

	eh.logger.Info("Hub restarted successfully")
	return nil
}

// startLoops starts the hub's background loops on the current context. Each
// loop keeps that context, so a Restart that swaps in a new one can't keep an
// old loop running.
func (eh *Hub) startLoops() {
	ctx := eh.Ctx
	loops := []func(context.Context){
		eh.runHub,
		eh.cleanupInactiveConnections,
		eh.syncPresenceWithRedis,
		eh.recheckSessions,
	}
	for _, loop := range loops {
		eh.loopsRunning.Add(1)
		go func() {
			defer eh.loopsRunning.Add(-1)
			loop(ctx)
		}()
	}
}

// runHub manages the main hub operations
func (eh *Hub) runHub(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			eh.logger.Warn("Hub context cancelled, stopping main loop")
			return

		case client := <-eh.Register:
			eh.handleClientRegistration(client)

		case client := <-eh.Unregister:
			eh.handleClientUnregistration(client)
		}
	}
}

// WaitRegistered waits until the hub has handled the client's registration.
// The controller reads a connection's requests only after it, so a request such
// as join_chat never runs before the connection's rooms are set (#122).
func (eh *Hub) WaitRegistered(ctx context.Context, client *models.Client) error {
	select {
	case <-client.Registered():
		return nil
	case <-eh.Ctx.Done():
		return errors.New("hub stopped before registering the connection")
	case <-ctx.Done():
		return fmt.Errorf("waiting for registration: %w", ctx.Err())
	}
}

// handleClientRegistration handles new client connections with pub/sub
func (eh *Hub) handleClientRegistration(client *models.Client) {
	defer client.MarkRegistered()
	eh.logger.WithFields(logrus.Fields{"clientID": client.ID, "userID": client.UserID.Hex()}).Info("Registering new client")

	// Get user's chats from database BEFORE acquiring mutex (avoid deadlock!)
	ctx := context.Background()
	loadChats := eh.loadChats
	if loadChats == nil {
		loadChats = eh.getUserChats
	}
	userChats, err := loadChats(ctx, client.UserID)
	if err != nil {
		eh.logger.WithError(err).Error("Failed to get user chats")
		userChats = []string{}
	}

	eh.Mutex.Lock()

	// The disconnect can be handled first: Register and Unregister are separate
	// channels, and select picks either. It closed Send, so a closed client is
	// gone and must not be registered (#54).
	if client.IsClosed() {
		eh.Mutex.Unlock()
		eh.logger.WithFields(logrus.Fields{"clientID": client.ID}).Info("Client closed before registration, not registering")
		return
	}

	// Add client to hub
	eh.Clients[client.ID] = client

	// Add to user mapping
	userID := client.UserID.Hex()
	if eh.UserClients[userID] == nil {
		eh.UserClients[userID] = make(map[string]*models.Client)
	}
	eh.UserClients[userID][client.ID] = client

	// Add client to all their chat rooms. Rooms are kept per connection, so
	// this never changes the rooms of the user's other connections (#20).
	for _, chatID := range userChats {
		addClientToRoom(eh.Hub, client, chatID)
		eh.logger.WithFields(logrus.Fields{"clientID": client.ID, "chatID": chatID}).Debug("Added client to chat")
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

	// Subscribe after joining the rooms, never before (#55, #123): see
	// subscribeChats.
	if err := eh.pubSubManager.SubscribeUserToChannels(client.UserID, userChats); err != nil {
		eh.logger.WithError(err).Error("Failed to subscribe to channels")
	}

	// Off the hub goroutine, so registrations don't wait on Redis (#22, #76).
	go eh.checkNewSession(client)

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

	eh.Mutex.Lock()

	if _, ok := eh.Clients[client.ID]; !ok {
		_, userConnected := eh.UserClients[client.UserID.Hex()]
		eh.Mutex.Unlock()
		// Not registered yet, or already removed. Closing Send marks it
		// closed, so a registration still queued skips it (#54).
		client.CloseSend()
		// The controller stored presence for this connection before queueing
		// it; with the registration skipped, nothing else removes it.
		if !userConnected {
			GetPresenceInstance().DeleteForClient(client.UserID, client.ID)
		}
		return
	}

	clientID := client.ID
	userID := client.UserID.Hex()

	// Leave every room this connection is in. Rooms left empty are no longer
	// needed on this instance, so their channels are released below (#20).
	// Deleting from a map while ranging over it is safe in Go (unlike the
	// slice this replaced, #116).
	var emptyRooms []string
	for chatID := range eh.ClientChats[clientID] {
		if removeClientFromRoom(eh.Hub, client, chatID) {
			emptyRooms = append(emptyRooms, chatID)
		}
	}

	delete(eh.Clients, clientID)

	lastConnection := false
	if userClients, exists := eh.UserClients[userID]; exists {
		delete(userClients, clientID)
		if len(userClients) == 0 {
			lastConnection = true
			delete(eh.UserClients, userID)
		}
	}

	eh.Mutex.Unlock()

	// Redis calls run after unlocking; the release takes the lock itself
	eh.pubSubManager.releaseEmptyChats(emptyRooms)

	if lastConnection {
		eh.pubSubManager.UnsubscribeUser(client.UserID)

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

	// Close send channel
	client.CloseSend()
}

// JoinChat adds the connection to the chat's room, subscribes the chat and
// announces the join. A connection already in the room, or already closed, is
// left as it is. A removal during the join undoes it and returns ErrNotInChat.
func (eh *Hub) JoinChat(ctx context.Context, client *models.Client, chatID string) error {
	eh.logger.WithFields(logrus.Fields{"clientID": client.ID, "chatID": chatID}).Info("Client joining chat")

	eh.Mutex.Lock()
	// A join handled after the connection closed would put it back in a room
	// that nothing removes it from again.
	if _, open := eh.Clients[client.ID]; !open {
		eh.Mutex.Unlock()
		return nil
	}
	// Already following: announcing again would let join_chat flood the chat
	// with join events (#48).
	if _, already := eh.ChatClients[chatID][client.ID]; already {
		eh.Mutex.Unlock()
		return nil
	}
	addClientToRoom(eh.Hub, client, chatID)
	eh.Mutex.Unlock()

	eh.pubSubManager.subscribeChats([]string{chatID})

	// A removal during the join took the connection out again: announcing the
	// join would tell the chat a removed member joined (#123).
	eh.Mutex.RLock()
	_, stillIn := eh.ChatClients[chatID][client.ID]
	eh.Mutex.RUnlock()
	if !stillIn {
		return fmt.Errorf("removed during the join: %w", ErrNotInChat)
	}

	// Publish join event to chat
	joinMsg := &models.WebSocketMessage{
		Type:     models.WSMessageTypeJoin,
		ChatID:   chatID,
		UserID:   client.UserID.Hex(),
		Username: client.Username,
		Avatar:   client.Avatar,
		Data: models.UserJoinLeave{
			UserID:    client.UserID.Hex(),
			Username:  client.Username,
			Avatar:    client.Avatar,
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

	// Only a client in the room may announce a leave, so nobody can spoof one.
	// A member removed from a group has already left the room (#14), so their
	// leave gets ErrNotInChat.
	if _, inRoom := eh.ChatClients[chatID][client.ID]; !inRoom {
		eh.Mutex.Unlock()
		return ErrNotInChat
	}

	emptied := removeClientFromRoom(eh.Hub, client, chatID)
	eh.Mutex.Unlock()

	// Publish leave event
	leaveMsg := &models.WebSocketMessage{
		Type:     models.WSMessageTypeLeave,
		ChatID:   chatID,
		UserID:   client.UserID.Hex(),
		Username: client.Username,
		Avatar:   client.Avatar,
		Data: models.UserJoinLeave{
			UserID:    client.UserID.Hex(),
			Username:  client.Username,
			Avatar:    client.Avatar,
			Action:    "leave",
			Timestamp: time.Now(),
		},
		Timestamp: time.Now(),
	}

	err := eh.pubSubManager.PublishToChat(chatID, leaveMsg)
	if emptied {
		eh.pubSubManager.releaseEmptyChats([]string{chatID})
	}
	return err
}

// ErrNotInChat means the connection isn't following the chat it tried to leave,
// or a removal undid its join.
var ErrNotInChat = errors.New("client is not in this chat")

// addClientToRoom puts a connection in a chat's room. Doing it twice changes
// nothing. The caller holds h.Mutex.
func addClientToRoom(h *models.Hub, client *models.Client, chatID string) {
	if h.ChatClients[chatID] == nil {
		h.ChatClients[chatID] = make(map[string]*models.Client)
	}
	h.ChatClients[chatID][client.ID] = client

	if h.ClientChats[client.ID] == nil {
		h.ClientChats[client.ID] = make(map[string]struct{})
	}
	h.ClientChats[client.ID][chatID] = struct{}{}
}

// removeClientFromRoom takes a connection out of a chat's room. The user's
// other connections keep their rooms. It reports whether the room is now
// empty, so the caller can release the chat's channel after unlocking with
// releaseEmptyChats. The caller holds h.Mutex.
func removeClientFromRoom(h *models.Hub, client *models.Client, chatID string) (emptied bool) {
	if chatClients, exists := h.ChatClients[chatID]; exists {
		delete(chatClients, client.ID)
		if len(chatClients) == 0 {
			delete(h.ChatClients, chatID)
			emptied = true
		}
	}

	if chats, exists := h.ClientChats[client.ID]; exists {
		delete(chats, chatID)
		if len(chats) == 0 {
			delete(h.ClientChats, client.ID)
		}
	}
	return emptied
}

// userActiveChats lists the chats any of the user's connections on this
// instance is in. The caller holds h.Mutex (read or write).
func userActiveChats(h *models.Hub, userID string) []string {
	seen := make(map[string]struct{})
	var chats []string
	for clientID := range h.UserClients[userID] {
		for chatID := range h.ClientChats[clientID] {
			if _, dup := seen[chatID]; !dup {
				seen[chatID] = struct{}{}
				chats = append(chats, chatID)
			}
		}
	}
	return chats
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
func (eh *Hub) syncPresenceWithRedis(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
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
func (eh *Hub) cleanupInactiveConnections(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			eh.performCleanup()
		}
	}
}

// performCleanup removes inactive clients
func (eh *Hub) performCleanup() {
	eh.Mutex.Lock()

	now := time.Now()
	inactiveThreshold := 5 * time.Minute
	toRemove := make([]*models.Client, 0)

	for _, client := range eh.Clients {
		if now.Sub(client.LastActivity()) > inactiveThreshold {
			toRemove = append(toRemove, client)
		}
	}

	// Clean expired cache entries
	for userID, expiry := range eh.CacheExpiry {
		if now.After(expiry) {
			delete(eh.UserInfoCache, userID)
			delete(eh.CacheExpiry, userID)
		}
	}

	eh.Mutex.Unlock()

	// Queue after unlocking: the consumer of Unregister needs the lock, so
	// waiting on a full queue while holding it would deadlock.
	for _, client := range toRemove {
		eh.logger.WithFields(logrus.Fields{"clientID": client.ID}).Info("Removing inactive client")
		select {
		case eh.Unregister <- client:
		case <-eh.Ctx.Done():
			return
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

// HubStats is what GET chats/hub/stats returns: counts for this instance.
type HubStats struct {
	InstanceID         string    `json:"instanceId"`
	TotalClients       int       `json:"totalClients"`
	TotalChats         int       `json:"totalChats"`
	TotalUsers         int       `json:"totalUsers"`
	CacheSize          int       `json:"cacheSize"`
	SubscribedChannels int       `json:"subscribedChannels"`
	Timestamp          time.Time `json:"timestamp"`
}

// GetStats returns counts for this hub. It never lists channel names: they are
// user:<id> and chat:<id>, so they would show who is online and which chats
// are active (#24).
func (eh *Hub) GetStats() HubStats {
	// Read the pub/sub count first, so the two locks are never held together.
	subscribedChannels := len(eh.pubSubManager.GetSubscribedChannels())

	eh.Mutex.RLock()
	defer eh.Mutex.RUnlock()

	return HubStats{
		InstanceID:         eh.instanceID,
		TotalClients:       len(eh.Clients),
		TotalChats:         len(eh.ChatClients),
		TotalUsers:         len(eh.UserClients),
		CacheSize:          len(eh.UserInfoCache),
		SubscribedChannels: subscribedChannels,
		Timestamp:          time.Now(),
	}
}
