package services

import (
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #50: the live message event carries the sender's name and avatar and the
// mentions, as the saved message does, so a client can show it without
// looking the sender up.
func TestMessageEventCarriesSenderAndMentions(t *testing.T) {
	ctx := useTestDB(t)
	rec := &recordingBroadcaster{}
	SetBroadcaster(rec)
	t.Cleanup(func() { SetBroadcaster(nil) })

	alice, bob := bson.NewObjectID(), bson.NewObjectID()
	chatID := bson.NewObjectID()
	users := database.DB.Collection(string(constants.UserColl))
	if _, err := users.InsertOne(ctx, bson.M{
		"_id": alice, "username": "alice",
		"profile": bson.M{"displayName": "Alice A", "avatar": "https://img/alice.png"},
	}); err != nil {
		t.Fatalf("insert alice: %v", err)
	}
	if _, err := users.InsertOne(ctx, bson.M{"_id": bob, "username": "bob"}); err != nil {
		t.Fatalf("insert bob: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"participants": bson.M{alice.Hex(): bson.M{"status": "accepted"}, bob.Hex(): bson.M{"status": "accepted"}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	if _, err := SendMessage(ctx, chatID, alice, "hi @bob", "text", nil, []string{bob.Hex()}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}

	var event *models.WebSocketMessage
	for i := range rec.events {
		if rec.events[i].Type == models.WSMessageTypeChat {
			event = &rec.events[i]
		}
	}
	if event == nil {
		t.Fatalf("no message event in %+v", rec.events)
	}
	if event.Username != "alice" || event.Avatar != "https://img/alice.png" {
		t.Errorf("envelope username %q, avatar %q; want alice and her avatar", event.Username, event.Avatar)
	}
	data, ok := event.Data.(models.ChatMessage)
	if !ok {
		t.Fatalf("data = %T, want models.ChatMessage", event.Data)
	}
	want := models.MessageSenderEmbed{UserID: alice, Username: "alice", DisplayName: "Alice A", Avatar: "https://img/alice.png"}
	if data.Sender == nil || *data.Sender != want {
		t.Errorf("data.sender = %+v, want %+v", data.Sender, want)
	}
	if len(data.Mentions) != 1 || data.Mentions[0] != bob.Hex() {
		t.Errorf("data.mentions = %v, want [%s]", data.Mentions, bob.Hex())
	}
}
