package services

import (
	"errors"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Invite routes find the group by the stored code, not by decrypting it (#15).
// An opaque random code and a code in the old encrypted format both work as
// long as they are stored, so codes made before the change keep working
// until they expire.
func TestInviteRoutesFindTheGroupByTheStoredCode(t *testing.T) {
	ctx := useTestDB(t)
	users := database.DB.Collection(string(constants.UserColl))
	chats := database.DB.Collection(string(constants.ChatColl))

	owner, joiner, target := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	for _, id := range []bson.ObjectID{owner, joiner, target} {
		if _, err := users.InsertOne(ctx, bson.M{"_id": id, "username": id.Hex(), "profile": bson.M{"displayName": "u"}}); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}

	const opaque = "00112233445566778899aabbccddeeff"
	// Any hex string longer than a new code stands in for an old encrypted
	// code: nothing decrypts it any more.
	const legacy = "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f6071829"
	groupID := bson.NewObjectID()
	invite := func(code string, expires time.Time) models.InviteCodeEmbed {
		return models.InviteCodeEmbed{InviteCode: code, UserIDs: []bson.ObjectID{}, ExpiredDate: expires, Status: "active"}
	}
	if _, err := chats.InsertOne(ctx, bson.M{
		"_id":          groupID,
		"chatType":     string(constants.ChatTypeGroup),
		"participants": bson.M{owner.Hex(): bson.M{"role": string(constants.ChatRoleOwner)}},
		"inviteCode": []models.InviteCodeEmbed{
			invite(opaque, time.Now().Add(time.Hour)),
			invite(legacy, time.Now().Add(time.Hour)),
			invite("ffeeddccbbaa99887766554433221100", time.Now().Add(-time.Minute)),
		},
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	t.Run("join with an opaque code", func(t *testing.T) {
		if err := JoinGroupByInvite(ctx, joiner, opaque); err != nil {
			t.Fatalf("JoinGroupByInvite: %v", err)
		}
		n, err := chats.CountDocuments(ctx, chatMemberFilter(groupID, joiner))
		if err != nil || n != 1 {
			t.Fatalf("joiner in participants: n=%d err=%v", n, err)
		}
	})

	t.Run("joined users of an opaque code", func(t *testing.T) {
		ids, err := GetJoinedUsersByInvite(ctx, owner, opaque)
		if err != nil {
			t.Fatalf("GetJoinedUsersByInvite: %v", err)
		}
		if len(ids) != 1 || ids[0] != joiner {
			t.Fatalf("joined = %v, want [%s]", ids, joiner.Hex())
		}
	})

	t.Run("send a code in the old format", func(t *testing.T) {
		if err := SendInviteToUser(ctx, owner, legacy, target, false); err != nil {
			t.Fatalf("SendInviteToUser: %v", err)
		}
	})

	t.Run("send an expired code", func(t *testing.T) {
		err := SendInviteToUser(ctx, owner, "ffeeddccbbaa99887766554433221100", target, false)
		if !errors.Is(err, ErrInvalidInvite) {
			t.Fatalf("err = %v, want ErrInvalidInvite", err)
		}
	})

	t.Run("delete an opaque code", func(t *testing.T) {
		if err := DeleteInvite(ctx, owner, opaque); err != nil {
			t.Fatalf("DeleteInvite: %v", err)
		}
		n, err := chats.CountDocuments(ctx, bson.M{"_id": groupID, "inviteCode": bson.M{"$elemMatch": bson.M{"inviteCode": opaque, "deleted": true}}})
		if err != nil || n != 1 {
			t.Fatalf("code marked deleted: n=%d err=%v", n, err)
		}
	})

	for _, code := range []string{"0123456789abcdef0123456789abcdef", "not-hex", ""} {
		t.Run("unknown code "+code, func(t *testing.T) {
			if err := JoinGroupByInvite(ctx, bson.NewObjectID(), code); !errors.Is(err, ErrInvalidInvite) {
				t.Fatalf("err = %v, want ErrInvalidInvite", err)
			}
		})
	}
}
