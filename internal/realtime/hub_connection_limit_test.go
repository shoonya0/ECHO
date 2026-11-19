package realtime

import (
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #40: one account holds at most MaxConnectionsPerUser connections on a server,
// registered and queued ones together, so it can't multiply its request limit or
// memory by opening more sockets. Other accounts aren't affected.
func TestQueueRegisterRefusesOverTheAccountLimit(t *testing.T) {
	hub := newTestHub(t)
	user := bson.NewObjectID()
	for i := 0; i < MaxConnectionsPerUser-1; i++ {
		addClient(hub, newTestClient(fmt.Sprintf("c%d", i), user))
	}
	// One more queued but not yet registered still counts.
	hub.Mutex.Lock()
	addPending(hub.Hub, newTestClient("queued", user))
	hub.Mutex.Unlock()

	if n := hub.ConnectionCount(user.Hex()); n != MaxConnectionsPerUser {
		t.Fatalf("ConnectionCount = %d, want %d", n, MaxConnectionsPerUser)
	}

	extra := newTestClient("extra", user)
	t.Cleanup(func() { GetPresenceInstance().Delete(user) })
	if hub.QueueRegister(extra, &UserPresence{UserID: user, Status: "online", ClientID: extra.ID}) {
		t.Fatal("QueueRegister accepted a connection over the limit")
	}
	hub.Mutex.RLock()
	_, pending := hub.PendingClients[user.Hex()][extra.ID]
	hub.Mutex.RUnlock()
	if pending {
		t.Fatal("the refused connection was left queued")
	}
	if p, ok := GetPresenceInstance().Get(user); ok && p.ClientID == extra.ID {
		t.Fatal("the refused connection's presence was stored")
	}

	other := bson.NewObjectID()
	t.Cleanup(func() { GetPresenceInstance().Delete(other) })
	if !hub.QueueRegister(newTestClient("other", other), nil) {
		t.Fatal("another account was refused")
	}
}
