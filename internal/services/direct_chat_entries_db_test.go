package services

import (
	"context"
	"errors"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// hasChatEntry reports whether the user document of userID has chats.<chatID>.
func hasChatEntry(t *testing.T, ctx context.Context, userID, chatID bson.ObjectID) bool {
	t.Helper()
	n, err := database.DB.Collection(string(constants.UserColl)).CountDocuments(ctx, bson.M{
		"_id": userID, "chats." + chatID.Hex(): bson.M{"$exists": true},
	})
	if err != nil {
		t.Fatalf("count user: %v", err)
	}
	return n == 1
}

// #132: a direct chat is kept only when both users got their chats entry, and
// opening an existing chat again writes back an entry that is missing.
func TestDirectChatEntriesForBothUsers(t *testing.T) {
	ctx := useTestDB(t)
	as := func(id bson.ObjectID) context.Context {
		return context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: id, Username: id.Hex()})
	}
	chats := database.DB.Collection(string(constants.ChatColl))

	t.Run("a user document missing at the write fails the create", func(t *testing.T) {
		gone, target := bson.NewObjectID(), bson.NewObjectID()
		insertTestUsers(t, ctx, target) // the caller's document no longer exists

		_, err := CreateDirectChat(as(gone), target)
		if !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("err = %v, want ErrUserNotFound", err)
		}
		n, err := chats.CountDocuments(ctx, bson.M{"participants." + target.Hex(): bson.M{"$exists": true}})
		if err != nil {
			t.Fatalf("count chats: %v", err)
		}
		if n != 0 {
			t.Errorf("%d direct chats left behind, want 0", n)
		}
		var u bson.M
		if err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, bson.M{"_id": target}).Decode(&u); err != nil {
			t.Fatalf("load target: %v", err)
		}
		if c, ok := u["chats"].(bson.M); ok && len(c) > 0 {
			t.Errorf("target keeps chats entries %v of the removed chat", c)
		}
	})

	t.Run("opening an existing chat repairs a missing entry", func(t *testing.T) {
		alice, bob := bson.NewObjectID(), bson.NewObjectID()
		insertTestUsers(t, ctx, alice, bob)

		chat, err := CreateDirectChat(as(alice), bob)
		if err != nil {
			t.Fatalf("CreateDirectChat: %v", err)
		}
		if _, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx,
			bson.M{"_id": bob}, bson.M{"$unset": bson.M{"chats." + chat.ChatID.Hex(): ""}}); err != nil {
			t.Fatalf("drop bob's entry: %v", err)
		}

		again, err := CreateDirectChat(as(alice), bob)
		if err != nil {
			t.Fatalf("CreateDirectChat again: %v", err)
		}
		if again.ChatID != chat.ChatID {
			t.Fatalf("got a second direct chat %s", again.ChatID.Hex())
		}
		for _, id := range []bson.ObjectID{alice, bob} {
			if !hasChatEntry(t, ctx, id, chat.ChatID) {
				t.Errorf("user %s has no chats entry for the chat", id.Hex())
			}
		}
	})
}
