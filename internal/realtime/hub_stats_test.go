package realtime

import (
	"encoding/json"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// GetStats gives counts only. Channel names are user:<id> and chat:<id>, so
// listing them shows who is online and which chats are active (#24).
func TestGetStatsHasNoIdentifiers(t *testing.T) {
	hub := newTestHub(t)
	userID, chatID := bson.NewObjectID(), bson.NewObjectID()
	addClient(hub, newTestClient("c1", userID), chatID.Hex())

	// The test hub can't reach Redis, so record the subscriptions directly.
	hub.pubSubManager.mu.Lock()
	hub.pubSubManager.subscriptions["user:"+userID.Hex()] = nil
	hub.pubSubManager.subscriptions["chat:"+chatID.Hex()] = nil
	hub.pubSubManager.mu.Unlock()

	raw, err := json.Marshal(hub.GetStats())
	if err != nil {
		t.Fatalf("marshal stats: %v", err)
	}
	for _, id := range []string{userID.Hex(), chatID.Hex()} {
		if strings.Contains(string(raw), id) {
			t.Errorf("stats contain %s: %s", id, raw)
		}
	}

	var got struct {
		TotalClients       int `json:"totalClients"`
		TotalChats         int `json:"totalChats"`
		TotalUsers         int `json:"totalUsers"`
		SubscribedChannels int `json:"subscribedChannels"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if got.TotalClients != 1 || got.TotalChats != 1 || got.TotalUsers != 1 || got.SubscribedChannels != 2 {
		t.Errorf("counts = %+v, want 1 client, 1 chat, 1 user, 2 channels", got)
	}
}
