package realtime

import (
	"fmt"
	"testing"

	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #49: a chat a user gains after connecting (a new direct chat, a new group, an
// invite join) is followed by their open connections at once, not after they
// reconnect.

func TestChatAddedMessage(t *testing.T) {
	chat := bson.NewObjectID().Hex()

	t.Run("puts every open connection of the user in the room and isn't forwarded", func(t *testing.T) {
		hub := newTestHub(t)
		gains, other := bson.NewObjectID(), bson.NewObjectID()
		g1, g2, o1 := bufferedClient("g1", gains), bufferedClient("g2", gains), bufferedClient("o1", other)
		addClient(hub, g1, "old")
		addClient(hub, g2)
		addClient(hub, o1)

		hub.pubSubManager.routeMessage(UserChannelPrefix+gains.Hex(), overRedis(t, chatAddedMessage(chat)))

		for _, c := range []*models.Client{g1, g2} {
			if !inRoom(hub, chat, c.ID) {
				t.Errorf("%s isn't in the room of a chat its user gained", c.ID)
			}
			if len(c.Send) != 0 {
				t.Errorf("%s got the hub-internal message", c.ID)
			}
		}
		if !inRoom(hub, "old", g1.ID) {
			t.Error("g1 left a room it was in")
		}
		if inRoom(hub, chat, o1.ID) {
			t.Error("another user's connection joined the room")
		}
	})

	t.Run("skips a connection that is closed or no longer registered", func(t *testing.T) {
		hub := newTestHub(t)
		user := bson.NewObjectID()
		closed, gone := bufferedClient("closed", user), bufferedClient("gone", user)
		addClient(hub, closed)
		closed.CloseSend()
		addClient(hub, gone)
		hub.Mutex.Lock()
		delete(hub.Clients, gone.ID) // unregistered, not yet out of UserClients
		hub.Mutex.Unlock()

		hub.pubSubManager.routeMessage(UserChannelPrefix+user.Hex(), overRedis(t, chatAddedMessage(chat)))

		if inRoom(hub, chat, closed.ID) || inRoom(hub, chat, gone.ID) {
			t.Error("a connection that is gone joined the room; nothing would take it out again")
		}
	})

	t.Run("without Redis, this instance's connections still follow the chat", func(t *testing.T) {
		hub := newTestHub(t) // its Redis is unreachable
		user := bson.NewObjectID()
		c := bufferedClient("c", user)
		addClient(hub, c)

		if err := hub.AddMembersToChat(chat, []string{user.Hex()}); err == nil {
			t.Error("a failed publish wasn't reported")
		}
		if !inRoom(hub, chat, c.ID) {
			t.Error("the local connection didn't follow the chat")
		}
	})

	t.Run("a message without a chat ID is ignored", func(t *testing.T) {
		hub := newTestHub(t)
		user := bson.NewObjectID()
		c := bufferedClient("c", user)
		addClient(hub, c)

		hub.pubSubManager.routeMessage(UserChannelPrefix+user.Hex(), overRedis(t, chatAddedMessage("")))

		hub.Mutex.RLock()
		rooms := len(hub.ClientChats[c.ID])
		hub.Mutex.RUnlock()
		if rooms != 0 || len(c.Send) != 0 {
			t.Errorf("rooms = %d, queued = %d; want both 0", rooms, len(c.Send))
		}
	})
}

// Run with -race: adds race connections arriving and leaving.
func TestChatAddedRace(t *testing.T) {
	hub := newTestHub(t)
	chat := bson.NewObjectID().Hex()
	user := bson.NewObjectID()

	runTogether(
		func(int) { hub.pubSubManager.addUserToRoom(user.Hex(), chat) },
		func(i int) { addClient(hub, bufferedClient(fmt.Sprintf("c%d", i), user)) },
		func(int) {
			hub.Mutex.Lock()
			for id, client := range hub.UserClients[user.Hex()] {
				removeClientFromRoom(hub.Hub, client, chat)
				delete(hub.Clients, id)
				delete(hub.UserClients[user.Hex()], id)
				break
			}
			hub.Mutex.Unlock()
		},
	)
}
