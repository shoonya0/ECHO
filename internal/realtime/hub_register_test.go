package realtime

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// These tests guard #54: the hub can handle a connection's disconnect before
// its registration, which then must not register a connection that is gone.

// stubRegistration makes registration run without MongoDB: the user follows
// chatIDs, and every session check passes.
func stubRegistration(hub *Hub, chatIDs ...string) {
	hub.loadChats = func(context.Context, bson.ObjectID) ([]string, error) { return chatIDs, nil }
	hub.checkSession = func(context.Context, bson.ObjectID, string, time.Time) error { return nil }
}

func TestDisconnectHandledBeforeRegistrationIsNotRegistered(t *testing.T) {
	hub := newTestHub(t)
	stubRegistration(hub, "chat1")
	client := newTestClient("gone", bson.NewObjectID())

	hub.handleClientUnregistration(client)
	hub.handleClientRegistration(client)

	hub.Mutex.RLock()
	_, inClients := hub.Clients[client.ID]
	_, inUsers := hub.UserClients[client.UserID.Hex()]
	_, inRoom := hub.ChatClients["chat1"][client.ID]
	_, hasRooms := hub.ClientChats[client.ID]
	hub.Mutex.RUnlock()
	if inClients || inUsers || inRoom || hasRooms {
		t.Fatalf("closed connection registered: clients=%v users=%v room=%v rooms=%v", inClients, inUsers, inRoom, hasRooms)
	}
	if !client.IsClosed() {
		t.Fatal("Send of the closed connection is still open")
	}
}

// The usual order still registers the connection and then removes it.
func TestRegistrationThenDisconnect(t *testing.T) {
	hub := newTestHub(t)
	stubRegistration(hub, "chat1")
	client := newTestClient("c1", bson.NewObjectID())

	hub.handleClientRegistration(client)
	if !inRoomUnlocked(hub, "chat1", client.ID) {
		t.Fatal("registered connection is not in its chat")
	}

	hub.handleClientUnregistration(client)
	if inRoomUnlocked(hub, "chat1", client.ID) || !client.IsClosed() {
		t.Fatal("disconnect did not remove and close the connection")
	}
}

// The controller stores presence for a connection before the hub registers it.
// A disconnect handled first must remove that entry: the skipped registration
// leaves nothing else to remove it (#54).
func TestDisconnectBeforeRegistrationRemovesItsPresence(t *testing.T) {
	hub := newTestHub(t)
	stubRegistration(hub)
	client := newTestClient("gone", bson.NewObjectID())
	GetPresenceInstance().Set(UserPresence{UserID: client.UserID, Status: "online", ClientID: client.ID})
	t.Cleanup(func() { GetPresenceInstance().Delete(client.UserID) })

	hub.handleClientUnregistration(client)
	hub.handleClientRegistration(client)

	if _, ok := GetPresenceInstance().Get(client.UserID); ok {
		t.Fatal("presence of a connection that never registered is still stored")
	}
}

// Presence stored by another connection of the user stays: that connection is
// still open, or will register.
func TestDisconnectBeforeRegistrationKeepsOtherConnectionsPresence(t *testing.T) {
	hub := newTestHub(t)
	userID := bson.NewObjectID()
	gone := newTestClient("gone", userID)
	GetPresenceInstance().Set(UserPresence{UserID: userID, Status: "online", ClientID: "newer"})
	t.Cleanup(func() { GetPresenceInstance().Delete(userID) })

	hub.handleClientUnregistration(gone)

	if p, ok := GetPresenceInstance().Get(userID); !ok || p.ClientID != "newer" {
		t.Fatalf("presence of the other connection was removed: %+v, %v", p, ok)
	}
}

// The controller reads a connection's requests only after the hub has handled
// its registration, so a join_chat can't run before the connection's rooms are
// set and be dropped (#122).
func TestWaitRegisteredReturnsOnceRegistered(t *testing.T) {
	hub := newTestHub(t)
	stubRegistration(hub, "chat1")
	go hub.runHub()
	client := newTestClient("c1", bson.NewObjectID())

	hub.Register <- client
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := hub.WaitRegistered(ctx, client); err != nil {
		t.Fatalf("WaitRegistered: %v", err)
	}

	hub.Mutex.RLock()
	_, registered := hub.Clients[client.ID]
	hub.Mutex.RUnlock()
	if !registered || !inRoomUnlocked(hub, "chat1", client.ID) {
		t.Fatal("WaitRegistered returned before the registration was done")
	}
}

// A registration skipped because the connection already closed (#54) still
// ends the wait.
func TestWaitRegisteredReturnsForSkippedRegistration(t *testing.T) {
	hub := newTestHub(t)
	stubRegistration(hub)
	client := newTestClient("gone", bson.NewObjectID())

	hub.handleClientUnregistration(client)
	hub.handleClientRegistration(client)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := hub.WaitRegistered(ctx, client); err != nil {
		t.Fatalf("WaitRegistered: %v", err)
	}
}

// A hub that never handles the registration doesn't block the connection
// forever.
func TestWaitRegisteredStopsAtTimeout(t *testing.T) {
	hub := newTestHub(t)
	client := newTestClient("c1", bson.NewObjectID())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := hub.WaitRegistered(ctx, client); err == nil {
		t.Fatal("WaitRegistered returned nil without a registration")
	}
}
