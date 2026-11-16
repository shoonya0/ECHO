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

// #112: the creator's chats.<id> entry is what the hub uses to deliver the
// group's live messages, so a group whose creator entry can't be written must
// not be left behind.
func TestCreateGroupChatFailsWhenCreatorEntryNotWritten(t *testing.T) {
	ctx := useTestDB(t)

	// The creator's user document is gone (for example removed after login),
	// so the creator update matches nothing.
	creator, invitee := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, invitee)

	creatorCtx := context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: creator, Username: creator.Hex()})
	chat, err := CreateGroupChat(creatorCtx, "g", "", []bson.ObjectID{invitee})
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("CreateGroupChat err = %v, want ErrUserNotFound", err)
	}
	if chat != nil {
		t.Fatalf("CreateGroupChat returned chat %v, want nil", chat.ChatID)
	}

	n, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, bson.M{"ownerId": creator})
	if err != nil {
		t.Fatalf("count chats: %v", err)
	}
	if n != 0 {
		t.Fatalf("stored groups owned by the creator = %d, want 0", n)
	}

	// The invitee got no invitation to a group that doesn't exist.
	var u models.User
	if err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, bson.M{"_id": invitee}).Decode(&u); err != nil {
		t.Fatalf("load invitee: %v", err)
	}
	if len(u.ChatInvitations) != 0 {
		t.Fatalf("invitee invitations = %v, want none", u.ChatInvitations)
	}
}

// The normal path still stores the creator's entry.
func TestCreateGroupChatWritesCreatorEntry(t *testing.T) {
	ctx := useTestDB(t)

	creator, invitee := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, creator, invitee)

	creatorCtx := context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: creator, Username: creator.Hex()})
	chat, err := CreateGroupChat(creatorCtx, "g", "", []bson.ObjectID{invitee})
	if err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}

	n, err := database.DB.Collection(string(constants.UserColl)).CountDocuments(ctx,
		bson.M{"_id": creator, "chats." + chat.ChatID.Hex(): bson.M{"$exists": true}})
	if err != nil {
		t.Fatalf("count creator: %v", err)
	}
	if n != 1 {
		t.Fatal("creator has no chats entry for the new group")
	}
}
