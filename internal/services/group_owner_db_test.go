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

func insertTestUsers(t *testing.T, ctx context.Context, ids ...bson.ObjectID) {
	t.Helper()
	for _, id := range ids {
		_, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": id, "username": id.Hex()})
		if err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
}

func loadChat(t *testing.T, ctx context.Context, chatID bson.ObjectID) models.Chat {
	t.Helper()
	var chat models.Chat
	if err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, bson.M{"_id": chatID}).Decode(&chat); err != nil {
		t.Fatalf("load chat: %v", err)
	}
	return chat
}

// #18: a group created through the API must let its creator manage members.
func TestCreateGroupChatCreatorCanAddMembers(t *testing.T) {
	ctx := useTestDB(t)

	creator, invitee, newcomer := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, creator, invitee, newcomer)

	creatorCtx := context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: creator, Username: creator.Hex()})
	chat, err := CreateGroupChat(creatorCtx, "g", "", []bson.ObjectID{invitee})
	if err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}

	if role := loadChat(t, ctx, chat.ChatID).Participants[creator].Role; role != string(constants.ChatRoleOwner) {
		t.Fatalf("stored creator role = %q, want %q", role, constants.ChatRoleOwner)
	}

	if err := AddGroupMember(ctx, creator, chat.ChatID, []bson.ObjectID{newcomer}); err != nil {
		t.Fatalf("creator AddGroupMember: %v", err)
	}
	if _, ok := loadChat(t, ctx, chat.ChatID).Participants[newcomer]; !ok {
		t.Fatal("newcomer was not added")
	}

	// The invitee is a plain member and still can't add people.
	if err := AddGroupMember(ctx, invitee, chat.ChatID, []bson.ObjectID{bson.NewObjectID()}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("invitee AddGroupMember err = %v, want ErrPermissionDenied", err)
	}
}

func TestBackfillGroupOwnerRoles(t *testing.T) {
	ctx := useTestDB(t)
	chats := database.DB.Collection(string(constants.ChatColl))

	owner, member := bson.NewObjectID(), bson.NewObjectID()
	legacy, alreadyOwned, ownerGone, direct := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	member0 := bson.M{"role": string(constants.ChatRoleMember), "requestStatus": "accepted"}
	docs := []any{
		// Pre-#18 group: the owner was stored as a member.
		bson.M{"_id": legacy, "chatType": string(constants.ChatTypeGroup), "ownerId": owner,
			"participants": bson.M{owner.Hex(): member0, member.Hex(): member0}},
		bson.M{"_id": alreadyOwned, "chatType": string(constants.ChatTypeGroup), "ownerId": owner,
			"participants": bson.M{owner.Hex(): bson.M{"role": string(constants.ChatRoleOwner)}}},
		// The owner left: don't re-create their participant entry.
		bson.M{"_id": ownerGone, "chatType": string(constants.ChatTypeGroup), "ownerId": owner,
			"participants": bson.M{member.Hex(): member0}},
		// Direct chats have no owner role.
		bson.M{"_id": direct, "chatType": string(constants.ChatTypeDirect), "ownerId": owner,
			"participants": bson.M{owner.Hex(): member0, member.Hex(): member0}},
	}
	if _, err := chats.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert chats: %v", err)
	}

	n, err := BackfillGroupOwnerRoles(ctx)
	if err != nil {
		t.Fatalf("BackfillGroupOwnerRoles: %v", err)
	}
	if n != 1 {
		t.Fatalf("updated = %d, want 1", n)
	}

	got := loadChat(t, ctx, legacy).Participants
	if got[owner].Role != string(constants.ChatRoleOwner) {
		t.Fatalf("legacy owner role = %q, want %q", got[owner].Role, constants.ChatRoleOwner)
	}
	if got[owner].RequestStatus != "accepted" {
		t.Fatalf("legacy owner requestStatus = %q, want it unchanged", got[owner].RequestStatus)
	}
	if got[member].Role != string(constants.ChatRoleMember) {
		t.Fatalf("legacy member role = %q, want %q", got[member].Role, constants.ChatRoleMember)
	}
	if _, ok := loadChat(t, ctx, ownerGone).Participants[owner]; ok {
		t.Fatal("backfill re-added an owner who left")
	}
	if role := loadChat(t, ctx, direct).Participants[owner].Role; role != string(constants.ChatRoleMember) {
		t.Fatalf("direct chat role = %q, want it unchanged", role)
	}

	if n, err := BackfillGroupOwnerRoles(ctx); err != nil || n != 0 {
		t.Fatalf("second run = (%d, %v), want (0, nil)", n, err)
	}
}
