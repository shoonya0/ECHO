package realtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/models"
)

// These tests guard #22: the hub goroutine handles every connect and disconnect,
// so it must not wait on MongoDB or Redis.

// waitRegistered fails the test unless client is registered within a second.
func waitRegistered(t *testing.T, client *models.Client) {
	t.Helper()
	select {
	case <-client.Registered():
	case <-time.After(time.Second):
		t.Fatalf("connection %s was not registered", client.ID)
	}
}

// A connection whose chats load slowly doesn't hold up other connections.
func TestSlowChatLoadDoesNotStallOtherConnections(t *testing.T) {
	hub := newTestHub(t)
	stubRegistration(hub)
	slowUser := bson.NewObjectID()
	release := make(chan struct{})
	hub.loadChats = func(ctx context.Context, userID bson.ObjectID) ([]string, error) {
		if userID == slowUser {
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
		return nil, nil
	}
	go hub.runHub(hub.Ctx)

	slow := newTestClient("slow", slowUser)
	fast := newTestClient("fast", bson.NewObjectID())
	// Let the slow registration finish before the test ends: it still reads
	// chatLoadTimeout, which the next test changes (a data race in CI).
	t.Cleanup(func() {
		close(release)
		waitRegistered(t, slow)
	})
	hub.Register <- slow
	hub.Register <- fast
	waitRegistered(t, fast)
}

// A connection whose chats can't be loaded is closed with 1011, so the app
// reconnects, instead of staying open without its chats.
func TestFailedChatLoadClosesTheConnection(t *testing.T) {
	old := chatLoadTimeout
	chatLoadTimeout = 50 * time.Millisecond
	t.Cleanup(func() { chatLoadTimeout = old })

	for name, load := range map[string]func(context.Context, bson.ObjectID) ([]string, error){
		"error": func(context.Context, bson.ObjectID) ([]string, error) { return nil, errors.New("mongo down") },
		"timeout": func(ctx context.Context, _ bson.ObjectID) ([]string, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	} {
		t.Run(name, func(t *testing.T) {
			hub := newTestHub(t)
			stubRegistration(hub)
			hub.loadChats = load
			client := newTestClient("c-"+name, bson.NewObjectID())

			done := make(chan struct{})
			go func() { hub.handleClientRegistration(client); close(done) }()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("registration did not give up on the chat load")
			}

			hub.Mutex.RLock()
			_, registered := hub.Clients[client.ID]
			hub.Mutex.RUnlock()
			if registered {
				t.Fatal("connection without its chats was registered")
			}
			if code, _ := client.CloseReason(); code != websocket.CloseInternalServerErr || !client.IsClosed() {
				t.Fatalf("close code %d, closed %v; want %d, true", code, client.IsClosed(), websocket.CloseInternalServerErr)
			}
		})
	}
}

// The online presence publish (Redis) runs after the hub lock is released, and
// after the chat channels are subscribed: presence reaches the user's chats
// through their channels, so a publish before the subscribe misses them (the
// contract test caught this). Needs Redis: run with TEST_DB_REQUIRED=1.
func TestRegistrationPublishesPresenceWithoutTheLock(t *testing.T) {
	hub := newRedisTestHub(t)
	chatID := bson.NewObjectID().Hex()
	stubRegistration(hub, chatID)
	pm := hub.pubSubManager
	client := newTestClient("c1", bson.NewObjectID())
	GetPresenceInstance().Set(UserPresence{UserID: client.UserID, Status: UserStatus(constants.UserStatusOnline), ClientID: client.ID})
	t.Cleanup(func() { GetPresenceInstance().Delete(client.UserID) })

	t.Cleanup(func() {
		_ = pm.unsubscribeFromChannel(pm.getChatChannel(chatID))
		_ = pm.unsubscribeFromChannel(pm.getUserChannel(client.UserID.Hex()))
	})

	published, lockFree, subscribed := false, false, false
	pm.beforePresencePublish = func(string) {
		published = true
		subscribed = pm.IsSubscribed(pm.getChatChannel(chatID))
		if hub.Mutex.TryLock() {
			lockFree = true
			hub.Mutex.Unlock()
		}
	}
	hub.handleClientRegistration(client)
	if !published || !lockFree || !subscribed {
		t.Fatalf("presence published %v, without the lock %v, after the subscribe %v; want all", published, lockFree, subscribed)
	}
}

// QueueRegister and QueueUnregister give up once the hub has stopped, so a
// connection's goroutine can't wait forever on a hub that won't read.
func TestQueueGivesUpWhenTheHubStops(t *testing.T) {
	hub := newTestHub(t)
	hub.Register = make(chan *models.Client)
	hub.Unregister = make(chan *models.Client)
	hub.Cancel()
	client := newTestClient("c1", bson.NewObjectID())

	done := make(chan struct{})
	go func() {
		hub.QueueRegister(client)
		hub.QueueUnregister(client)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("queueing blocked on a stopped hub")
	}
}

// The user's last connection closing while another one registers must not leave
// the new one without the user channel: the release checks again afterwards.
// Needs Redis: run with TEST_DB_REQUIRED=1.
func TestUserChannelKeptWhenAConnectionRegistersDuringTheRelease(t *testing.T) {
	hub := newRedisTestHub(t)
	pm := hub.pubSubManager
	userID := bson.NewObjectID()
	channel := pm.getUserChannel(userID.Hex())
	t.Cleanup(func() { _ = pm.unsubscribeFromChannel(channel) })
	if err := pm.subscribeToChannel(channel); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	next := newTestClient("next", userID)
	pm.beforeUserRelease = func(bson.ObjectID) {
		addClient(hub, next) // registers after the release decided to run
		if err := pm.SubscribeUserToChannels(userID, nil); err != nil {
			t.Errorf("subscribe next: %v", err)
		}
	}
	pm.UnsubscribeUser(userID)

	if !pm.IsSubscribed(channel) {
		t.Fatal("user channel released while a connection is open")
	}
}

// A connection that closes between its room entry and its user channel subscribe
// must not leave the channel subscribed: the subscribe checks again afterwards.
// Needs Redis: run with TEST_DB_REQUIRED=1.
func TestUserChannelReleasedWhenTheConnectionClosedBeforeTheSubscribe(t *testing.T) {
	hub := newRedisTestHub(t)
	pm := hub.pubSubManager
	client := newTestClient("c1", bson.NewObjectID())
	channel := pm.getUserChannel(client.UserID.Hex())
	t.Cleanup(func() { _ = pm.unsubscribeFromChannel(channel) })

	addClient(hub, client)
	pm.beforeUserSubscribe = func(bson.ObjectID) {
		hub.handleClientUnregistration(client) // nothing subscribed yet to release
		hub.background.Wait()
	}
	if err := pm.SubscribeUserToChannels(client.UserID, nil); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	if pm.IsSubscribed(channel) {
		t.Fatal("user channel subscribed for a user with no connection")
	}
}

// recordPresence records the presence statuses the hub publishes, in order.
func recordPresence(pm *PubSubManager) func() []string {
	var mu sync.Mutex
	var got []string
	pm.beforePresencePublish = func(status string) {
		mu.Lock()
		got = append(got, status)
		mu.Unlock()
	}
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), got...)
	}
}

