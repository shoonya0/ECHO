package services

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// useTestDB points database.DB at a throwaway database on the MongoDB at
// TEST_DB_URI (default: the docker-compose one) and drops it afterwards.
// The test is skipped when MongoDB isn't reachable.
func useTestDB(t *testing.T) context.Context {
	t.Helper()
	uri := os.Getenv("TEST_DB_URI")
	if uri == "" {
		uri = "mongodb://echo:echo@localhost:27017"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("MongoDB unavailable: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		t.Skipf("MongoDB unavailable: %v", err)
	}

	prev := database.DB
	database.DB = client.Database("echo_test_" + bson.NewObjectID().Hex())
	t.Cleanup(func() {
		_ = database.DB.Drop(context.Background())
		_ = client.Disconnect(context.Background())
		database.DB = prev
	})
	return ctx
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
