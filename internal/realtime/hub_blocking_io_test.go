package realtime

import (
	"context"
	"errors"
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
	t.Cleanup(func() { close(release) })
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
	pm.beforePresencePublish = func() {
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
