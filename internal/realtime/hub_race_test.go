package realtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// These tests guard #19. They only prove anything under `go test -race`, which
// `node scripts/verify.mjs --full` (and so CI) runs.

const raceIterations = 500

// newTestHub returns a hub whose Redis calls fail at once, so the hub code runs
// without a server. Failed publishes are only logged or returned.
func newTestHub(t *testing.T) *Hub {
	t.Helper()

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	redisClient := redis.NewClient(&redis.Options{
		MaxRetries: -1,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("redis is not available in tests")
		},
	})
	t.Cleanup(func() { _ = redisClient.Close() })

	base := &models.Hub{
		Clients:       make(map[string]*models.Client),
		ChatClients:   make(map[string]map[string]*models.Client),
		UserClients:   make(map[string]map[string]*models.Client),
		UserInfoCache: make(map[string]*models.UserDisplayInfo),
		CacheExpiry:   make(map[string]time.Time),
		ClientChats:   make(map[string]map[string]struct{}),
		Register:      make(chan *models.Client, 8),
		Unregister:    make(chan *models.Client, 8),
	}
	base.Ctx, base.Cancel = context.WithCancel(context.Background())
	t.Cleanup(base.Cancel)

	return &Hub{
		Hub:           base,
		pubSubManager: NewPubSubManager(redisClient, base, logger, "test"),
		redisClient:   redisClient,
		logger:        logger,
		instanceID:    "test",
		saveLastSeen:  func(context.Context, bson.ObjectID, time.Time) error { return nil },
	}
}

func newTestClient(id string, userID bson.ObjectID) *models.Client {
	return &models.Client{ID: id, UserID: userID, Send: make(chan models.WebSocketMessage, 1)}
}

// addClient puts a client in the hub's maps, as handleClientRegistration does.
func addClient(hub *Hub, client *models.Client, chatIDs ...string) {
	userID := client.UserID.Hex()

	hub.Mutex.Lock()
	defer hub.Mutex.Unlock()

	hub.Clients[client.ID] = client
	if hub.UserClients[userID] == nil {
		hub.UserClients[userID] = make(map[string]*models.Client)
	}
	hub.UserClients[userID][client.ID] = client
	for _, chatID := range chatIDs {
		addClientToRoom(hub.Hub, client, chatID)
	}
}

// runTogether runs each function raceIterations times, all at the same time.
func runTogether(fns ...func(i int)) {
	var wg sync.WaitGroup
	for _, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range raceIterations {
				fn(i)
			}
		}()
	}
	wg.Wait()
}

// A chat broadcast used to range over the chat's client map after unlocking,
// while joins wrote to that map: a fatal "concurrent map iteration and map write".
func TestChatBroadcastDuringJoinAndLeave(t *testing.T) {
	hub := newTestHub(t)
	addClient(hub, newTestClient("seed", bson.NewObjectID()), "chat1")
	message := &models.WebSocketMessage{Type: "message"}

	runTogether(
		func(int) { hub.pubSubManager.handleChatMessage("chat1", message) },
		func(i int) {
			client := newTestClient(fmt.Sprintf("c%d", i), bson.NewObjectID())
			_ = hub.JoinChat(context.Background(), client, "chat1")
			_ = hub.LeaveChat(client, "chat1")
		},
	)
}

// The same pattern on UserClients, with disconnects as the writer.
func TestUserBroadcastDuringDisconnect(t *testing.T) {
	hub := newTestHub(t)
	userID := bson.NewObjectID()
	addClient(hub, newTestClient("seed", userID))
	message := &models.WebSocketMessage{Type: "message"}

	runTogether(
		func(int) { hub.pubSubManager.handleUserMessage(userID.Hex(), message) },
		func(i int) {
			client := newTestClient(fmt.Sprintf("c%d", i), userID)
			addClient(hub, client)
			hub.handleClientUnregistration(client)
		},
	)
}

