package services

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

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

func participantStatus(t *testing.T, ctx context.Context, chatID, userID bson.ObjectID) (string, bool) {
	t.Helper()
	p, ok := loadChat(t, ctx, chatID).Participants[userID]
	return p.RequestStatus, ok
}

// Accepting makes the pending member accepted, keeps their chats entry, drops
// their invitations to the group and lets the hub follow it (#5, #17).
func TestAcceptGroupInvite(t *testing.T) {
	ctx := useTestDB(t)
	owner, user, outsider := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	chatID := insertInviteGroup(t, ctx, owner, user)
	insertTestUsers(t, ctx, outsider)
	code := mintInvite(t, time.Now().Add(time.Hour))
	if _, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(ctx, bson.M{"_id": chatID},
		bson.M{"$push": bson.M{"inviteCode": code}}); err != nil {
		t.Fatalf("add code: %v", err)
	}
	if _, err := addInvitation(ctx, code.InviteCode, user); err != nil {
		t.Fatalf("add invitation: %v", err)
	}

	if err := AcceptGroupInvite(ctx, user, chatID); err != nil {
		t.Fatalf("AcceptGroupInvite: %v", err)
	}
	if status, _ := participantStatus(t, ctx, chatID, user); status != string(constants.StatusAccepted) {
		t.Fatalf("status = %q, want accepted", status)
	}
	if !hasChatEntry(t, ctx, user, chatID) {
		t.Fatalf("chats entry missing after accept")
	}
	if invites, err := GetAllInvitesOfUser(ctx, user); err != nil || len(invites) != 0 {
		t.Fatalf("invitations = %v, err %v; want none", invites, err)
	}
	if got, err := GetUserChats(ctx, user); err != nil || !slices.Contains(got, chatID) {
		t.Fatalf("GetUserChats = %v, err %v; want the accepted group", got, err)
	}

	if err := AcceptGroupInvite(ctx, user, chatID); !errors.Is(err, ErrAlreadyParticipant) {
		t.Fatalf("second accept: err %v, want ErrAlreadyParticipant", err)
	}
	// An accept that stopped after the status write (a 500 to the client) is
	// finished by the retry, which still answers 409 (code review of #5).
	if _, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx, bson.M{"_id": user},
		bson.M{"$unset": bson.M{"chats." + chatID.Hex(): ""}}); err != nil {
		t.Fatalf("drop chats entry: %v", err)
	}
	if err := AcceptGroupInvite(ctx, user, chatID); !errors.Is(err, ErrAlreadyParticipant) {
		t.Fatalf("retried accept: err %v, want ErrAlreadyParticipant", err)
	}
	if !hasChatEntry(t, ctx, user, chatID) {
		t.Fatalf("retried accept did not restore the chats entry")
	}
	// No invite and no such group answer the same, so group IDs can't be probed (#59).
	if err := AcceptGroupInvite(ctx, outsider, chatID); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("outsider: err %v, want ErrChatNotFound", err)
	}
	if err := AcceptGroupInvite(ctx, user, bson.NewObjectID()); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("unknown group: err %v, want ErrChatNotFound", err)
	}
}

// Declining takes the invitee out of the group and its count, and off their
// chats list (#5). They can be added again later.
func TestDeclineGroupInvite(t *testing.T) {
	ctx := useTestDB(t)
	owner, user := bson.NewObjectID(), bson.NewObjectID()
	chatID := insertInviteGroup(t, ctx, owner, user)

	if err := DeclineGroupInvite(ctx, user, chatID); err != nil {
		t.Fatalf("DeclineGroupInvite: %v", err)
	}
	chat := loadChat(t, ctx, chatID)
	if _, ok := chat.Participants[user]; ok {
		t.Fatalf("declined user still a participant")
	}
	if chat.Stats.ParticipantCount != 1 {
		t.Fatalf("participantCount = %d, want 1", chat.Stats.ParticipantCount)
	}
	if hasChatEntry(t, ctx, user, chatID) {
		t.Fatalf("chats entry left after decline")
	}

	if err := DeclineGroupInvite(ctx, user, chatID); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("second decline: err %v, want ErrChatNotFound", err)
	}
	if err := DeclineGroupInvite(ctx, owner, chatID); !errors.Is(err, ErrAlreadyParticipant) {
		t.Fatalf("accepted member declining: err %v, want ErrAlreadyParticipant", err)
	}
	if err := AddGroupMember(ctx, owner, chatID, []bson.ObjectID{user}); err != nil {
		t.Fatalf("adding the declined user again: %v", err)
	}
}

// The pending list holds only the user's own pending groups (#5).
func TestPendingGroupInvites(t *testing.T) {
	ctx := useTestDB(t)
	owner, user, other := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	pending := insertInviteGroup(t, ctx, owner, user, other)
	accepted := insertInviteGroup(t, ctx, owner, user)
	if err := AcceptGroupInvite(ctx, user, accepted); err != nil {
		t.Fatalf("accept: %v", err)
	}
	insertInviteGroup(t, ctx, owner, other) // someone else's invite

	got, err := PendingGroupInvites(ctx, user)
	if err != nil {
		t.Fatalf("PendingGroupInvites: %v", err)
	}
	if len(got) != 1 || got[0].ChatID != pending {
		t.Fatalf("pending = %+v, want only %s", got, pending.Hex())
	}
	if got[0].InvitedBy != owner || got[0].ParticipantCount != 3 || got[0].Name == "" {
		t.Fatalf("pending entry = %+v, want inviter %s, 3 participants and a name", got[0], owner.Hex())
	}
}
