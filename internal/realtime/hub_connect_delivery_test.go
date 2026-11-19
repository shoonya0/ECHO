package realtime

import (
	"context"
	"sync"
	"testing"

	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// connectHub is a Redis-backed hub whose chat loads return each list of loads in
// turn (the last one from then on), calling onLoad with the load's number first.
func connectHub(t *testing.T, client *models.Client, onLoad func(n int), loads ...[]string) *Hub {
	t.Helper()
	hub := newRedisTestHub(t)
	stubRegistration(hub)
	pm := hub.pubSubManager
	t.Cleanup(func() { _ = pm.unsubscribeFromChannel(pm.getUserChannel(client.UserID.Hex())) })
	var mu sync.Mutex
	n := 0
	hub.loadChats = func(context.Context, bson.ObjectID) ([]string, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		if onLoad != nil {
			onLoad(n)
		}
		return loads[min(n, len(loads))-1], nil
	}
	return hub
}

// #168: a chat the user gained after the connection's chats were read, but before
// its user channel was subscribed, was missed: the chat_added message went out
// while this server didn't listen yet. The chats are read again once it listens.
func TestChatAddedDuringConnectIsJoined(t *testing.T) {
	client := newTestClient("c1", bson.NewObjectID())
	hub := connectHub(t, client, nil, []string{"chat1"}, []string{"chat1", "chat2"})

	hub.handleClientRegistration(client)
	t.Cleanup(func() { unregisterAndWait(hub, client) })

	if !inRoomUnlocked(hub, "chat2", client.ID) {
		t.Fatal("the connection isn't in the chat added while it connected")
	}
}

// #168: the welcome was sent last, so messages from the connection's rooms could
// fill its send queue first and the welcome was dropped. It is queued before the
// connection is in any room.
func TestWelcomeIsQueuedBeforeRoomMessages(t *testing.T) {
	client := newTestClient("c1", bson.NewObjectID()) // a queue of one
	fill := func(n int) {
		if n > 1 { // after the rooms are joined: a room's message takes the slot
			client.TrySend(models.WebSocketMessage{Type: "message"})
		}
	}
	hub := connectHub(t, client, fill, []string{"chat1"})

	hub.handleClientRegistration(client)
	t.Cleanup(func() { unregisterAndWait(hub, client) })

	select {
	case msg := <-client.Send:
		data, _ := msg.Data.(map[string]interface{})
		if data["status"] != "connected" {
			t.Fatalf("first message %+v, want the welcome", msg)
		}
	default:
		t.Fatal("nothing queued, want the welcome")
	}
}
