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

// #38: a contact request to yourself put you in your own pendingOut and
// pendingIn, and a direct chat with yourself was saved with one participant.
// Both are refused now, before anything is written.
func TestSelfTargetsAreRefused(t *testing.T) {
	ctx := useTestDB(t)
	users := database.DB.Collection(string(constants.UserColl))
	chats := database.DB.Collection(string(constants.ChatColl))

	me := bson.NewObjectID()
	if _, err := users.InsertOne(ctx, bson.M{"_id": me, "username": me.Hex(), "profile": bson.M{"displayName": "me"}}); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	t.Run("contact request to yourself", func(t *testing.T) {
		err := SendContactRequest(ctx, me, me)
		if !errors.Is(err, ErrSelfContactRequest) {
			t.Fatalf("err = %v, want ErrSelfContactRequest", err)
		}
		n, err := users.CountDocuments(ctx, bson.M{"_id": me, "$or": []bson.M{
			{"contactInfo.pendingOut": me},
			{"contactInfo.pendingIn": me},
		}})
		if err != nil || n != 0 {
			t.Fatalf("self request stored: n=%d err=%v", n, err)
		}
	})

	t.Run("direct chat with yourself", func(t *testing.T) {
		userCtx := context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: me, Username: me.Hex()})
		_, err := CreateDirectChat(userCtx, me)
		if !errors.Is(err, ErrSelfDirectChat) {
			t.Fatalf("err = %v, want ErrSelfDirectChat", err)
		}
		n, err := chats.CountDocuments(ctx, bson.M{"participants." + me.Hex(): bson.M{"$exists": true}})
		if err != nil || n != 0 {
			t.Fatalf("self chat stored: n=%d err=%v", n, err)
		}
	})
}