// Code review of #22: a connection that closes while its registration subscribes
// must not be published as online after its disconnect published it as offline.
// Needs Redis: run with TEST_DB_REQUIRED=1.
func TestRegistrationRacingItsDisconnectEndsOffline(t *testing.T) {
	hub := newRedisTestHub(t)
	stubRegistration(hub)
	pm := hub.pubSubManager
	client := newTestClient("c1", bson.NewObjectID())
	t.Cleanup(func() { _ = pm.unsubscribeFromChannel(pm.getUserChannel(client.UserID.Hex())) })
	GetPresenceInstance().Set(UserPresence{UserID: client.UserID, Status: UserStatus(constants.UserStatusOnline), ClientID: client.ID})
	t.Cleanup(func() { GetPresenceInstance().Delete(client.UserID) })
	published := recordPresence(pm)

	pm.beforeUserSubscribe = func(bson.ObjectID) {
		unregisterAndWait(hub, client) // the socket drops during the subscribe
	}
	hub.handleClientRegistration(client)

	if got := published(); len(got) == 0 || got[len(got)-1] != string(constants.UserStatusOffline) {
		t.Fatalf("published %v; want offline last", got)
	}
	if client.TrySend(models.WebSocketMessage{}) {
		t.Fatal("closed connection can still be sent to")
	}
}