// A disconnect used to change ChatClients and the user's chat list with no lock held,
// while JoinChat and LeaveChat changed them under the lock. Each client here is
// its user's last connection, so the whole disconnect path runs.
func TestDisconnectDuringJoinAndLeave(t *testing.T) {
	hub := newTestHub(t)
	addClient(hub, newTestClient("seed", bson.NewObjectID()), "chat1")
	message := &models.WebSocketMessage{Type: "message"}

	runTogether(
		func(int) { hub.pubSubManager.handleChatMessage("chat1", message) },
		func(i int) {
			client := newTestClient(fmt.Sprintf("a%d", i), bson.NewObjectID())
			addClient(hub, client, "chat1", "chat2")
			hub.handleClientUnregistration(client)
		},
		func(i int) {
			client := newTestClient(fmt.Sprintf("b%d", i), bson.NewObjectID())
			_ = hub.JoinChat(context.Background(), client, "chat1")
			_ = hub.LeaveChat(client, "chat1")
		},
	)
}

// LeaveChat shifts a user's active chats in place, so the presence handler must
// not read that slice after unlocking.
func TestPresenceUpdateDuringLeave(t *testing.T) {
	hub := newTestHub(t)
	client := newTestClient("c1", bson.NewObjectID())
	userID := client.UserID.Hex()
	addClient(hub, client, "chat1", "chat2", "chat3")
	// As it arrives from Redis, so the handler reads the rooms (#31).
	message := &models.WebSocketMessage{Type: "presence", UserID: userID, Data: map[string]any{
		"userId": userID, "status": "online", "chats": []any{"chat1", "chat2", "chat3"},
	}}

	runTogether(
		func(int) { hub.pubSubManager.handlePresenceMessage(message) },
		func(int) {
			_ = hub.LeaveChat(client, "chat1")
			_ = hub.JoinChat(context.Background(), client, "chat1")
		},
	)
}

// GetUserInfo hands out the cached pointer, so an update must not change the
// struct that a caller may be reading.
func TestUserInfoUpdateDuringRead(t *testing.T) {
	hub := newTestHub(t)
	userID := bson.NewObjectID()
	hub.UserInfoCache[userID.Hex()] = &models.UserDisplayInfo{Username: "old"}
	hub.CacheExpiry[userID.Hex()] = time.Now().Add(time.Hour)

	runTogether(
		func(int) {
			info, err := hub.GetUserInfo(userID)
			if err != nil {
				t.Errorf("GetUserInfo: %v", err)
				return
			}
			_ = info.Username
		},
		func(i int) {
			hub.UpdateUserInfoCache(userID.Hex(), models.UserDisplayInfo{Username: fmt.Sprintf("u%d", i)})
		},
	)

	info, err := hub.GetUserInfo(userID)
	if err != nil {
		t.Fatalf("GetUserInfo: %v", err)
	}
	if want := fmt.Sprintf("u%d", raceIterations-1); info.Username != want {
		t.Fatalf("cached username = %q, want %q", info.Username, want)
	}
}

// performCleanup used to send on Unregister while holding hub.Mutex. With the
// queue full, it waited for handleClientUnregistration, the only consumer, and
// that waited for the lock. Two stale clients are needed: the consumer takes the
// first, then needs the lock while cleanup is stuck sending the second.
func TestCleanupDoesNotBlockUnregister(t *testing.T) {
	hub := newTestHub(t)
	hub.Unregister = make(chan *models.Client) // no buffer: the same as a full queue
	addClient(hub, newTestClient("stale1", bson.NewObjectID()))
	addClient(hub, newTestClient("stale2", bson.NewObjectID()))

	go hub.performCleanup()

	finished := make(chan struct{})
	go func() {
		defer close(finished)
		hub.handleClientUnregistration(<-hub.Unregister)
		hub.handleClientUnregistration(<-hub.Unregister)
	}()

	select {
	case <-finished:
	case <-time.After(20 * time.Second):
		t.Fatal("deadlock: performCleanup kept hub.Mutex while waiting on Unregister")
	}

	hub.Mutex.RLock()
	defer hub.Mutex.RUnlock()
	if len(hub.Clients) != 0 {
		t.Fatalf("%d stale clients are still registered", len(hub.Clients))
	}
}

// Read and write goroutines record activity while cleanup reads it.
func TestActivityUpdateDuringCleanup(t *testing.T) {
	hub := newTestHub(t)
	client := newTestClient("c1", bson.NewObjectID())
	client.Touch()
	addClient(hub, client)

	runTogether(
		func(int) { client.Touch() },
		func(int) { hub.performCleanup() },
	)

	if len(hub.Unregister) != 0 {
		t.Fatal("active client was queued for removal")
	}
}

