package services

import (
	"errors"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestJoinGroupByInviteRequiresStoredInvite(t *testing.T) {
	ctx := useTestDB(t)

	owner, joiner := bson.NewObjectID(), bson.NewObjectID()
	chatID := bson.NewObjectID()

	for _, id := range []bson.ObjectID{owner, joiner} {
		_, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{
			"_id":      id,
			"username": id.Hex(),
			"profile":  bson.M{"displayName": "user_" + id.Hex()},
		})
		if err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}

	creator := models.LoginUserResponse{ID: owner}
	mint := func(t *testing.T) models.InviteCodeEmbed {
		t.Helper()
		invite, ok := utils.NewInviteCodeWithDefaults(creator, chatID, time.Now().Add(time.Hour))
		if !ok {
			t.Fatal("mint invite code")
		}
		return invite
	}
	live, deleted := mint(t), mint(t)
	deleted.Deleted = true

	_, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":          chatID,
		"chatType":     string(constants.ChatTypeGroup),
		"participants": bson.M{owner.Hex(): bson.M{"role": string(constants.ChatRoleOwner)}},
		"inviteCode":   []models.InviteCodeEmbed{live, deleted},
	})
	if err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	// isMember reports whether joiner is in the chat's participants and
	// whether the chat is in joiner's chats list (what the hub subscribes to).
	isMember := func(t *testing.T) (inParticipants, inChats bool) {
		t.Helper()
		exists := func(coll constants.Collection, id bson.ObjectID, field string) bool {
			n, err := database.DB.Collection(string(coll)).CountDocuments(ctx, bson.M{"_id": id, field: bson.M{"$exists": true}})
			if err != nil {
				t.Fatalf("count %s: %v", coll, err)
			}
			return n > 0
		}
		return exists(constants.ChatColl, chatID, "participants."+joiner.Hex()),
			exists(constants.UserColl, joiner, "chats."+chatID.Hex())
	}

	rejected := []struct {
		name string
		code string
	}{
		{"forged code that decrypts but isn't stored", mint(t).InviteCode},
		{"deleted code", deleted.InviteCode},
		{"garbage code", "not-a-hex-code"},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			err := JoinGroupByInvite(ctx, joiner, tc.code)
			if !errors.Is(err, ErrInvalidInvite) {
				t.Fatalf("err = %v, want ErrInvalidInvite", err)
			}
			if p, c := isMember(t); p || c {
				t.Fatalf("joiner got in: inParticipants=%v inChats=%v", p, c)
			}
		})
	}

	t.Run("stored live code joins", func(t *testing.T) {
		if err := JoinGroupByInvite(ctx, joiner, live.InviteCode); err != nil {
			t.Fatalf("JoinGroupByInvite: %v", err)
		}
		if p, c := isMember(t); !p || !c {
			t.Fatalf("joiner not added: inParticipants=%v inChats=%v", p, c)
		}
	})

	t.Run("joining twice is a conflict", func(t *testing.T) {
		err := JoinGroupByInvite(ctx, joiner, live.InviteCode)
		if !errors.Is(err, ErrAlreadyParticipant) {
			t.Fatalf("err = %v, want ErrAlreadyParticipant", err)
		}
	})
}
