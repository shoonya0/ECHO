package services

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// pendingInvitations returns the invite codes in userID's chatInvitations.
func pendingInvitations(t *testing.T, userID bson.ObjectID) []string {
	t.Helper()
	var u struct {
		ChatInvitations []struct {
			Token string `bson:"token"`
		} `bson:"chatInvitations"`
	}
	if err := database.DB.Collection(string(constants.UserColl)).FindOne(t.Context(), bson.M{"_id": userID}).Decode(&u); err != nil {
		t.Fatalf("load user: %v", err)
	}
	tokens := make([]string, 0, len(u.ChatInvitations))
	for _, inv := range u.ChatInvitations {
		tokens = append(tokens, inv.Token)
	}
	return tokens
}

// hasChatRef reports whether userID's chats map has an entry for chatID, which the hub
// needs to deliver the chat's live messages.
func hasChatRef(t *testing.T, userID, chatID bson.ObjectID) bool {
	t.Helper()
	n, err := database.DB.Collection(string(constants.UserColl)).CountDocuments(t.Context(),
		bson.M{"_id": userID, "chats." + chatID.Hex(): bson.M{"$exists": true}})
	if err != nil {
		t.Fatalf("check chats entry: %v", err)
	}
	return n == 1
}

// #103: someone added to a group as pending accepts by joining with one of its codes.
func TestPendingMemberAcceptsByJoiningWithCode(t *testing.T) {
	ctx := useTestDB(t)
	owner, invitee := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, invitee)
	ownerCtx := context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: owner, Username: owner.Hex()})

	chat, err := CreateGroupChat(ownerCtx, "g", "", []bson.ObjectID{invitee})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	code := chat.InviteCode[0].InviteCode
	if got := participantsOf(t, chat.ChatID)[invitee].RequestStatus; got != string(constants.StatusPending) {
		t.Fatalf("invitee status before joining = %q, want pending", got)
	}
	if got := pendingInvitations(t, invitee); !slices.Equal(got, []string{code}) {
		t.Fatalf("invitations before joining = %v, want [%s]", got, code)
	}

	if err := JoinGroupByInvite(ctx, invitee, code); err != nil {
		t.Fatalf("pending member joins with the code: %v", err)
	}

	p := participantsOf(t, chat.ChatID)
	if got := p[invitee]; got.RequestStatus != string(constants.StatusAccepted) || got.Role != string(constants.ChatRoleMember) {
		t.Fatalf("invitee after joining: status %q role %q, want accepted member", got.RequestStatus, got.Role)
	}
	if got := p[owner].Role; got != string(constants.ChatRoleOwner) {
		t.Fatalf("owner role = %q, want owner", got)
	}
	if count, n := storedCount(t, chat.ChatID); count != 2 || n != 2 {
		t.Fatalf("participantCount = %d, participants = %d; want 2 and 2 (accepting adds nobody)", count, n)
	}
	if got := pendingInvitations(t, invitee); len(got) != 0 {
		t.Fatalf("invitations after joining = %v, want none", got)
	}
	if !hasChatRef(t, invitee, chat.ChatID) {
		t.Fatal("invitee has no chats entry for the group after joining")
	}

	if err := JoinGroupByInvite(ctx, invitee, code); !errors.Is(err, ErrAlreadyParticipant) {
		t.Fatalf("joining again: err = %v, want ErrAlreadyParticipant", err)
	}
}

// #103: a pending member already counts toward the size limit, so a full group doesn't
// stop them from accepting. An unusable code still does.
func TestPendingMemberJoinChecksTheCodeNotTheSize(t *testing.T) {
	ctx := useTestDB(t)
	owner, pending := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, pending)
	live := mintInvite(t, time.Now().Add(time.Hour))
	expired := mintInvite(t, time.Now().Add(-time.Minute))
	chatID := insertSizedGroup(t, ctx, owner, 0, 2, live, expired)
	_, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, bson.M{"_id": chatID}, bson.M{"$set": bson.M{
		participantField(pending): bson.M{"role": string(constants.ChatRoleMember), "requestStatus": string(constants.StatusPending)},
	}})
	if err != nil {
		t.Fatalf("add pending member: %v", err)
	}

	if err := JoinGroupByInvite(ctx, pending, expired.InviteCode); !errors.Is(err, ErrInvalidInvite) {
		t.Fatalf("expired code: err = %v, want ErrInvalidInvite", err)
	}
	if got := participantsOf(t, chatID)[pending].RequestStatus; got != string(constants.StatusPending) {
		t.Fatalf("status after expired code = %q, want still pending", got)
	}

	if err := JoinGroupByInvite(ctx, pending, live.InviteCode); err != nil {
		t.Fatalf("pending member of a full group joins: %v", err)
	}
	if got := participantsOf(t, chatID)[pending].RequestStatus; got != string(constants.StatusAccepted) {
		t.Fatalf("status = %q, want accepted", got)
	}
}