// One connection closing must leave the user's other connection working.
func TestDisconnectKeepsOtherConnectionOfUser(t *testing.T) {
	hub := newTestHub(t)
	userID := bson.NewObjectID()
	closing := newTestClient("closing", userID)
	staying := newTestClient("staying", userID)
	addClient(hub, closing, "chat1")
	addClient(hub, staying, "chat1")

	unregisterAndWait(hub, closing)
	unregisterAndWait(hub, closing) // a second unregister must do nothing

	if _, open := <-closing.Send; open {
		t.Fatal("closing client's Send channel should be closed")
	}
	hub.Mutex.RLock()
	_, closingRegistered := hub.Clients[closing.ID]
	_, closingInChat := hub.ChatClients["chat1"][closing.ID]
	_, stayingRegistered := hub.UserClients[userID.Hex()][staying.ID]
	_, stayingInChat := hub.ChatClients["chat1"][staying.ID]
	hub.Mutex.RUnlock()
	if closingRegistered {
		t.Fatal("closing client is still in Clients")
	}
	if closingInChat {
		t.Fatal("closing client is still in the chat")
	}
	if !stayingRegistered {
		t.Fatal("other connection was removed from UserClients")
	}
	if !stayingInChat {
		t.Fatal("other connection was removed from the chat")
	}

	hub.pubSubManager.handleChatMessage("chat1", &models.WebSocketMessage{Type: "message"})
	if len(staying.Send) != 1 {
		t.Fatal("other connection did not get the message")
	}
}

// When a user's last connection closes, the user must leave the hub's maps.
func TestDisconnectOfLastConnectionRemovesUser(t *testing.T) {
	hub := newTestHub(t)
	client := newTestClient("c1", bson.NewObjectID())
	addClient(hub, client, "chat1")

	unregisterAndWait(hub, client)

	hub.Mutex.RLock()
	_, inUsers := hub.UserClients[client.UserID.Hex()]
	_, hasRooms := hub.ClientChats[client.ID]
	_, chatLeft := hub.ChatClients["chat1"]
	hub.Mutex.RUnlock()
	if inUsers {
		t.Fatal("user is still in UserClients")
	}
	if hasRooms {
		t.Fatal("closed connection is still in ClientChats")
	}
	if chatLeft {
		t.Fatal("empty chat is still in ChatClients")
	}
}

// Closing a user's last connection must take every chat out of the hub and
// unsubscribe every chat channel. The loop used to edit the list it was
// reading, so with [A, B, C] it kept B and unsubscribed only B (#116).
func TestDisconnectOfLastConnectionLeavesEveryChat(t *testing.T) {
	hub := newTestHub(t)
	client := newTestClient("c1", bson.NewObjectID())
	chats := []string{"chatA", "chatB", "chatC"}
	addClient(hub, client, chats...)

	// Stand-in subscriptions: a PubSub with no channels never dials Redis.
	pm := hub.pubSubManager
	pm.mu.Lock()
	for _, chatID := range chats {
		pm.subscriptions[pm.getChatChannel(chatID)] = hub.redisClient.Subscribe(context.Background())
	}
	pm.mu.Unlock()

	unregisterAndWait(hub, client)

	hub.Mutex.RLock()
	left, ok := hub.ClientChats[client.ID]
	hub.Mutex.RUnlock()
	if ok {
		t.Fatalf("closed connection is still in chats %v", left)
	}
	for _, chatID := range chats {
		hub.Mutex.RLock()
		_, ok := hub.ChatClients[chatID]
		hub.Mutex.RUnlock()
		if ok {
			t.Errorf("%s is still in ChatClients", chatID)
		}
		if pm.IsSubscribed(pm.getChatChannel(chatID)) {
			t.Errorf("%s is still subscribed", chatID)
		}
	}
}

// stubSubscriptions marks channels as subscribed without Redis: a PubSub with
// no channels never dials.
func stubSubscriptions(hub *Hub, channels ...string) {
	pm := hub.pubSubManager
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, channel := range channels {
		pm.subscriptions[channel] = hub.redisClient.Subscribe(context.Background())
	}
}

