package realtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// newRedisTestHub is newTestHub with a pub/sub manager on the test Redis.
func newRedisTestHub(t *testing.T) *Hub {
	t.Helper()
	redisClient := testutil.UseTestRedis(t)
	hub := newTestHub(t)
	hub.redisClient = redisClient
	hub.pubSubManager = NewPubSubManager(redisClient, hub.Hub, hub.logger, "test")
	t.Cleanup(hub.pubSubManager.cancel)
	return hub
}

// A connection that already follows a chat and joins it again is not announced
// again: join_chat by chatId would otherwise let a member flood a group with
// join events (#48). Needs Redis: run with TEST_DB_REQUIRED=1.
func TestRepeatedJoinIsAnnouncedOnce(t *testing.T) {
	hub := newRedisTestHub(t)
	pm := hub.pubSubManager

	chatID := bson.NewObjectID().Hex()
	client := newTestClient("c1", bson.NewObjectID())
	addClient(hub, client)
	t.Cleanup(func() { _ = pm.unsubscribeFromChannel(pm.getChatChannel(chatID)) })

	observer := hub.redisClient.Subscribe(context.Background(), pm.getChatChannel(chatID))
	t.Cleanup(func() { _ = observer.Close() })
	if _, err := observer.Receive(context.Background()); err != nil {
		t.Fatalf("observer subscribe: %v", err)
	}

	for i := range 3 {
		if err := hub.JoinChat(context.Background(), client, chatID); err != nil {
			t.Fatalf("join %d: %v", i+1, err)
		}
	}

	if _, err := observer.ReceiveTimeout(context.Background(), 2*time.Second); err != nil {
		t.Fatalf("the first join was not announced: %v", err)
	}
	if msg, err := observer.ReceiveTimeout(context.Background(), 200*time.Millisecond); err == nil {
		t.Fatalf("a repeated join was announced again: %v", msg)
	}
	if !inRoomUnlocked(hub, chatID, client.ID) {
		t.Fatal("client is not in the room")
	}
}

// A join undone by a removal during it reports ErrNotInChat, so join_chat
// doesn't tell the client it follows the chat (#48). Needs Redis.
func TestJoinUndoneByRemovalReportsNotInChat(t *testing.T) {
	hub := newRedisTestHub(t)
	pm := hub.pubSubManager

	chatID := bson.NewObjectID().Hex()
	joining := newTestClient("joining", bson.NewObjectID())
	addClient(hub, joining)
	t.Cleanup(func() { _ = pm.unsubscribeFromChannel(pm.getChatChannel(chatID)) })

	pm.afterRoomJoin = func(string) {
		if removeUsersFromRoom(hub.Hub, chatID, []string{joining.UserID.Hex()}) {
			pm.releaseEmptyChats([]string{chatID})
		}
	}

	if err := hub.JoinChat(context.Background(), joining, chatID); !errors.Is(err, ErrNotInChat) {
		t.Fatalf("err = %v, want ErrNotInChat", err)
	}
}
