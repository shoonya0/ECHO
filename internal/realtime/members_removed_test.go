package realtime

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// #14: a member removed from a group must stop getting its live messages at
// once, not when their connection next reconnects.

func bufferedClient(id string, userID bson.ObjectID) *models.Client {
	return &models.Client{ID: id, UserID: userID, Send: make(chan models.WebSocketMessage, 8)}
}

// overRedis returns message as a subscriber gets it: marshalled and decoded
// again, so Data holds []any, not []string.
func overRedis(t *testing.T, message models.WebSocketMessage) *models.WebSocketMessage {
	t.Helper()
	data, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded models.WebSocketMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return &decoded
}

func inRoom(hub *Hub, chatID, clientID string) bool {
	hub.Mutex.RLock()
	defer hub.Mutex.RUnlock()
	_, ok := hub.ChatClients[chatID][clientID]
	return ok
}

func TestMembersRemovedMessage(t *testing.T) {
	const chat = "chat1"
	setup := func(t *testing.T) (*Hub, *models.Client, *models.Client, *models.Client) {
		hub := newTestHub(t)
		stay, gone := bson.NewObjectID(), bson.NewObjectID()
		a1, b1, b2 := bufferedClient("a1", stay), bufferedClient("b1", gone), bufferedClient("b2", gone)
		addClient(hub, a1, chat, "other")
		addClient(hub, b1, chat, "other")
		addClient(hub, b2, chat, "other")
		return hub, a1, b1, b2
	}

	t.Run("removes every connection of the listed users and isn't forwarded", func(t *testing.T) {
		hub, a1, b1, b2 := setup(t)

		hub.pubSubManager.routeMessage(ChatChannelPrefix+chat, overRedis(t, membersRemovedMessage(chat, []string{b1.UserID.Hex()})))

		if inRoom(hub, chat, b1.ID) || inRoom(hub, chat, b2.ID) {
			t.Fatal("a removed user's connection is still in the room")
		}
		if !inRoom(hub, chat, a1.ID) {
			t.Fatal("a member who wasn't removed left the room")
		}
		if !inRoom(hub, "other", b1.ID) || !inRoom(hub, "other", b2.ID) {
			t.Fatal("the removed user also left their other chats")
		}
		hub.Mutex.RLock()
		active := hub.UserActiveChats[b1.UserID.Hex()]
		hub.Mutex.RUnlock()
		if len(active) != 1 || active[0] != "other" {
			t.Fatalf("removed user's active chats = %v, want [other]", active)
		}
		if n := len(a1.Send); n != 0 {
			t.Fatalf("the internal message reached a client (%d queued)", n)
		}

		// Later messages reach only the members still in the room.
		hub.pubSubManager.routeMessage(ChatChannelPrefix+chat, &models.WebSocketMessage{Type: models.WSMessageTypeChat, ChatID: chat})
		if len(a1.Send) != 1 || len(b1.Send) != 0 || len(b2.Send) != 0 {
			t.Fatalf("queued after removal: a1=%d b1=%d b2=%d, want 1 0 0", len(a1.Send), len(b1.Send), len(b2.Send))
		}
	})

	t.Run("bad data removes nobody", func(t *testing.T) {
		for _, data := range []any{"not a list", []any{1, nil, true}, nil} {
			hub, a1, b1, b2 := setup(t)
			hub.pubSubManager.routeMessage(ChatChannelPrefix+chat, overRedis(t, models.WebSocketMessage{Type: membersRemovedType, ChatID: chat, Data: data}))
			for _, c := range []*models.Client{a1, b1, b2} {
				if !inRoom(hub, chat, c.ID) {
					t.Fatalf("data %v removed %s", data, c.ID)
				}
				if len(c.Send) != 0 {
					t.Fatalf("data %v: the internal message reached %s", data, c.ID)
				}
			}
		}
	})

	t.Run("a user with no connection in the room changes nothing", func(t *testing.T) {
		hub, a1, b1, b2 := setup(t)
		hub.pubSubManager.routeMessage(ChatChannelPrefix+chat, overRedis(t, membersRemovedMessage(chat, []string{bson.NewObjectID().Hex()})))
		for _, c := range []*models.Client{a1, b1, b2} {
			if !inRoom(hub, chat, c.ID) {
				t.Fatalf("%s left the room", c.ID)
			}
		}
	})

	t.Run("a failed publish still removes this instance's connections", func(t *testing.T) {
		hub, a1, b1, b2 := setup(t)
		// newTestHub's Redis refuses every publish.
		if err := hub.RemoveMembersFromChat(chat, []string{b1.UserID.Hex()}); err == nil {
			t.Fatal("err = nil, want the publish error")
		}
		if inRoom(hub, chat, b1.ID) || inRoom(hub, chat, b2.ID) || !inRoom(hub, chat, a1.ID) {
			t.Fatal("the local removal didn't run")
		}
	})
}

