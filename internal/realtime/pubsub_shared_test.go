package realtime

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// namedRedis returns a client to the test Redis whose connections carry a
// unique name, so CLIENT LIST can count them. Needs Redis.
func namedRedis(t *testing.T) (*redis.Client, string) {
	t.Helper()
	base := testutil.UseTestRedis(t)
	opts := *base.Options()
	opts.ClientName = "echo-test-" + bson.NewObjectID().Hex()
	client := redis.NewClient(&opts)
	t.Cleanup(func() { _ = client.Close() })
	return client, opts.ClientName
}

// pubsubConnections counts the pub/sub connections named name.
func pubsubConnections(t *testing.T, client *redis.Client, name string) int {
	t.Helper()
	list, err := client.ClientList(context.Background()).Result()
	if err != nil {
		t.Fatalf("CLIENT LIST: %v", err)
	}
	n := 0
	for _, line := range strings.Split(list, "\n") {
		if strings.Contains(line, " name="+name+" ") && !strings.Contains(line, " sub=0 ") {
			n++
		}
	}
	return n
}

// Every channel used to open its own Redis connection, so connections grew with
// online users and chats until Redis refused new ones (#21). All channels of an
// instance now share one connection.
func TestChannelsShareOneRedisConnection(t *testing.T) {
	client, name := namedRedis(t)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	pm := NewPubSubManager(client, &models.Hub{}, logger, "test-"+bson.NewObjectID().Hex())
	t.Cleanup(pm.cancel)

	if err := pm.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	for range 5 {
		if err := pm.subscribeToChannel(pm.getChatChannel(bson.NewObjectID().Hex())); err != nil {
			t.Fatalf("subscribe: %v", err)
		}
	}
	if err := pm.subscribeToChannel(pm.getUserChannel(bson.NewObjectID().Hex())); err != nil {
		t.Fatalf("subscribe user: %v", err)
	}

	if got := pubsubConnections(t, client, name); got != 1 {
		t.Fatalf("%d pub/sub connections for 10 channels, want 1", got)
	}
}

// subscribeToChannel returns only after Redis has confirmed the subscribe, so a
// message published right after it returns is delivered (#22).
func TestSubscribeReturnsAfterRedisConfirms(t *testing.T) {
	client, _ := namedRedis(t)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	pm := NewPubSubManager(client, &models.Hub{}, logger, "test")
	t.Cleanup(pm.cancel)

	for range 20 {
		channel := pm.getChatChannel(bson.NewObjectID().Hex())
		if err := pm.subscribeToChannel(channel); err != nil {
			t.Fatalf("subscribe: %v", err)
		}
		if n := client.Publish(context.Background(), channel, "x").Val(); n != 1 {
			t.Fatalf("publish right after subscribe reached %d subscribers, want 1", n)
		}
	}
}

// Releasing one channel of the shared connection leaves the others delivering,
// and nothing is delivered for the released one.
func TestUnsubscribeKeepsOtherChannels(t *testing.T) {
	client, _ := namedRedis(t)
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	hub := newTestHub(t)
	pm := NewPubSubManager(client, hub.Hub, logger, "test")
	t.Cleanup(pm.cancel)

	released, kept := bson.NewObjectID().Hex(), bson.NewObjectID().Hex()
	inReleased, inKept := bufferedClient("r", bson.NewObjectID()), bufferedClient("k", bson.NewObjectID())
	addClient(hub, inReleased, released)
	addClient(hub, inKept, kept)
	for _, chatID := range []string{released, kept} {
		if err := pm.subscribeToChannel(pm.getChatChannel(chatID)); err != nil {
			t.Fatalf("subscribe: %v", err)
		}
	}
	if err := pm.unsubscribeFromChannel(pm.getChatChannel(released)); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}

	sender := NewPubSubManager(client, &models.Hub{}, logger, "sender")
	t.Cleanup(sender.cancel)
	for _, chatID := range []string{released, kept} {
		if err := sender.PublishToChat(chatID, &models.WebSocketMessage{Type: models.WSMessageTypeChat, ChatID: chatID}); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for len(inKept.Send) < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(inKept.Send) != 1 {
		t.Fatal("the kept chat's message wasn't delivered")
	}
	if pm.IsSubscribed(pm.getChatChannel(released)) {
		t.Fatal("released channel still counts as subscribed")
	}
	if len(inReleased.Send) != 0 {
		t.Fatal("a message of the released chat was delivered")
	}
}

// Subscribe, unsubscribe and subscribe again on one channel: the commands go
// out in order, so the channel ends up subscribed and the last subscribe
// returns only once Redis has it.
func TestResubscribeAfterReleaseIsConfirmed(t *testing.T) {
	client, _ := namedRedis(t)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	pm := NewPubSubManager(client, &models.Hub{}, logger, "test")
	t.Cleanup(pm.cancel)

	channel := pm.getChatChannel(bson.NewObjectID().Hex())
	for range 20 {
		if err := pm.subscribeToChannel(channel); err != nil {
			t.Fatalf("subscribe: %v", err)
		}
		if err := pm.unsubscribeFromChannel(channel); err != nil {
			t.Fatalf("unsubscribe: %v", err)
		}
		if err := pm.subscribeToChannel(channel); err != nil {
			t.Fatalf("subscribe again: %v", err)
		}
		if n := client.Publish(context.Background(), channel, "x").Val(); n != 1 {
			t.Fatalf("publish after resubscribe reached %d subscribers, want 1", n)
		}
		if err := pm.unsubscribeFromChannel(channel); err != nil {
			t.Fatalf("unsubscribe: %v", err)
		}
	}
}

// A subscribe that can't reach Redis fails and leaves the channel
// unsubscribed, so the next subscribe tries again.
func TestFailedSubscribeLeavesChannelUnsubscribed(t *testing.T) {
	hub := newTestHub(t) // its Redis calls fail at once
	pm := hub.pubSubManager
	t.Cleanup(pm.cancel)

	channel := pm.getChatChannel(bson.NewObjectID().Hex())
	if err := pm.subscribeToChannel(channel); err == nil {
		t.Fatal("subscribe without Redis succeeded")
	}
	if pm.IsSubscribed(channel) {
		t.Fatal("failed subscribe left the channel subscribed")
	}
	pm.mu.RLock()
	waiters := len(pm.pending[channel])
	pm.mu.RUnlock()
	if waiters != 0 {
		t.Fatalf("failed subscribe left %d waiters", waiters)
	}
}
