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
