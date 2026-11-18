package services

import (
	"context"
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
)

// insertInviteGroup stores a group owned by owner, with each of pending as a pending
// member, and gives every user a chats entry for it, as creating or adding does since
// #5. Users are inserted too.
func insertInviteGroup(t *testing.T, ctx context.Context, owner bson.ObjectID, pending ...bson.ObjectID) bson.ObjectID {
	t.Helper()
	chatID := bson.NewObjectID()
	participants := bson.M{owner.Hex(): bson.M{"role": string(constants.ChatRoleOwner), "requestStatus": string(constants.StatusAccepted)}}
	for _, id := range pending {
		participants[id.Hex()] = bson.M{
			"role": string(constants.ChatRoleMember), "requestStatus": string(constants.StatusPending), "requestedBy": owner,
		}
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id": chatID, "chatType": string(constants.ChatTypeGroup), "name": "g-" + chatID.Hex(),
		"participants": participants, "stats": bson.M{"participantCount": len(participants)},
	}); err != nil {
		t.Fatalf("insert group: %v", err)
	}
	for _, id := range append([]bson.ObjectID{owner}, pending...) {
		if _, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx,
			bson.M{"_id": id},
			bson.M{"$set": bson.M{"username": id.Hex(), "chats." + chatID.Hex(): 0}},
			options.UpdateOne().SetUpsert(true),
		); err != nil {
			t.Fatalf("upsert user: %v", err)
		}
	}
	return chatID
}

// An invitee gets a chats entry for the group when it is created, so the group
// shows in their pending invites (#5).
func TestCreateGroupChatGivesInviteesAChatsEntry(t *testing.T) {
	ctx := useTestDB(t)
	creator, invitee := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, creator, invitee)

	creatorCtx := context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: creator, Username: creator.Hex()})
	chat, err := CreateGroupChat(creatorCtx, "g", "", []bson.ObjectID{invitee})
	if err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	if !hasChatEntry(t, ctx, invitee, chat.ChatID) {
		t.Fatalf("invitee has no chats entry for the new group")
	}
}

// Members added later get the entry too (#5).
func TestAddGroupMemberGivesInviteesAChatsEntry(t *testing.T) {
	ctx := useTestDB(t)
	owner, newcomer := bson.NewObjectID(), bson.NewObjectID()
	chatID := insertInviteGroup(t, ctx, owner)
	insertTestUsers(t, ctx, newcomer)

	if err := AddGroupMember(ctx, owner, chatID, []bson.ObjectID{newcomer}); err != nil {
		t.Fatalf("AddGroupMember: %v", err)
	}
	if !hasChatEntry(t, ctx, newcomer, chatID) {
		t.Fatalf("added member has no chats entry for the group")
	}
}

// The hub follows only chats a user has accepted: a pending invite gives no live
// room (#5, #17).
func TestGetUserChatsSkipsPendingGroups(t *testing.T) {
	ctx := useTestDB(t)
	owner, user := bson.NewObjectID(), bson.NewObjectID()
	pendingGroup := insertInviteGroup(t, ctx, owner, user)
	joined := insertInviteGroup(t, ctx, owner)
	if _, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, bson.M{"_id": joined},
		bson.M{"$set": bson.M{"participants." + user.Hex(): bson.M{"role": "member", "requestStatus": "accepted"}}}); err != nil {
		t.Fatalf("add accepted member: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx, bson.M{"_id": user},
		bson.M{"$set": bson.M{"chats." + joined.Hex(): 0}}); err != nil {
		t.Fatalf("add chats entry: %v", err)
	}

	got, err := GetUserChats(ctx, user)
	if err != nil {
		t.Fatalf("GetUserChats: %v", err)
	}
	if slices.Contains(got, pendingGroup) || !slices.Contains(got, joined) {
		t.Fatalf("GetUserChats = %v, want only the accepted group %s", got, joined.Hex())
	}
}
