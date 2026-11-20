package services

import (
	"testing"
	"time"

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

// #42: readBy grew by one key per reader on every message. The app reads readSeqs
// since ECHO-app #9, so marking a message read no longer writes it, a new message
// doesn't carry it, and a one-time job takes it off older messages.
func TestReadByIsNoLongerStored(t *testing.T) {
	ctx := useTestDB(t)
	SetBroadcaster(&recordingBroadcaster{})
	t.Cleanup(func() { SetBroadcaster(nil) })
	member, chatID, messageID := seedReadableChat(ctx, t)
	messages := database.DB.Collection(string(constants.MessageColl))
	old := bson.NewObjectID()
	if _, err := messages.InsertOne(ctx, bson.M{
		"_id": old, "chatId": chatID, "readBy": bson.M{member.Hex(): time.Now()},
	}); err != nil {
		t.Fatalf("insert an old message: %v", err)
	}

	if err := MarkMessagesAsRead(ctx, chatID, member, []bson.ObjectID{messageID}); err != nil {
		t.Fatalf("MarkMessagesAsRead: %v", err)
	}
	sent, err := SendMessage(asUser(ctx, member), chatID, member, "hi", "text", nil, nil)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	for _, id := range []bson.ObjectID{messageID, sent.ID} {
		if err := messages.FindOne(ctx, bson.M{"_id": id, "readBy": bson.M{"$exists": true}}).Err(); err == nil {
			t.Fatalf("message %s has readBy", id.Hex())
		}
	}

	n, err := RemoveMessageReadBy(ctx)
	if err != nil || n != 1 {
		t.Fatalf("RemoveMessageReadBy = (%d, %v), want (1, nil)", n, err)
	}
	if err := messages.FindOne(ctx, bson.M{"_id": old, "readBy": bson.M{"$exists": true}}).Err(); err == nil {
		t.Fatal("the old message still has readBy")
	}
	if n, err := RemoveMessageReadBy(ctx); err != nil || n != 0 {
		t.Fatalf("second RemoveMessageReadBy = (%d, %v), want (0, nil)", n, err)
	}
}
