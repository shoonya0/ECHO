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
		Clients:         make(map[string]*models.Client),
		ChatClients:     make(map[string]map[string]*models.Client),
		UserClients:     make(map[string]map[string]*models.Client),
		UserInfoCache:   make(map[string]*models.UserDisplayInfo),
		CacheExpiry:     make(map[string]time.Time),
		UserActiveChats: make(map[string][]string),
		Register:        make(chan *models.Client, 8),
		Unregister:      make(chan *models.Client, 8),
	}
	base.Ctx, base.Cancel = context.WithCancel(context.Background())
	t.Cleanup(base.Cancel)

	return &Hub{
		Hub:           base,
		pubSubManager: NewPubSubManager(redisClient, base, logger, "test"),
		redisClient:   redisClient,
		logger:        logger,
		instanceID:    "test",
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
		if hub.ChatClients[chatID] == nil {
			hub.ChatClients[chatID] = make(map[string]*models.Client)
		}
		hub.ChatClients[chatID][client.ID] = client
	}
	if len(chatIDs) > 0 {
		hub.UserActiveChats[userID] = append([]string(nil), chatIDs...)
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

// A disconnect used to change ChatClients and UserActiveChats with no lock held,
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
	message := &models.WebSocketMessage{Type: "presence", UserID: userID}

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

	hub.handleClientUnregistration(closing)
	hub.handleClientUnregistration(closing) // a second unregister must do nothing

	if _, open := <-closing.Send; open {
		t.Fatal("closing client's Send channel should be closed")
	}
	if _, ok := hub.Clients[closing.ID]; ok {
		t.Fatal("closing client is still in Clients")
	}
	if _, ok := hub.ChatClients["chat1"][closing.ID]; ok {
		t.Fatal("closing client is still in the chat")
	}
	if _, ok := hub.UserClients[userID.Hex()][staying.ID]; !ok {
		t.Fatal("other connection was removed from UserClients")
	}
	if _, ok := hub.ChatClients["chat1"][staying.ID]; !ok {
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

	hub.handleClientUnregistration(client)

	if _, ok := hub.UserClients[client.UserID.Hex()]; ok {
		t.Fatal("user is still in UserClients")
	}
	if _, ok := hub.UserActiveChats[client.UserID.Hex()]; ok {
		t.Fatal("user is still in UserActiveChats")
	}
	if _, ok := hub.ChatClients["chat1"]; ok {
		t.Fatal("empty chat is still in ChatClients")
	}
}