// unregisterAndWait handles client's disconnect and waits for the Redis work it
// leaves running (#22), so a test can check the result.
func unregisterAndWait(hub *Hub, client *models.Client) {
	hub.handleClientUnregistration(client)
	hub.background.Wait()
}

func inRoomUnlocked(hub *Hub, chatID, clientID string) bool {
	hub.Mutex.RLock()
	defer hub.Mutex.RUnlock()
	_, ok := hub.ChatClients[chatID][clientID]
	return ok
}

// A user's last connection closing releases their user channel even when they
// follow no chat (#20).
func TestDisconnectWithNoChatsReleasesUserChannel(t *testing.T) {
	hub := newTestHub(t)
	pm := hub.pubSubManager
	client := newTestClient("c1", bson.NewObjectID())
	addClient(hub, client)
	stubSubscriptions(hub, pm.getUserChannel(client.UserID.Hex()))

	unregisterAndWait(hub, client)

	if pm.IsSubscribed(pm.getUserChannel(client.UserID.Hex())) {
		t.Fatal("user channel is still subscribed")
	}
}

// A second device registering used to overwrite the user's chat list, so the
// first device's join_chat room was forgotten and it stayed in it after
// closing (#20).
func TestDisconnectLeavesChatJoinedBeforeAnotherDeviceConnected(t *testing.T) {
	hub := newTestHub(t)
	pm := hub.pubSubManager
	userID := bson.NewObjectID()
	first := newTestClient("first", userID)
	second := newTestClient("second", userID)
	stubSubscriptions(hub, pm.getChatChannel("chatA"), pm.getChatChannel("chatB"))

	addClient(hub, first, "chatA")
	_ = hub.JoinChat(context.Background(), first, "chatB") // publish fails without Redis
	addClient(hub, second, "chatA")

	unregisterAndWait(hub, first)

	if inRoomUnlocked(hub, "chatB", first.ID) {
		t.Fatal("closed connection is still in the chat it joined")
	}
	if pm.IsSubscribed(pm.getChatChannel("chatB")) {
		t.Fatal("chat nobody on this instance follows is still subscribed")
	}
	if !inRoomUnlocked(hub, "chatA", second.ID) || !pm.IsSubscribed(pm.getChatChannel("chatA")) {
		t.Fatal("the other device lost a chat it still follows")
	}
}

// Leaving a chat on one device used to take it off the list for every device,
// so the other device stayed in the room after closing (#20).
func TestDisconnectAfterOtherDeviceLeftChat(t *testing.T) {
	hub := newTestHub(t)
	pm := hub.pubSubManager
	userID := bson.NewObjectID()
	leaving := newTestClient("leaving", userID)
	closing := newTestClient("closing", userID)
	addClient(hub, leaving, "chatA")
	addClient(hub, closing, "chatA")
	stubSubscriptions(hub, pm.getChatChannel("chatA"))

	_ = hub.LeaveChat(leaving, "chatA") // publish fails without Redis
	unregisterAndWait(hub, closing)

	if inRoomUnlocked(hub, "chatA", closing.ID) {
		t.Fatal("closed connection is still in the room")
	}
	if pm.IsSubscribed(pm.getChatChannel("chatA")) {
		t.Fatal("empty room's channel is still subscribed")
	}
}

// A chat another user still follows here stays subscribed.
func TestDisconnectKeepsChatOtherUserFollows(t *testing.T) {
	hub := newTestHub(t)
	pm := hub.pubSubManager
	closing := newTestClient("x", bson.NewObjectID())
	staying := newTestClient("y", bson.NewObjectID())
	addClient(hub, closing, "chatA")
	addClient(hub, staying, "chatA")
	stubSubscriptions(hub, pm.getChatChannel("chatA"), pm.getUserChannel(closing.UserID.Hex()))

	unregisterAndWait(hub, closing)

	if !pm.IsSubscribed(pm.getChatChannel("chatA")) {
		t.Fatal("chat another user follows was unsubscribed")
	}
	if pm.IsSubscribed(pm.getUserChannel(closing.UserID.Hex())) {
		t.Fatal("closed user's channel is still subscribed")
	}
}

