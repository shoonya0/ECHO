package realtime

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
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
	// saveLastSeen stores when a user's last connection closed; nil means
	// services.RecordLastSeen.
	saveLastSeen func(ctx context.Context, userID bson.ObjectID, at time.Time) error
	// loadPresence reads a user's status and privacy settings for their
	// offline; nil means services.GetPresenceSettings.
	loadPresence func(ctx context.Context, userID bson.ObjectID) (services.PresenceSettings, error)
	// background tracks the Redis work a disconnect leaves running, so tests can
	// wait for it (#22).
	background sync.WaitGroup
	// presenceLocks order each user's online and offline publishes; see
	// presenceLock. slots bounds concurrent registrations; see registrationSlots.
	presenceLocks [64]sync.Mutex
	// announced holds the users this instance published as online, with the
	// chats the online went to, until it publishes them as offline. Changed
	// under the user's presence lock.
	announced sync.Map // userID -> []string
	// withheld holds the users this instance didn't announce because they
	// chose to hide, until their offline would be due; it is skipped too (#31).
	// Changed under the user's presence lock.
	withheld sync.Map // userID -> struct{}
	// audiences holds, for each user in announced, who their presence events
	// reach and whether they show lastSeen (#32). Changed with announced.
	audiences sync.Map // userID -> presenceAudience
	slots     chan struct{}
	slotsOnce sync.Once
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

// startLoops starts the hub's background loops. They stop when the hub's
// context is cancelled; nothing restarts them (#79).
func (eh *Hub) startLoops() {
	ctx := eh.Ctx
	loops := []func(context.Context){
		eh.runHub,
		eh.cleanupInactiveConnections,
		eh.syncPresenceWithRedis,
		eh.recheckSessions,
	}
	for _, loop := range loops {
		go loop(ctx)
	}
}

// chatLoadTimeout bounds the MongoDB read of a new connection's chats (#22). A
// variable so tests can shorten it.
var chatLoadTimeout = 5 * time.Second

// maxRegistrations is how many registrations may load chats and subscribe at
// once, so a reconnect storm can't flood MongoDB and Redis (#22). A variable so
// tests can lower it before the hub's first registration.
var maxRegistrations = 32

// QueueRegister hands client to the hub for registration. Until the hub adds it
// to the maps, the connection counts as pending, so a disconnect of the user's
// other connection isn't taken as their last (#22). It gives up once the hub
// has stopped, so the connection's goroutine can't wait forever; WaitRegistered
// then reports the stop.
//
// presence, when not nil, is stored under the same lock that marks the
// connection pending: stored earlier, an older connection of the user closing
// in between would take itself for the last and delete it, and the new
// connection would never be announced online (#31).
//
// It returns false, and queues nothing, when the user already holds
// MaxConnectionsPerUser connections here, registered or queued (#40). The count
// and the queueing share the lock, so two connects at once can't both pass.
func (eh *Hub) QueueRegister(client *models.Client, presence *UserPresence) bool {
	eh.Mutex.Lock()
	if connectionCountLocked(eh.Hub, client.UserID.Hex()) >= MaxConnectionsPerUser {
		eh.Mutex.Unlock()
		return false
	}
	addPending(eh.Hub, client)
	if presence != nil {
		GetPresenceInstance().Set(*presence)
	}
	eh.Mutex.Unlock()
	select {
	case eh.Register <- client:
	case <-eh.Ctx.Done():
		eh.Mutex.Lock()
		removePending(eh.Hub, client)
		eh.Mutex.Unlock()
	}
	return true
}

// QueueUnregister hands client to the hub for removal. It gives up once the hub
// has stopped, so the connection's goroutine can't wait forever (#22).
func (eh *Hub) QueueUnregister(client *models.Client) {
	select {
	case eh.Unregister <- client:
	case <-eh.Ctx.Done():
	}
}

// addPending marks client as queued for registration. The caller holds h.Mutex.
func addPending(h *models.Hub, client *models.Client) {
	userID := client.UserID.Hex()
	if h.PendingClients == nil {
		h.PendingClients = make(map[string]map[string]struct{})
	}
	if h.PendingClients[userID] == nil {
		h.PendingClients[userID] = make(map[string]struct{})
	}
	h.PendingClients[userID][client.ID] = struct{}{}
}

// removePending unmarks client and reports whether it was pending. The caller
// holds h.Mutex.
func removePending(h *models.Hub, client *models.Client) bool {
	userID := client.UserID.Hex()
	if _, ok := h.PendingClients[userID][client.ID]; !ok {
		return false
	}
	delete(h.PendingClients[userID], client.ID)
	if len(h.PendingClients[userID]) == 0 {
		delete(h.PendingClients, userID)
	}
	return true
}

