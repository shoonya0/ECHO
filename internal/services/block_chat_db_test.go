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

// #27: a block did nothing in chats. Either side of a block now can't open a
// direct chat with the other, whether or not the chat already exists.
func TestDirectChatRefusedWhenBlocked(t *testing.T) {
	ctx := useTestDB(t)
	chats := database.DB.Collection(string(constants.ChatColl))
	a, b := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, a, b)

	as := func(id bson.ObjectID) context.Context {
		return context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: id, Username: id.Hex()})
	}
	countChats := func() int64 {
		t.Helper()
		n, err := chats.CountDocuments(ctx, bson.M{
			"participants." + a.Hex(): bson.M{"$exists": true},
			"participants." + b.Hex(): bson.M{"$exists": true},
		})
		if err != nil {
			t.Fatalf("count chats: %v", err)
		}
		return n
	}

	if err := BlockUnblockUser(ctx, a, b, string(constants.StatusBlocked)); err != nil {
		t.Fatalf("A blocks B: %v", err)
	}
	for name, caller := range map[string]bson.ObjectID{"blocked user": b, "blocker": a} {
		other := a
		if caller == a {
			other = b
		}
		if _, err := CreateDirectChat(as(caller), other); !errors.Is(err, ErrBlocked) {
			t.Fatalf("%s opens new chat: err = %v, want ErrBlocked", name, err)
		}
	}
	if n := countChats(); n != 0 {
		t.Fatalf("chats stored while blocked = %d, want 0", n)
	}

	// Unblocked, the chat can be made; blocked again, the existing chat is refused.
	if err := BlockUnblockUser(ctx, a, b, string(constants.StatusUnblocked)); err != nil {
		t.Fatalf("A unblocks B: %v", err)
	}
	if _, err := CreateDirectChat(as(b), a); err != nil {
		t.Fatalf("B opens chat after unblock: %v", err)
	}
	if err := BlockUnblockUser(ctx, a, b, string(constants.StatusBlocked)); err != nil {
		t.Fatalf("A blocks B again: %v", err)
	}
	if _, err := CreateDirectChat(as(b), a); !errors.Is(err, ErrBlocked) {
		t.Fatalf("B opens existing chat: err = %v, want ErrBlocked", err)
	}
	if n := countChats(); n != 1 {
		t.Fatalf("chats = %d, want the 1 made while unblocked", n)
	}
}

// A block whose second write (the target's blockedBy) never landed still counts:
// the check reads each side's own blocked list.
func TestIsBlockedBetweenReadsBothBlockedLists(t *testing.T) {
	ctx := useTestDB(t)
	a, b, c := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, b, c)
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{
		"_id": a, "username": a.Hex(), "contactInfo": bson.M{"blocked": bson.A{b}},
	}); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	for _, tc := range []struct {
		name string
		x, y bson.ObjectID
		want bool
	}{
		{"blocker first", a, b, true},
		{"blocked user first", b, a, true},
		{"no block", a, c, false},
	} {
		got, err := IsBlockedBetween(ctx, tc.x, tc.y)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
}