// leave_chat emptying a room releases its channel (#20).
func TestLeaveChatReleasesEmptyRoom(t *testing.T) {
	hub := newTestHub(t)
	pm := hub.pubSubManager
	client := newTestClient("c1", bson.NewObjectID())
	addClient(hub, client, "chatA")
	stubSubscriptions(hub, pm.getChatChannel("chatA"))

	_ = hub.LeaveChat(client, "chatA") // publish fails without Redis

	if pm.IsSubscribed(pm.getChatChannel("chatA")) {
		t.Fatal("empty room's channel is still subscribed after leave_chat")
	}
}

// Removing the last local members of a group releases its channel (#20).
// Without Redis the publish fails, so the local removal path runs.
func TestRemoveMembersReleasesEmptyRoom(t *testing.T) {
	hub := newTestHub(t)
	pm := hub.pubSubManager
	client := newTestClient("c1", bson.NewObjectID())
	addClient(hub, client, "chatA")
	stubSubscriptions(hub, pm.getChatChannel("chatA"))

	_ = hub.RemoveMembersFromChat("chatA", []string{client.UserID.Hex()})

	if pm.IsSubscribed(pm.getChatChannel("chatA")) {
		t.Fatal("empty room's channel is still subscribed after remove_user")
	}
}

// A join_chat handled after the connection was unregistered must not put the
// closed connection back in a room, where nothing would ever remove it.
func TestJoinChatAfterDisconnectIsIgnored(t *testing.T) {
	hub := newTestHub(t)
	pm := hub.pubSubManager
	client := newTestClient("c1", bson.NewObjectID())
	addClient(hub, client)
	stubSubscriptions(hub, pm.getChatChannel("chatA"))
	unregisterAndWait(hub, client)

	_ = hub.JoinChat(context.Background(), client, "chatA")

	if inRoomUnlocked(hub, "chatA", client.ID) {
		t.Fatal("closed connection was added to a room")
	}
	hub.Mutex.RLock()
	_, ok := hub.ClientChats[client.ID]
	hub.Mutex.RUnlock()
	if ok {
		t.Fatal("closed connection is in ClientChats")
	}
}

// A join landing between the release's "room is empty" check and its
// unsubscribe must leave the joiner subscribed (#55). The hook runs the whole
// join at exactly that point. Needs Redis: run with TEST_DB_REQUIRED=1.
func TestJoinDuringChannelReleaseKeepsSubscription(t *testing.T) {
	redisClient := testutil.UseTestRedis(t)
	hub := newTestHub(t)
	hub.redisClient = redisClient
	hub.pubSubManager = NewPubSubManager(redisClient, hub.Hub, hub.logger, "test")
	pm := hub.pubSubManager
	t.Cleanup(pm.cancel)

	chatID := bson.NewObjectID().Hex()
	leaving := newTestClient("leaving", bson.NewObjectID())
	joining := newTestClient("joining", bson.NewObjectID())
	addClient(hub, leaving, chatID)
	addClient(hub, joining)
	if err := pm.subscribeToChannel(pm.getChatChannel(chatID)); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = pm.unsubscribeFromChannel(pm.getChatChannel(chatID)) })

	// Set before anything runs and never changed, so no goroutine races it
	// (#124). The join releases again itself, which must not join again.
	var once sync.Once
	pm.afterEmptyCheck = func(string) {
		once.Do(func() { _ = hub.JoinChat(context.Background(), joining, chatID) })
	}

	unregisterAndWait(hub, leaving)

	if !inRoomUnlocked(hub, chatID, joining.ID) {
		t.Fatal("joiner is not in the room")
	}
	if !pm.IsSubscribed(pm.getChatChannel(chatID)) {
		t.Fatal("joiner is in the room but the chat isn't subscribed")
	}
}