// Through real Redis, the removal message keeps its place in the chat channel:
// the removed user gets the chat_updated published before it, and nothing after.
func TestMembersRemovedKeepsChannelOrder(t *testing.T) {
	redisClient := testutil.UseTestRedis(t)
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	chat := bson.NewObjectID().Hex()
	hub := newTestHub(t)
	receiver := NewPubSubManager(redisClient, hub.Hub, logger, "receiver")
	t.Cleanup(receiver.cancel)
	if err := receiver.subscribeToChannel(ChatChannelPrefix + chat); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() { _ = receiver.unsubscribeFromChannel(ChatChannelPrefix + chat) })

	a1, b1 := bufferedClient("a1", bson.NewObjectID()), bufferedClient("b1", bson.NewObjectID())
	addClient(hub, a1, chat)
	addClient(hub, b1, chat)

	sender := NewPubSubManager(redisClient, &models.Hub{}, logger, "sender")
	t.Cleanup(sender.cancel)
	for _, m := range []models.WebSocketMessage{
		{Type: models.WSMessageTypeChatUpdated, ChatID: chat},
		membersRemovedMessage(chat, []string{b1.UserID.Hex()}),
		{Type: models.WSMessageTypeChat, ChatID: chat},
	} {
		if err := sender.PublishToChat(chat, &m); err != nil {
			t.Fatalf("publish %s: %v", m.Type, err)
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for len(a1.Send) < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := receivedTypes(a1); len(got) != 2 || got[0] != models.WSMessageTypeChatUpdated || got[1] != models.WSMessageTypeChat {
		t.Fatalf("staying member got %v, want [chat_updated message]", got)
	}
	if got := receivedTypes(b1); len(got) != 1 || got[0] != models.WSMessageTypeChatUpdated {
		t.Fatalf("removed member got %v, want [chat_updated]", got)
	}
}

func receivedTypes(c *models.Client) []string {
	var types []string
	for len(c.Send) > 0 {
		types = append(types, (<-c.Send).Type)
	}
	return types
}

// The removal writes the hub's maps while clients join, leave and receive
// broadcasts. It only proves anything under `go test -race` (#19).
func TestMembersRemovedRacesWithJoinsAndBroadcasts(t *testing.T) {
	hub := newTestHub(t)
	gone := bson.NewObjectID()
	removal := membersRemovedMessage("chat1", []string{gone.Hex()})
	broadcast := &models.WebSocketMessage{Type: models.WSMessageTypeChat}

	runTogether(
		func(i int) {
			c := newTestClient("g"+string(rune('a'+i%26)), gone)
			addClient(hub, c, "chat1")
			hub.handleClientUnregistration(c)
		},
		func(int) { hub.pubSubManager.handleChatMessage("chat1", &removal) },
		func(int) {
			hub.pubSubManager.handleChatMessage("chat1", broadcast)
			for len(hub.Unregister) > 0 {
				<-hub.Unregister
			}
		},
	)
}