// MaxConnectionsPerUser is the most connections one account may hold on one server
// (#40). Each has its own request limit, so without it an account could multiply its
// rate and memory by opening more sockets. Five leaves room for several devices and
// a reconnect overlapping the old connection. Servers don't share the count.
const MaxConnectionsPerUser = 5

// ConnectionCount returns how many connections userID holds on this server,
// registered or queued.
func (eh *Hub) ConnectionCount(userID string) int {
	eh.Mutex.RLock()
	defer eh.Mutex.RUnlock()
	return connectionCountLocked(eh.Hub, userID)
}

// connectionCountLocked counts userID's registered and queued connections. A
// queued one leaves PendingClients when it is registered, so none counts twice.
// The caller holds h.Mutex (read or write).
func connectionCountLocked(h *models.Hub, userID string) int {
	return len(h.UserClients[userID]) + len(h.PendingClients[userID])
}

// userInUseLocked says whether the user has a connection registered or queued
// for registration. The caller holds h.Mutex (read or write).
func userInUseLocked(h *models.Hub, userID string) bool {
	return len(h.UserClients[userID]) > 0 || len(h.PendingClients[userID]) > 0
}

// presenceLock returns the lock that orders one user's online and offline
// publishes, so a registration and a disconnect of the same user can't publish
// them in the wrong order (#22). Users share 64 locks.
func (eh *Hub) presenceLock(userID string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(userID)) // never fails
	return &eh.presenceLocks[h.Sum32()%uint32(len(eh.presenceLocks))]
}

// registrationSlots returns the semaphore that bounds concurrent registrations.
func (eh *Hub) registrationSlots() chan struct{} {
	eh.slotsOnce.Do(func() { eh.slots = make(chan struct{}, maxRegistrations) })
	return eh.slots
}