// A removal that empties the room between a join's room entry and its
// subscribe finds nothing to release; the join must then release the channel
// itself, or it stays subscribed for a room nobody is in (#123). Needs Redis:
// run with TEST_DB_REQUIRED=1.
func TestRemovalDuringJoinReleasesChannel(t *testing.T) {
	redisClient := testutil.UseTestRedis(t)
	hub := newTestHub(t)
	hub.redisClient = redisClient
	hub.pubSubManager = NewPubSubManager(redisClient, hub.Hub, hub.logger, "test")
	pm := hub.pubSubManager
	t.Cleanup(pm.cancel)

	chatID := bson.NewObjectID().Hex()
	joining := newTestClient("joining", bson.NewObjectID())
	addClient(hub, joining)
	t.Cleanup(func() { _ = pm.unsubscribeFromChannel(pm.getChatChannel(chatID)) })

	// Another connection watches the chat, to see whether a join is announced.
	observer := redisClient.Subscribe(context.Background(), pm.getChatChannel(chatID))
	t.Cleanup(func() { _ = observer.Close() })
	if _, err := observer.Receive(context.Background()); err != nil {
		t.Fatalf("observer subscribe: %v", err)
	}

	pm.afterRoomJoin = func(string) {
		// What RemoveMembersFromChat does when its publish fails.
		if removeUsersFromRoom(hub.Hub, chatID, []string{joining.UserID.Hex()}) {
			pm.releaseEmptyChats([]string{chatID})
		}
	}

	_ = hub.JoinChat(context.Background(), joining, chatID)

	if inRoomUnlocked(hub, chatID, joining.ID) {
		t.Fatal("removed member is still in the room")
	}
	if pm.IsSubscribed(pm.getChatChannel(chatID)) {
		t.Fatal("chat nobody on this instance is in is still subscribed")
	}
	if msg, err := observer.ReceiveTimeout(context.Background(), 200*time.Millisecond); err == nil {
		t.Fatalf("a join was announced for a removed member: %v", msg)
	}
}

// The usual removal path: a members-removed message read from the chat's
// channel releases it on another goroutine, possibly while the join releases
// too (#123).
func TestAsyncRemovalDuringJoinReleasesChannel(t *testing.T) {
	redisClient := testutil.UseTestRedis(t)
	hub := newTestHub(t)
	hub.redisClient = redisClient
	hub.pubSubManager = NewPubSubManager(redisClient, hub.Hub, hub.logger, "test")
	pm := hub.pubSubManager
	t.Cleanup(pm.cancel)

	chatID := bson.NewObjectID().Hex()
	joining := newTestClient("joining", bson.NewObjectID())
	addClient(hub, joining)
	t.Cleanup(func() { _ = pm.unsubscribeFromChannel(pm.getChatChannel(chatID)) })

	pm.afterRoomJoin = func(string) {
		pm.handleMembersRemoved(chatID, &models.WebSocketMessage{Data: []any{joining.UserID.Hex()}})
	}

	_ = hub.JoinChat(context.Background(), joining, chatID)

	deadline := time.Now().Add(2 * time.Second)
	for pm.IsSubscribed(pm.getChatChannel(chatID)) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if pm.IsSubscribed(pm.getChatChannel(chatID)) {
		t.Fatal("chat nobody on this instance is in is still subscribed")
	}
	if inRoomUnlocked(hub, chatID, joining.ID) {
		t.Fatal("removed member is still in the room")
	}
}

// The same race on registration, which subscribes to all of the user's chats
// after entering their rooms (#123).
func TestRemovalDuringRegistrationReleasesChannel(t *testing.T) {
	redisClient := testutil.UseTestRedis(t)
	hub := newTestHub(t)
	hub.redisClient = redisClient
	hub.pubSubManager = NewPubSubManager(redisClient, hub.Hub, hub.logger, "test")
	pm := hub.pubSubManager
	t.Cleanup(pm.cancel)

	removed, kept := bson.NewObjectID().Hex(), bson.NewObjectID().Hex()
	stubRegistration(hub, removed, kept)
	client := newTestClient("c1", bson.NewObjectID())
	t.Cleanup(func() {
		pm.UnsubscribeUser(client.UserID)
		_ = pm.unsubscribeFromChannel(pm.getChatChannel(removed))
		_ = pm.unsubscribeFromChannel(pm.getChatChannel(kept))
	})

	pm.afterRoomJoin = func(chatID string) {
		if chatID == removed && removeUsersFromRoom(hub.Hub, chatID, []string{client.UserID.Hex()}) {
			pm.releaseEmptyChats([]string{chatID})
		}
	}

	hub.handleClientRegistration(client)

	if pm.IsSubscribed(pm.getChatChannel(removed)) {
		t.Fatal("chat the user was removed from is still subscribed")
	}
	if !inRoomUnlocked(hub, kept, client.ID) || !pm.IsSubscribed(pm.getChatChannel(kept)) {
		t.Fatal("the user's other chat lost its room or subscription")
	}
}
