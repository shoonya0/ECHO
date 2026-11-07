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

// useTestDB is testutil.UseTestDB under the name this package's tests use.
func useTestDB(t *testing.T) context.Context {
	t.Helper()
	return testutil.UseTestDB(t)
}

func TestGetChatMessagesRequiresMembership(t *testing.T) {
	ctx := useTestDB(t)

	member, outsider := bson.NewObjectID(), bson.NewObjectID()
	chatID := bson.NewObjectID()

	_, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"participants": bson.M{member.Hex(): bson.M{"status": "accepted"}},
	})
	if err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	_, err = database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, bson.M{
		"_id":      bson.NewObjectID(),
		"chatId":   chatID,
		"senderId": member,
		"content":  "private",
	})
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}

	t.Run("member reads history", func(t *testing.T) {
		msgs, _, err := GetChatMessages(ctx, member, chatID, 10, nil)
		if err != nil {
			t.Fatalf("GetChatMessages: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("got %d messages, want 1", len(msgs))
		}
	})

	t.Run("non-member gets not found", func(t *testing.T) {
		msgs, _, err := GetChatMessages(ctx, outsider, chatID, 10, nil)
		if !errors.Is(err, ErrChatNotFound) {
			t.Fatalf("err = %v, want ErrChatNotFound", err)
		}
		if msgs != nil {
			t.Fatalf("leaked %d messages to a non-member", len(msgs))
		}
	})

	t.Run("unknown chat gets not found", func(t *testing.T) {
		_, _, err := GetChatMessages(ctx, member, bson.NewObjectID(), 10, nil)
		if !errors.Is(err, ErrChatNotFound) {
			t.Fatalf("err = %v, want ErrChatNotFound", err)
		}
	})
}