// runHub manages the main hub operations
func (eh *Hub) runHub(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			eh.logger.Warn("Hub context cancelled, stopping main loop")
			return

		case client := <-eh.Register:
			// In its own goroutine: it reads MongoDB and Redis, and this loop
			// must not wait on either (#22). The maps are changed only under the
			// lock, and a disconnect handled first is caught by IsClosed (#54).
			go eh.handleClientRegistration(client)

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

	// At most maxRegistrations at once. A connection that waits too long is
	// closed with 1011 so the app reconnects later (#22).
	slots := eh.registrationSlots()
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-time.After(chatLoadTimeout):
		eh.closeForRetry(client, errors.New("too many registrations at once"))
		return
	case <-eh.Ctx.Done():
		return
	}

	// Read the chats before taking the lock, with a time limit (#22).
	loadChats := eh.loadChats
	if loadChats == nil {
		loadChats = eh.getUserChats
	}
	ctx, cancel := context.WithTimeout(eh.Ctx, chatLoadTimeout)
	userChats, err := loadChats(ctx, client.UserID)
	cancel()
	if err != nil {
		// Registered without its chats, the connection would stay open and get
		// no messages.
		eh.closeForRetry(client, fmt.Errorf("load chats: %w", err))
		return
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
	removePending(eh.Hub, client)

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

	eh.Mutex.Unlock()

	// Subscribe after joining the rooms, never before (#55, #123): see
	// subscribeChats. Without its user channel the connection would miss direct
	// events, so it is closed like a failed chat load; its disconnect then takes
	// it out of the maps.
	if err := eh.pubSubManager.SubscribeUserToChannels(client.UserID, userChats); err != nil {
		eh.closeForRetry(client, fmt.Errorf("subscribe: %w", err))
		return
	}

	// Without the hub lock, so a slow Redis can't block every broadcast. It
	// reaches the members of the user's chats over presence:global (#31).
	if !eh.publishOnline(client) {
		return // closed during the subscribe: no welcome, no session check
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

// closeForRetry closes a connection the hub couldn't set up with 1011, so the app
// reconnects with backoff (#22). Its disconnect then cleans up.
func (eh *Hub) closeForRetry(client *models.Client, err error) {
	eh.logger.WithError(err).WithFields(logrus.Fields{"clientID": client.ID}).Error("Could not set up a new connection; closing it")
	client.Revoke(websocket.CloseInternalServerErr, "try again")
}

// publishOnline publishes the user as online if their presence says so and no
// other connection of theirs has announced it, and reports whether the
// connection is still registered. Announcing once per user keeps a burst of
// connections from flooding each other with presence events. Under the user's presence
// lock, the check and the publish can't interleave with the offline publish of a
// disconnect (#22).
func (eh *Hub) publishOnline(client *models.Client) bool {
	userID := client.UserID.Hex()
	lock := eh.presenceLock(userID)
	lock.Lock()
	defer lock.Unlock()

	eh.Mutex.RLock()
	_, registered := eh.Clients[client.ID]
	first := len(eh.UserClients[userID]) == 1
	chats := userActiveChats(eh.Hub, userID)
	eh.Mutex.RUnlock()
	if !registered || client.IsClosed() {
		return false
	}
	if first {
		// Later connections leave it to the sync loop, which also retries a
		// failed mark, so a slow Redis holds the lock once per user (#31).
		eh.markOnline(userID)
	}
	if _, already := eh.announced.Load(userID); already {
		return true // another connection of the user announced it
	}
	presence, ok := GetPresenceInstance().Get(client.UserID)
	if !ok {
		return true
	}
	audience, shown := audienceOf(presence)
	if !shown {
		eh.withheld.Store(userID, struct{}{})
		return true
	}
	status := presence.Status
	if status == "" {
		status = UserStatus(constants.UserStatusOnline)
	}
	eh.announced.Store(userID, chats)
	eh.audiences.Store(userID, audience)
	if err := eh.pubSubManager.PublishPresenceUpdate(audience.event(&models.WSPresenceStatus{
		UserID:   userID,
		Status:   string(status), // the chosen one, as REST shows it (#31)
		LastSeen: time.Now(),
		Chats:    chats,
	})); err != nil {
		eh.logger.WithError(err).Error("Failed to publish presence update")
	}
	return true
}

// handleClientUnregistration handles client disconnections with pub/sub cleanup
func (eh *Hub) handleClientUnregistration(client *models.Client) {
	eh.logger.WithFields(logrus.Fields{"clientID": client.ID, "userID": client.UserID.Hex()}).Info("Unregistering client")

	clientID := client.ID
	userID := client.UserID.Hex()

	eh.Mutex.Lock()

	if _, ok := eh.Clients[clientID]; !ok {
		// Not registered yet, or already removed. Closing Send marks it
		// closed, so a registration still queued skips it (#54).
		wasPending := removePending(eh.Hub, client)
		inUse := userInUseLocked(eh.Hub, userID)
		if !inUse {
			// The controller stored presence for this connection before
			// queueing it; with the registration skipped, nothing else removes it.
			GetPresenceInstance().DeleteForClient(client.UserID, clientID)
		}
		eh.Mutex.Unlock()
		client.CloseSend()
		if wasPending && !inUse {
			// A connection that closed while this one was queued left the
			// user's channel and offline status to it (#22).
			eh.releaseInBackground(client.UserID, nil, true, nil)
		}
		return
	}

	// Leave every room this connection is in. Rooms left empty are no longer
	// needed on this instance, so their channels are released below (#20).
	// Deleting from a map while ranging over it is safe in Go (unlike the
	// slice this replaced, #116). The chats are kept for the offline event,
	// which goes to their members (#31).
	chats := slices.Collect(maps.Keys(eh.ClientChats[clientID]))
	var emptyRooms []string
	for chatID := range eh.ClientChats[clientID] {
		if removeClientFromRoom(eh.Hub, client, chatID) {
			emptyRooms = append(emptyRooms, chatID)
		}
	}

	delete(eh.Clients, clientID)
	if userClients, exists := eh.UserClients[userID]; exists {
		delete(userClients, clientID)
		if len(userClients) == 0 {
			delete(eh.UserClients, userID)
		}
	}

	// The last connection counts the ones still registering (#22). Presence is
	// removed under the lock, so a registration can't add a connection between
	// this check and the delete.
	lastConnection := !userInUseLocked(eh.Hub, userID)
	if lastConnection {
		GetPresenceInstance().Delete(client.UserID) // in memory, no I/O
	}

	eh.Mutex.Unlock()

	client.CloseSend()
	eh.releaseInBackground(client.UserID, emptyRooms, lastConnection, chats)
}

// releaseInBackground runs releaseAfterDisconnect off the hub goroutine (#22).
func (eh *Hub) releaseInBackground(userID bson.ObjectID, emptyRooms []string, lastConnection bool, chats []string) {
	eh.background.Add(1)
	go func() {
		defer eh.background.Done()
		eh.releaseAfterDisconnect(userID, emptyRooms, lastConnection, chats)
	}()
}

// releaseAfterDisconnect releases the Redis channels a closed connection left
// unused and, after the user's last connection, publishes them as offline. Each
// release checks again, so a connection registering meanwhile keeps its channels.
// The offline publish runs under the user's presence lock, and only while the
// user has no connection, so it can't overtake a registration's online publish.
// It goes to the members of chats, the closed connection's chats, and of the
// chats the online went to; it is skipped while another server holds the user
// (#31).
func (eh *Hub) releaseAfterDisconnect(userID bson.ObjectID, emptyRooms []string, lastConnection bool, chats []string) {
	eh.pubSubManager.releaseEmptyChats(emptyRooms)
	if !lastConnection {
		eh.rememberOfflineChats(userID.Hex(), chats)
		return
	}
	eh.pubSubManager.UnsubscribeUser(userID)
	current, known := eh.currentPresence(userID)

	lock := eh.presenceLock(userID.Hex())
	lock.Lock()
	defer lock.Unlock()
	if eh.pubSubManager.userInUse(userID.Hex()) {
		return // a connection registered meanwhile; it announces itself
	}
	announced, wasAnnounced := eh.announced.LoadAndDelete(userID.Hex())
	if wasAnnounced {
		chats = unionChats(chats, announced.([]string))
	}
	stored, _ := eh.audiences.LoadAndDelete(userID.Hex())
	announcedTo, _ := stored.(presenceAudience)
	audience := announcedTo.forOffline(current, known)
	_, wasWithheld := eh.withheld.LoadAndDelete(userID.Hex())
	heldUntil, elsewhere, err := eh.markOffline(userID.Hex())
	if wasWithheld && !wasAnnounced {
		// Not announced because they hide (invisible, or offline by choice):
		// an offline with a fresh lastSeen would show they were just connected.
		eh.recordLastSeen(userID, time.Now())
		return
	}
	if err != nil {
		// Publish anyway: one server is the usual case, and a user shown
		// online forever is worse than a wrong offline.
		eh.logger.WithError(err).Error("Could not check the user's other servers; publishing offline")
	}
	if elsewhere {
		// Still connected to another server, or that server stopped without
		// removing its mark; check again once the mark has expired.
		eh.recheckOfflineAt(userID, chats, audience, heldUntil)
		return
	}
	eh.publishOffline(userID, chats, audience)
}

// rememberOfflineChats adds a closed connection's chats to those the user's
// offline goes to, so chats it joined after the online aren't missed (#31).
func (eh *Hub) rememberOfflineChats(userID string, chats []string) {
	if len(chats) == 0 {
		return
	}
	lock := eh.presenceLock(userID)
	lock.Lock()
	defer lock.Unlock()
	if announced, ok := eh.announced.Load(userID); ok {
		eh.announced.Store(userID, unionChats(announced.([]string), chats))
	}
}

// recheckOfflineAt publishes the offline skipped for another server's mark
// once that mark has expired, unless the user is connected again here or a
// server still holds them. A server that stopped without removing its mark,
// as in a deploy, can't otherwise leave the user online forever (#31).
func (eh *Hub) recheckOfflineAt(userID bson.ObjectID, chats []string, audience presenceAudience, heldUntil time.Time) {
	time.AfterFunc(time.Until(heldUntil)+offlineRecheckMargin, func() {
		if eh.Ctx.Err() != nil {
			return
		}
		lock := eh.presenceLock(userID.Hex())
		lock.Lock()
		defer lock.Unlock()
		if eh.pubSubManager.userInUse(userID.Hex()) {
			return // connected here again; that connection's close publishes it
		}
		if _, elsewhere, err := eh.markOffline(userID.Hex()); err != nil || elsewhere {
			return // the server holding the user publishes it when they leave
		}
		eh.publishOffline(userID, chats, audience)
	})
}

// hidesPresence reports whether a user who chose status is never announced
// as online: invisible, or offline by choice. Away and dnd users are (#31).
func hidesPresence(status UserStatus) bool {
	return constants.UserStatus(status).Hidden()
}

// presenceAudience is who a user's presence events reach and what they show,
// from their privacy settings (#32).
type presenceAudience struct {
	contactsOnly bool
	contacts     []string // with contactsOnly, the only users the events reach
	showLastSeen bool
}

// audienceOf returns who presence's events reach, and false when the user
// is announced to nobody: they chose to hide (#31), show their status to
// nobody, or to contacts only and have none (#32).
func audienceOf(presence UserPresence) (presenceAudience, bool) {
	if hidesPresence(presence.Status) {
		return presenceAudience{}, false
	}
	audience := presenceAudience{showLastSeen: presence.ShowLastSeen}
	switch scope := constants.PrivacyScope(presence.ShowOnline); {
	case scope == constants.PrivacyContacts && len(presence.Contacts) > 0:
		audience.contactsOnly = true
		audience.contacts = presence.Contacts
	case !scope.Lets(false):
		return presenceAudience{}, false // none, unknown, or contacts with none
	}
	return audience, true
}

// forOffline is the audience of the offline for a user announced to a, now
// that their settings are current (known false: they couldn't be read). The
// offline reaches everyone the online did, so nobody is left showing them
// online; it shows lastSeen only if the current settings still let all of
// those people see it, so a change made while connected is respected (#32).
func (a presenceAudience) forOffline(current UserPresence, known bool) presenceAudience {
	now, shown := audienceOf(current)
	if !known || !shown || !now.showLastSeen {
		a.showLastSeen = false
		return a
	}
	if now.contactsOnly && (!a.contactsOnly || !subsetOf(a.contacts, now.contacts)) {
		a.showLastSeen = false
	}
	return a
}

// subsetOf reports whether every ID in ids is in of.
func subsetOf(ids, of []string) bool {
	set := make(map[string]bool, len(of))
	for _, id := range of {
		set[id] = true
	}
	for _, id := range ids {
		if !set[id] {
			return false
		}
	}
	return true
}

// currentPresence reads userID's settings for their offline, outside the
// presence lock, which other users share.
func (eh *Hub) currentPresence(userID bson.ObjectID) (UserPresence, bool) {
	load := eh.loadPresence
	if load == nil {
		load = services.GetPresenceSettings
	}
	ctx, cancel := context.WithTimeout(context.Background(), lastSeenTimeout)
	defer cancel()
	settings, err := load(ctx, userID)
	if err != nil {
		eh.logger.WithError(err).WithField("userID", userID.Hex()).Warn("Could not read the user's privacy settings; offline sent without lastSeen")
		return UserPresence{}, false
	}
	return PresenceFromSettings(userID, settings), true
}

// event applies the audience to a presence event: who it reaches, and no
// lastSeen when the user hides it.
func (a presenceAudience) event(e *models.WSPresenceStatus) *models.WSPresenceStatus {
	e.ContactsOnly = a.contactsOnly
	e.Audience = a.contacts
	if !a.showLastSeen {
		e.LastSeen = time.Time{}
	}
	return e
}

// publishOffline publishes userID as offline to the members of chats that
// audience reaches, and stores the time as their lastSeen. The caller holds
// the user's presence lock.
func (eh *Hub) publishOffline(userID bson.ObjectID, chats []string, audience presenceAudience) {
	at := time.Now()
	if err := eh.pubSubManager.PublishPresenceUpdate(audience.event(&models.WSPresenceStatus{
		UserID:   userID.Hex(),
		Status:   string(constants.UserStatusOffline),
		LastSeen: at,
		Chats:    chats,
	})); err != nil {
		eh.logger.WithError(err).Error("Failed to publish presence update")
	}
	eh.recordLastSeen(userID, at)
}

// lastSeenTimeout bounds the lastSeen write.
const lastSeenTimeout = 5 * time.Second

// recordLastSeen saves at as userID's lastSeen in the background, so a slow
// MongoDB doesn't hold the presence lock, which other users share. Writes
// can't move lastSeen back, so their order doesn't matter (#31). It isn't
// tracked by eh.background: an offline published by recheckOfflineAt's timer
// would call Add while a test's Wait may run with the count at zero.
func (eh *Hub) recordLastSeen(userID bson.ObjectID, at time.Time) {
	save := eh.saveLastSeen
	if save == nil {
		save = services.RecordLastSeen
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), lastSeenTimeout)
		defer cancel()
		if err := save(ctx, userID, at); err != nil {
			eh.logger.WithError(err).WithField("userID", userID.Hex()).Error("Failed to save the user's last seen time")
		}
	}()
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

// performPresenceSync renews this server's online entries for its connected
// users (#31).
func (eh *Hub) performPresenceSync() {
	eh.Mutex.RLock()
	userIDs := slices.Collect(maps.Keys(eh.UserClients))
	eh.Mutex.RUnlock()

	if err := eh.refreshOnline(userIDs); err != nil {
		eh.logger.WithError(err).Error("Failed to renew online entries in Redis")
	}
}

// unionChats returns the chats in a or b, each once.
func unionChats(a, b []string) []string {
	out := slices.Clone(a)
	for _, chatID := range b {
		if !slices.Contains(out, chatID) {
			out = append(out, chatID)
		}
	}
	return out
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
		eh.QueueUnregister(client)
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
