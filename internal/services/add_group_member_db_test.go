package services

import (
	"errors"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestAddGroupMemberAuthorization(t *testing.T) {
	ctx := useTestDB(t)

	owner, admin, member, outsider, newcomer := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	chatID := bson.NewObjectID()

	for _, id := range []bson.ObjectID{owner, admin, member, outsider, newcomer} {
		_, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": id, "username": id.Hex()})
		if err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}
	_, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
		"participants": bson.M{
			owner.Hex():  bson.M{"role": string(constants.ChatRoleOwner), "requestStatus": "accepted"},
			admin.Hex():  bson.M{"role": string(constants.ChatRoleAdmin), "requestStatus": "accepted"},
			member.Hex(): bson.M{"role": string(constants.ChatRoleMember), "requestStatus": "accepted"},
		},
	})
	if err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	participants := func(t *testing.T) map[bson.ObjectID]models.ParticipantEmbed {
		t.Helper()
		var chat models.Chat
		if err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, bson.M{"_id": chatID}).Decode(&chat); err != nil {
			t.Fatalf("load chat: %v", err)
		}
		return chat.Participants
	}

	t.Run("outsider cannot add themselves", func(t *testing.T) {
		err := AddGroupMember(ctx, outsider, chatID, []bson.ObjectID{outsider})
		if !errors.Is(err, ErrNotParticipant) {
			t.Fatalf("err = %v, want ErrNotParticipant", err)
		}
		if _, ok := participants(t)[outsider]; ok {
			t.Fatal("outsider was added to the group")
		}
	})

	t.Run("plain member cannot add users", func(t *testing.T) {
		err := AddGroupMember(ctx, member, chatID, []bson.ObjectID{outsider})
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("err = %v, want ErrPermissionDenied", err)
		}
		if _, ok := participants(t)[outsider]; ok {
			t.Fatal("member added an outsider")
		}
	})

	t.Run("re-adding the owner does not demote them", func(t *testing.T) {
		err := AddGroupMember(ctx, admin, chatID, []bson.ObjectID{owner})
		if !errors.Is(err, ErrAlreadyParticipant) {
			t.Fatalf("err = %v, want ErrAlreadyParticipant", err)
		}
		if role := participants(t)[owner].Role; role != string(constants.ChatRoleOwner) {
			t.Fatalf("owner role = %q, want %q", role, constants.ChatRoleOwner)
		}
	})

	t.Run("admin adds a new user as pending", func(t *testing.T) {
		if err := AddGroupMember(ctx, admin, chatID, []bson.ObjectID{newcomer, owner}); err != nil {
			t.Fatalf("AddGroupMember: %v", err)
		}
		p := participants(t)
		if got := p[newcomer].RequestStatus; got != string(constants.StatusPending) {
			t.Fatalf("newcomer status = %q, want %q", got, constants.StatusPending)
		}
		if role := p[owner].Role; role != string(constants.ChatRoleOwner) {
			t.Fatalf("owner role = %q after mixed add, want %q", role, constants.ChatRoleOwner)
		}
	})

	t.Run("unknown chat gets not found", func(t *testing.T) {
		err := AddGroupMember(ctx, owner, bson.NewObjectID(), []bson.ObjectID{outsider})
		if !errors.Is(err, ErrChatNotFound) {
			t.Fatalf("err = %v, want ErrChatNotFound", err)
		}
	})
}