// Code review of #22: while a second connection of the user is registering, the
// first one closing isn't the user's last: their presence, user channel and online
// status stay, and the new connection announces online once registered.
func TestDisconnectDuringAnotherRegistrationIsNotTheLast(t *testing.T) {
	hub := newTestHub(t)
	stubRegistration(hub)
	pm := hub.pubSubManager
	userID := bson.NewObjectID()
	first, second := newTestClient("first", userID), newTestClient("second", userID)
	addClient(hub, first)
	stubSubscriptions(hub, pm.getUserChannel(userID.Hex()))
	published := recordPresence(pm)

	hub.QueueRegister(second) // queued, not handled yet
	GetPresenceInstance().Set(UserPresence{UserID: userID, Status: UserStatus(constants.UserStatusOnline), ClientID: second.ID})
	t.Cleanup(func() { GetPresenceInstance().Delete(userID) })

	unregisterAndWait(hub, first)
	if _, ok := GetPresenceInstance().Get(userID); !ok {
		t.Fatal("presence of the registering connection was deleted")
	}
	if !pm.IsSubscribed(pm.getUserChannel(userID.Hex())) {
		t.Fatal("user channel released while a connection is registering")
	}
	if got := published(); len(got) != 0 {
		t.Fatalf("published %v while a connection is registering; want nothing", got)
	}

	hub.handleClientRegistration(<-hub.Register)
	if got := published(); len(got) != 1 || got[0] != string(constants.UserStatusOnline) {
		t.Fatalf("published %v; want [online]", got)
	}
}

// Code review of #22: a reconnect storm runs at most maxRegistrations chat loads
// at once, so MongoDB isn't flooded.
func TestRegistrationsAreBounded(t *testing.T) {
	old := maxRegistrations
	maxRegistrations = 2
	t.Cleanup(func() { maxRegistrations = old })
	hub := newTestHub(t)
	stubRegistration(hub)
	var mu sync.Mutex
	running, peak := 0, 0
	release := make(chan struct{})
	hub.loadChats = func(context.Context, bson.ObjectID) ([]string, error) {
		mu.Lock()
		running++
		peak = max(peak, running)
		mu.Unlock()
		<-release
		mu.Lock()
		running--
		mu.Unlock()
		return nil, nil
	}

	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hub.handleClientRegistration(newTestClient(fmt.Sprint("c", i), bson.NewObjectID()))
		}()
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	if peak > 2 {
		t.Fatalf("%d chat loads ran at once; want at most 2", peak)
	}
}

// Code review of #22: a connection whose user channel can't be subscribed gets no
// direct events, so it is closed with 1011 like a failed chat load.
func TestFailedSubscribeClosesTheConnection(t *testing.T) {
	hub := newTestHub(t) // its Redis refuses every connection
	stubRegistration(hub, "chat1")
	client := newTestClient("c1", bson.NewObjectID())

	hub.handleClientRegistration(client)

	if code, _ := client.CloseReason(); code != websocket.CloseInternalServerErr {
		t.Fatalf("close code %d; want %d", code, websocket.CloseInternalServerErr)
	}
}

// Live check of #22: a user already online is not announced again by each new
// connection. Twenty connections of one user at once used to send each other
// twenty presence events, filling their queues so some were dropped as slow and
// others never got the welcome. Needs Redis: run with TEST_DB_REQUIRED=1.
func TestOnlineIsAnnouncedOncePerUser(t *testing.T) {
	hub := newRedisTestHub(t)
	stubRegistration(hub)
	pm := hub.pubSubManager
	userID := bson.NewObjectID()
	t.Cleanup(func() { _ = pm.unsubscribeFromChannel(pm.getUserChannel(userID.Hex())) })
	GetPresenceInstance().Set(UserPresence{UserID: userID, Status: UserStatus(constants.UserStatusOnline)})
	t.Cleanup(func() { GetPresenceInstance().Delete(userID) })
	published := recordPresence(pm)

	first, second := newTestClient("first", userID), newTestClient("second", userID)
	hub.handleClientRegistration(first)
	hub.handleClientRegistration(second)
	unregisterAndWait(hub, second)
	if got := published(); len(got) != 1 || got[0] != string(constants.UserStatusOnline) {
		t.Fatalf("published %v; want [online] once", got)
	}

	unregisterAndWait(hub, first)
	third := newTestClient("third", userID)
	GetPresenceInstance().Set(UserPresence{UserID: userID, Status: UserStatus(constants.UserStatusOnline), ClientID: third.ID})
	hub.handleClientRegistration(third)
	want := []string{"online", "offline", "online"}
	if got := published(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("published %v; want %v", got, want)
	}
}
