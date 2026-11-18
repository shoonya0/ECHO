package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// These tests guard #31: online and offline reach the members of the user's
// chats, on every server, once per connection. They need Redis: run with
// TEST_DB_REQUIRED=1.

// newPresenceHub returns a hub on redisClient that listens to presence:global,
// as a started server does.
func newPresenceHub(t *testing.T, redisClient *redis.Client, instanceID string) *Hub {
	t.Helper()
	hub := newTestHub(t)
	hub.redisClient = redisClient
	hub.instanceID = instanceID
	hub.pubSubManager = NewPubSubManager(redisClient, hub.Hub, hub.logger, instanceID)
	t.Cleanup(hub.pubSubManager.cancel)
	if err := hub.pubSubManager.subscribeToChannel(PresenceChannel); err != nil {
		t.Fatalf("subscribe presence: %v", err)
	}
	t.Cleanup(func() { _ = hub.pubSubManager.unsubscribeFromChannel(PresenceChannel) })
	return hub
}

// connectOnline adds client to hub's rooms and announces it, as a registration
// does once the controller stored the connection's presence.
func connectOnline(t *testing.T, hub *Hub, client *models.Client, chatIDs ...string) {
	t.Helper()
	GetPresenceInstance().Set(UserPresence{UserID: client.UserID, Status: UserStatus(constants.UserStatusOnline), ClientID: client.ID})
	t.Cleanup(func() { GetPresenceInstance().Delete(client.UserID) })
	t.Cleanup(func() { _ = hub.redisClient.Del(context.Background(), onlineKey(client.UserID.Hex())).Err() })
	addClient(hub, client, chatIDs...)
	if !hub.publishOnline(client) {
		t.Fatalf("connection %s isn't registered", client.ID)
	}
}

// nextPresence waits for the next presence event client gets about userID,
// skipping other events.
func nextPresence(t *testing.T, client *models.Client, userID bson.ObjectID) map[string]any {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case msg := <-client.Send:
			data, _ := msg.Data.(map[string]any)
			if msg.Type == models.WSMessageTypePresence && data["userId"] == userID.Hex() {
				return data
			}
		case <-deadline:
			t.Fatalf("connection %s got no presence event for %s", client.ID, userID.Hex())
			return nil
		}
	}
}

// noPresence fails if client gets a presence event about userID within wait.
func noPresence(t *testing.T, client *models.Client, userID bson.ObjectID, wait time.Duration) {
	t.Helper()
	deadline := time.After(wait)
	for {
		select {
		case msg := <-client.Send:
			data, _ := msg.Data.(map[string]any)
			if msg.Type == models.WSMessageTypePresence && data["userId"] == userID.Hex() {
				t.Fatalf("connection %s got presence %v for %s, want none", client.ID, data["status"], userID.Hex())
			}
		case <-deadline:
			return
		}
	}
}

func wantStatus(t *testing.T, data map[string]any, status constants.UserStatus) {
	t.Helper()
	if data["status"] != string(status) {
		t.Fatalf("status = %v, want %s", data["status"], status)
	}
}

// The user's last connection closing tells the members of their chats. Before
// #31, the server looked up the chats after the connection had left them, so
// offline reached nobody.
func TestOfflineReachesChatMembers(t *testing.T) {
	hub := newPresenceHub(t, testutil.UseTestRedis(t), "one")
	chat := bson.NewObjectID().Hex()
	a1 := bufferedClient("a1", bson.NewObjectID())
	b1 := bufferedClient("b1", bson.NewObjectID())
	connectOnline(t, hub, b1, chat)
	connectOnline(t, hub, a1, chat)
	wantStatus(t, nextPresence(t, b1, a1.UserID), constants.UserStatusOnline)

	unregisterAndWait(hub, a1)
	wantStatus(t, nextPresence(t, b1, a1.UserID), constants.UserStatusOffline)
}

