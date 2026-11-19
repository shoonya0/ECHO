package services

import (
	"context"
	"strings"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Joining by code, accepting and declining an invite each write the group and
// then the user. A failed user write left the group changed (#37); each test
// fails that write and checks the group write was rolled back.

func newTestGroup(t *testing.T, ctx context.Context, owner bson.ObjectID, invitees ...bson.ObjectID) *models.Chat {
	t.Helper()
	chat, err := CreateGroupChat(asUser(ctx, owner), "g", "", invitees)
	if err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	return chat
}

// checkFailedAt fails unless err came from the write described by want: the
// failpoint counts updates, so a new update before the chat write would make
// it fail the chat write instead, and the test would no longer test a rollback.
func checkFailedAt(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("the call succeeded, want the failed user write's error")
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want it from the user write (%q)", err, want)
	}
}

func TestJoinGroupByInviteRollsBackOnFailedUserUpdate(t *testing.T) {
	ctx := useTestDB(t)
	owner, joiner := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, joiner)
	chat := newTestGroup(t, ctx, owner)

	testutil.FailCommand(t, ctx, "update", 1) // the chat write passes
	checkFailedAt(t, JoinGroupByInvite(ctx, joiner, chat.InviteCode[0].InviteCode), "failed to add chat to user")

	got := loadTestChat(t, ctx, chat.ChatID)
	if _, ok := got.Participants[joiner]; ok {
		t.Error("joiner is a participant without a chats entry, want the join rolled back")
	}
	if n := len(got.Participants); n != 1 {
		t.Errorf("participants = %d, want 1", n)
	}
}

func TestAcceptGroupInviteRollsBackOnFailedUserUpdate(t *testing.T) {
	ctx := useTestDB(t)
	owner, invitee := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, invitee)
	chat := newTestGroup(t, ctx, owner, invitee)

	testutil.FailCommand(t, ctx, "update", 1) // the chat write passes
	checkFailedAt(t, AcceptGroupInvite(ctx, invitee, chat.ChatID), "failed to add chat to user")

	got := loadTestChat(t, ctx, chat.ChatID).Participants[invitee]
	if got.RequestStatus != string(constants.StatusPending) {
		t.Errorf("invitee status = %q, want still pending", got.RequestStatus)
	}
	if n := len(loadTestUser(t, ctx, invitee).ChatInvitations); n != 1 {
		t.Errorf("invitee has %d invitations, want the 1 still there", n)
	}
}

func TestDeclineGroupInviteRollsBackOnFailedUserUpdate(t *testing.T) {
	ctx := useTestDB(t)
	owner, invitee := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, invitee)
	chat := newTestGroup(t, ctx, owner, invitee)

	testutil.FailCommand(t, ctx, "update", 1) // the chat write passes
	checkFailedAt(t, DeclineGroupInvite(ctx, invitee, chat.ChatID), "failed to remove the group from the invitee")

	got := loadTestChat(t, ctx, chat.ChatID)
	if _, ok := got.Participants[invitee]; !ok {
		t.Error("invitee left the group while keeping its chats entry, want the decline rolled back")
	}
	if n := len(got.Participants); n != 2 {
		t.Errorf("participants = %d, want 2", n)
	}
}
