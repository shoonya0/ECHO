package services

import (
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Messages were saved with reactions: null, and MongoDB can't create
// reactions.<emoji> inside null, so every reaction on a real message failed (#72).
// Both a message sent now and one saved before the fix must take reactions.
func TestReactionsWorkOnRealMessages(t *testing.T) {
	ctx := useTestDB(t)

	alice, bob := bson.NewObjectID(), bson.NewObjectID()
	chatID := bson.NewObjectID()
	for _, id := range []bson.ObjectID{alice, bob} {
		if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": id, "username": "user-" + id.Hex()}); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"participants": bson.M{alice.Hex(): bson.M{"status": "accepted"}, bob.Hex(): bson.M{"status": "accepted"}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	sent, err := SendMessage(ctx, chatID, alice, "hi", "text", nil, nil)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	raw, err := database.DB.Collection(string(constants.MessageColl)).FindOne(ctx, bson.M{"_id": sent.ID}).Raw()
	if err != nil {
		t.Fatalf("find sent message: %v", err)
	}
	if got := raw.Lookup("reactions").Type; got != bson.TypeEmbeddedDocument {
		t.Errorf("sent message stores reactions as %v, want an empty document", got)
	}

	// A message saved before the fix, or by an older server copy during a deploy.
	oldID := bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, bson.M{
		"_id": oldID, "chatId": chatID, "senderId": alice, "content": "old", "reactions": nil,
	}); err != nil {
		t.Fatalf("insert old message: %v", err)
	}

	type reaction struct {
		Count int             `bson:"count"`
		Users []bson.ObjectID `bson:"users"`
	}
	reactions := func(t *testing.T, messageID bson.ObjectID) map[string]reaction {
		t.Helper()
		var doc struct {
			Reactions map[string]reaction `bson:"reactions"`
		}
		if err := database.DB.Collection(string(constants.MessageColl)).FindOne(ctx, bson.M{"_id": messageID}).Decode(&doc); err != nil {
			t.Fatalf("find message: %v", err)
		}
		return doc.Reactions
	}

	messages := []struct {
		name string
		id   bson.ObjectID
	}{
		{"message sent now", sent.ID},
		{"message saved with reactions: null", oldID},
	}
	for _, m := range messages {
		t.Run(m.name, func(t *testing.T) {
			if err := AddMessageReaction(chatID, m.id, bob, "👍"); err != nil {
				t.Fatalf("add: %v", err)
			}
			got, ok := reactions(t, m.id)["👍"]
			if !ok || got.Count != 1 || len(got.Users) != 1 || got.Users[0] != bob {
				t.Fatalf("after add, reactions = %+v, want 👍 with count 1 by bob", reactions(t, m.id))
			}
			if err := RemoveMessageReaction(chatID, m.id, bob, "👍"); err != nil {
				t.Fatalf("remove: %v", err)
			}
			if _, ok := reactions(t, m.id)["👍"]; ok {
				t.Fatalf("after remove, reactions = %+v, want no 👍", reactions(t, m.id))
			}
		})
	}
}