// A member connected to another server gets both events. Before #31, only the
// server holding the user knew their chats.
func TestPresenceReachesOtherServers(t *testing.T) {
	redisClient := testutil.UseTestRedis(t)
	hub1 := newPresenceHub(t, redisClient, "one")
	hub2 := newPresenceHub(t, redisClient, "two")
	chat := bson.NewObjectID().Hex()
	a1 := bufferedClient("a1", bson.NewObjectID())
	b2 := bufferedClient("b2", bson.NewObjectID())
	addClient(hub2, b2, chat)

	connectOnline(t, hub1, a1, chat)
	wantStatus(t, nextPresence(t, b2, a1.UserID), constants.UserStatusOnline)
	unregisterAndWait(hub1, a1)
	wantStatus(t, nextPresence(t, b2, a1.UserID), constants.UserStatusOffline)
}

// Closing the user's last connection on one server isn't offline while they
// are still connected to another.
func TestNoOfflineWhileOnlineOnAnotherServer(t *testing.T) {
	redisClient := testutil.UseTestRedis(t)
	hub1 := newPresenceHub(t, redisClient, "one")
	hub2 := newPresenceHub(t, redisClient, "two")
	chat := bson.NewObjectID().Hex()
	userA := bson.NewObjectID()
	a1, a2 := bufferedClient("a1", userA), bufferedClient("a2", userA)
	b1 := bufferedClient("b1", bson.NewObjectID())
	addClient(hub1, b1, chat)
	connectOnline(t, hub1, a1, chat)
	connectOnline(t, hub2, a2, chat)
	wantStatus(t, nextPresence(t, b1, userA), constants.UserStatusOnline)

	unregisterAndWait(hub1, a1)
	// The second online, from hub 2, may still arrive; no offline may.
	deadline := time.After(300 * time.Millisecond)
	for done := false; !done; {
		select {
		case msg := <-b1.Send:
			if data, _ := msg.Data.(map[string]any); msg.Type == models.WSMessageTypePresence && data["status"] != string(constants.UserStatusOnline) {
				t.Fatalf("got presence %v while the user is connected to hub 2", data["status"])
			}
		case <-deadline:
			done = true
		}
	}

	unregisterAndWait(hub2, a2)
	wantStatus(t, nextPresence(t, b1, userA), constants.UserStatusOffline)
}

// A member who shares several chats with the user gets one event per
// connection, without the chat list the servers use to route it.
func TestPresenceOncePerConnection(t *testing.T) {
	hub := newPresenceHub(t, testutil.UseTestRedis(t), "one")
	chats := []string{bson.NewObjectID().Hex(), bson.NewObjectID().Hex(), bson.NewObjectID().Hex()}
	a1 := bufferedClient("a1", bson.NewObjectID())
	b1 := bufferedClient("b1", bson.NewObjectID())
	addClient(hub, b1, chats...)

	connectOnline(t, hub, a1, chats...)
	data := nextPresence(t, b1, a1.UserID)
	if _, leaked := data["chats"]; leaked {
		t.Fatalf("the event sent to clients lists chats: %v", data["chats"])
	}
	noPresence(t, b1, a1.UserID, 300*time.Millisecond)
}

// A server that stopped without removing its entry doesn't keep the user
// online once the entry has expired.
func TestExpiredOnlineEntryIsNotOnlineElsewhere(t *testing.T) {
	redisClient := testutil.UseTestRedis(t)
	hub := newPresenceHub(t, redisClient, "one")
	other := newPresenceHub(t, redisClient, "crashed")
	userID := bson.NewObjectID().Hex()
	t.Cleanup(func() { _ = redisClient.Del(context.Background(), onlineKey(userID)).Err() })

	prev := presenceTTL
	presenceTTL = 50 * time.Millisecond
	t.Cleanup(func() { presenceTTL = prev })

	other.markOnline(userID)
	hub.markOnline(userID)
	if elsewhere, err := hub.markOffline(userID); err != nil || !elsewhere {
		t.Fatalf("live entry: online elsewhere = %v, %v; want true", elsewhere, err)
	}
	time.Sleep(100 * time.Millisecond)
	if elsewhere, err := hub.markOffline(userID); err != nil || elsewhere {
		t.Fatalf("expired entry: online elsewhere = %v, %v; want false", elsewhere, err)
	}
}
