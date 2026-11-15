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

// Only a group's owner and admins may create, list, delete or send invite codes,
// or see who joined by one (#30, decided 2026-11-15). Before, any participant
// could, pending members included.
func TestInviteCodesNeedOwnerOrAdmin(t *testing.T) {
	ctx := useTestDB(t)

	owner, admin, member, pending := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	outsider, target, inGroup := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	for _, id := range []bson.ObjectID{owner, admin, member, pending, outsider, target, inGroup} {
		if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": id, "username": "user-" + id.Hex()}); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}

	groupID := bson.NewObjectID()
	code, deletedCode := "00112233445566778899aabbccddeeff", "ffeeddccbbaa99887766554433221100"
	participant := func(role, status string) bson.M { return bson.M{"role": role, "status": status} }
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":      groupID,
		"chatType": string(constants.ChatTypeGroup),
		"participants": bson.M{
			owner.Hex():   participant(string(constants.ChatRoleOwner), string(constants.StatusAccepted)),
			admin.Hex():   participant(string(constants.ChatRoleAdmin), string(constants.StatusAccepted)),
			member.Hex():  participant(string(constants.ChatRoleMember), string(constants.StatusAccepted)),
			pending.Hex(): participant(string(constants.ChatRoleMember), string(constants.StatusPending)),
			inGroup.Hex(): participant(string(constants.ChatRoleMember), string(constants.StatusAccepted)),
		},
		"inviteCode": bson.A{
			models.InviteCodeEmbed{InviteCode: code, UserIDs: []bson.ObjectID{inGroup}, ExpiredDate: time.Now().Add(time.Hour)},
			models.InviteCodeEmbed{InviteCode: deletedCode, ExpiredDate: time.Now().Add(time.Hour), Deleted: true},
		},
	}); err != nil {
		t.Fatalf("insert group: %v", err)
	}

	invitations := func(t *testing.T, userID bson.ObjectID) []models.ChatInvitationEmbed {
		t.Helper()
		got, err := GetAllInvitesOfUser(ctx, userID)
		if err != nil {
			t.Fatalf("GetAllInvitesOfUser: %v", err)
		}
		return got
	}
	codeDeleted := func(t *testing.T) bool {
		t.Helper()
		invites, err := GetGroupInvites(ctx, owner, groupID)
		if err != nil {
			t.Fatalf("GetGroupInvites as owner: %v", err)
		}
		for _, inv := range invites {
			if inv.InviteCode == code {
				return inv.Deleted
			}
		}
		t.Fatalf("code %s is missing from the group's invites", code)
		return false
	}

	actions := []struct {
		name string
		call func(userID bson.ObjectID) error
	}{
		{"create", func(u bson.ObjectID) error { _, err := CreateInvite(ctx, u, groupID); return err }},
		{"list", func(u bson.ObjectID) error { _, err := GetGroupInvites(ctx, u, groupID); return err }},
		{"joined users", func(u bson.ObjectID) error { _, err := GetJoinedUsersByInvite(ctx, u, code); return err }},
		{"send", func(u bson.ObjectID) error { return SendInviteToUser(ctx, u, code, target, false) }},
	}
	refused := []struct {
		name string
		user bson.ObjectID
		want error
	}{
		{"member", member, ErrPermissionDenied},
		{"pending member", pending, ErrPermissionDenied},
		{"outsider", outsider, ErrNotParticipant},
	}
	for _, a := range actions {
		for _, r := range refused {
			t.Run(a.name+" by "+r.name+" is refused", func(t *testing.T) {
				if err := a.call(r.user); !errors.Is(err, r.want) {
					t.Fatalf("err = %v, want %v", err, r.want)
				}
			})
		}
	}
	t.Run("delete by a member is refused and keeps the code", func(t *testing.T) {
		if err := DeleteInvite(ctx, member, code); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("err = %v, want ErrPermissionDenied", err)
		}
		if codeDeleted(t) {
			t.Fatal("a member's delete removed the code")
		}
	})
	if got := invitations(t, target); len(got) != 0 {
		t.Fatalf("refused sends left invitations %+v on the target", got)
	}

	for _, a := range actions {
		for _, who := range []struct {
			name string
			user bson.ObjectID
		}{{"owner", owner}, {"admin", admin}} {
			t.Run(a.name+" by "+who.name+" works", func(t *testing.T) {
				if err := a.call(who.user); err != nil {
					t.Fatalf("err = %v", err)
				}
			})
		}
	}

	t.Run("the list includes deleted codes, marked", func(t *testing.T) {
		invites, err := GetGroupInvites(ctx, admin, groupID)
		if err != nil {
			t.Fatalf("GetGroupInvites: %v", err)
		}
		for _, inv := range invites {
			if inv.InviteCode == deletedCode && inv.Deleted {
				return
			}
		}
		t.Fatalf("invites = %+v, want %s marked deleted", invites, deletedCode)
	})

	t.Run("sending twice leaves one invitation", func(t *testing.T) {
		if got := invitations(t, target); len(got) != 1 || got[0].Token != code {
			t.Fatalf("invitations = %+v, want one for %s after sends by owner and admin", got, code)
		}
	})

	t.Run("sending to a user who doesn't exist is refused", func(t *testing.T) {
		if err := SendInviteToUser(ctx, owner, code, bson.NewObjectID(), false); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("err = %v, want ErrUserNotFound", err)
		}
	})

	t.Run("sending to someone already in the group is refused", func(t *testing.T) {
		if err := SendInviteToUser(ctx, owner, code, inGroup, false); !errors.Is(err, ErrAlreadyParticipant) {
			t.Fatalf("err = %v, want ErrAlreadyParticipant", err)
		}
	})

	t.Run("listing a chat that doesn't exist is not found", func(t *testing.T) {
		if _, err := GetGroupInvites(ctx, owner, bson.NewObjectID()); !errors.Is(err, ErrChatNotFound) {
			t.Fatalf("err = %v, want ErrChatNotFound", err)
		}
	})

	t.Run("joining clears the pending invitation", func(t *testing.T) {
		if err := JoinGroupByInvite(ctx, target, code); err != nil {
			t.Fatalf("JoinGroupByInvite: %v", err)
		}
		if got := invitations(t, target); len(got) != 0 {
			t.Fatalf("invitations after joining = %+v, want none", got)
		}
	})

	t.Run("delete by an admin works", func(t *testing.T) {
		if err := DeleteInvite(ctx, admin, code); err != nil {
			t.Fatalf("DeleteInvite: %v", err)
		}
		if !codeDeleted(t) {
			t.Fatal("the code isn't marked deleted")
		}
	})
}
