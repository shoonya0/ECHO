package services

import (
	"context"
	"errors"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// seedReadableChat inserts a member, a chat holding them and one message.
func seedReadableChat(ctx context.Context, t *testing.T) (member, chatID, messageID bson.ObjectID) {
	t.Helper()
	member, chatID, messageID = bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": member, "username": "reader-" + member.Hex()}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"participants": bson.M{member.Hex(): bson.M{"status": "accepted"}},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, bson.M{
		"_id": messageID, "chatId": chatID, "senderId": member, "content": "hi",
	}); err != nil {
		t.Fatalf("insert message: %v", err)
	}
	return member, chatID, messageID
}

// A failed write of the chat's read receipt used to be discarded, so the
// reader got no error and the chat was told about a read that wasn't saved (#37).
func TestMarkMessagesAsReadReturnsChatWriteError(t *testing.T) {
	ctx := useTestDB(t)
	rec := &recordingBroadcaster{}
	SetBroadcaster(rec)
	t.Cleanup(func() { SetBroadcaster(nil) })

	member, chatID, messageID := seedReadableChat(ctx, t)

	testutil.FailCommand(t, ctx, "update", 1) // the messages' readBy write passes
	if err := MarkMessagesAsRead(ctx, chatID, member, []bson.ObjectID{messageID}); err == nil {
		t.Fatal("MarkMessagesAsRead = nil, want the chat write's error")
	}
	if len(rec.events) != 0 {
		t.Fatalf("published %d read events after a failed write", len(rec.events))
	}
}

// Read receipts and reactions stop when the request's context ends, instead of
// running on context.Background() (#37).
func TestReadAndReactionStopWithTheRequestContext(t *testing.T) {
	ctx := useTestDB(t)
	SetBroadcaster(&recordingBroadcaster{})
	t.Cleanup(func() { SetBroadcaster(nil) })

	member, chatID, messageID := seedReadableChat(ctx, t)
	ended, cancel := context.WithCancel(ctx)
	cancel()

	calls := map[string]func() error{
		"mark read": func() error { return MarkMessagesAsRead(ended, chatID, member, []bson.ObjectID{messageID}) },
		"add reaction": func() error {
			_, err := AddMessageReaction(ended, chatID, messageID, member, "👍")
			return err
		},
		"remove reaction": func() error {
			_, err := RemoveMessageReaction(ended, chatID, messageID, member, "👍")
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
		})
	}
}
