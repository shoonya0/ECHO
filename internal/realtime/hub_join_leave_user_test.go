package realtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #50: join and leave events name the user, from the profile the connection
// loaded when it opened, so the hub needs no lookup. Needs Redis: run with
// TEST_DB_REQUIRED=1.
func TestJoinAndLeaveEventsNameTheUser(t *testing.T) {
	hub := newRedisTestHub(t)
	pm := hub.pubSubManager

	chatID := bson.NewObjectID().Hex()
	client := newTestClient("c1", bson.NewObjectID())
	client.Username, client.Avatar = "alice", "https://img/alice.png"
	addClient(hub, client)
	t.Cleanup(func() { _ = pm.unsubscribeFromChannel(pm.getChatChannel(chatID)) })

	observer := hub.redisClient.Subscribe(context.Background(), pm.getChatChannel(chatID))
	t.Cleanup(func() { _ = observer.Close() })
	if _, err := observer.Receive(context.Background()); err != nil {
		t.Fatalf("observer subscribe: %v", err)
	}

	if err := hub.JoinChat(context.Background(), client, chatID); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := hub.LeaveChat(client, chatID); err != nil {
		t.Fatalf("leave: %v", err)
	}

	for _, action := range []string{"join", "leave"} {
		raw, err := observer.ReceiveMessage(context.Background())
		if err != nil {
			t.Fatalf("%s event: %v", action, err)
		}
		var event struct {
			Type     string               `json:"type"`
			Username string               `json:"username"`
			Avatar   string               `json:"avatar"`
			Data     models.UserJoinLeave `json:"data"`
		}
		if err := json.Unmarshal([]byte(raw.Payload), &event); err != nil {
			t.Fatalf("decode %s event: %v", action, err)
		}
		if event.Type != action || event.Data.Action != action {
			t.Fatalf("event %s/%s, want %s", event.Type, event.Data.Action, action)
		}
		if event.Data.Username != "alice" || event.Data.Avatar != "https://img/alice.png" {
			t.Errorf("%s data username %q, avatar %q; want alice and her avatar", action, event.Data.Username, event.Data.Avatar)
		}
		if event.Username != "alice" || event.Avatar != "https://img/alice.png" {
			t.Errorf("%s envelope username %q, avatar %q; want alice and her avatar", action, event.Username, event.Avatar)
		}
	}
}
