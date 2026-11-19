package services

import (
	"context"
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

func TestJoinGroupByInviteRollsBackOnFailedUserUpdate(t *testing.T) {
	ctx := useTestDB(t)
	owner, joiner := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, joiner)
	chat := newTestGroup(t, ctx, owner)

	testutil.FailCommand(t, ctx, "update", 1) // the chat write passes
	if err := JoinGroupByInvite(ctx, joiner, chat.InviteCode[0].InviteCode); err == nil {
		t.Fatal("JoinGroupByInvite succeeded, want the failed write's error")
	}

	got := loadTestChat(t, ctx, chat.ChatID)
	if _, ok := got.Participants[joiner]; ok {
		t.Error("joiner is a participant without a chats entry, want the join rolled back")
	}
	if got.Stats.ParticipantCount != 1 {
		t.Errorf("participantCount = %d, want 1", got.Stats.ParticipantCount)
	}
}

func TestAcceptGroupInviteRollsBackOnFailedUserUpdate(t *testing.T) {
	ctx := useTestDB(t)
	owner, invitee := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, invitee)
	chat := newTestGroup(t, ctx, owner, invitee)

	testutil.FailCommand(t, ctx, "update", 1) // the chat write passes
	if err := AcceptGroupInvite(ctx, invitee, chat.ChatID); err == nil {
		t.Fatal("AcceptGroupInvite succeeded, want the failed write's error")
	}

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
	if err := DeclineGroupInvite(ctx, invitee, chat.ChatID); err == nil {
		t.Fatal("DeclineGroupInvite succeeded, want the failed write's error")
	}

	got := loadTestChat(t, ctx, chat.ChatID)
	if _, ok := got.Participants[invitee]; !ok {
		t.Error("invitee left the group while keeping its chats entry, want the decline rolled back")
	}
	if got.Stats.ParticipantCount != 2 {
		t.Errorf("participantCount = %d, want 2", got.Stats.ParticipantCount)
	}
}
