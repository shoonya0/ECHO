package services

import (
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #42: every reader added a key to every message's readBy. The chat now keeps
// each member's highest read seq, which only moves forward. The delivery event
// carries it, and ChatReadSeqs returns it for the history reply.
func TestMarkMessagesAsReadKeepsTheHighestReadSeq(t *testing.T) {
	ctx := useTestDB(t)
	rec := &recordingBroadcaster{}
	SetBroadcaster(rec)
	t.Cleanup(func() { SetBroadcaster(nil) })

	member, chatID, legacy := seedReadableChat(ctx, t) // legacy: a message without seq
	other := bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, bson.M{"_id": chatID},
		bson.M{"$set": bson.M{"participants." + other.Hex(): bson.M{"status": "accepted"}}}); err != nil {
		t.Fatalf("add a member: %v", err)
	}
	ids := make([]bson.ObjectID, 3)
	for i := range ids {
		ids[i] = bson.NewObjectID()
		if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, bson.M{
			"_id": ids[i], "chatId": chatID, "senderId": other, "content": "hi", "seq": int64(i + 1),
		}); err != nil {
			t.Fatalf("insert message: %v", err)
		}
	}

	for _, step := range []struct {
		name string
		read []bson.ObjectID
	}{
		{"reading seq 3 moves it to 3", []bson.ObjectID{ids[2]}},
		{"reading seq 1 later keeps 3", []bson.ObjectID{ids[0]}},
		{"reading a message without seq keeps 3", []bson.ObjectID{legacy}},
	} {
		if err := MarkMessagesAsRead(ctx, chatID, member, step.read); err != nil {
			t.Fatalf("%s: MarkMessagesAsRead: %v", step.name, err)
		}
		data, _ := rec.events[len(rec.events)-1].Data.(map[string]interface{})
		if got, _ := data["readUpToSeq"].(int64); got != 3 {
			t.Fatalf("%s: event readUpToSeq = %v, want 3", step.name, data["readUpToSeq"])
		}
	}

	seqs, err := ChatReadSeqs(ctx, chatID)
	if err != nil {
		t.Fatalf("ChatReadSeqs: %v", err)
	}
	if len(seqs) != 1 || seqs[member.Hex()] != 3 {
		t.Fatalf("ChatReadSeqs = %v, want only %s: 3", seqs, member.Hex())
	}
}
