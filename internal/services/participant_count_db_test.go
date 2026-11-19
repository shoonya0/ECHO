package services

import (
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// reportedCount is the participantCount GET groups/pending shows watcher, a pending
// member of chatID, next to the size of the chat's participants map.
func reportedCount(t *testing.T, watcher, chatID bson.ObjectID) (count, participants int) {
	t.Helper()
	invites, err := PendingGroupInvites(t.Context(), watcher)
	if err != nil {
		t.Fatalf("PendingGroupInvites: %v", err)
	}
	count = -1
	for _, inv := range invites {
		if inv.ChatID == chatID {
			count = inv.ParticipantCount
		}
	}
	if count < 0 {
		t.Fatalf("chat %s isn't in the watcher's pending list", chatID.Hex())
	}
	return count, len(loadChat(t, t.Context(), chatID).Participants)
}

// #110: participantCount is the size of participants when read, whatever count is
// stored (an old server copy or a missed write could leave one wrong), through every
// add, join and removal. The steps are the ones in #16.
func TestParticipantCountFollowsMembership(t *testing.T) {
	ctx := useTestDB(t)
	owner, m1, m2, a1, a2, joiner, watcher := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(),
		bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, m1, m2, a1, a2, joiner, watcher)

	invite, ok := utils.NewInviteCodeWithDefaults(time.Now().Add(time.Hour))
	if !ok {
		t.Fatal("mint invite code")
	}
	chatID := bson.NewObjectID()
	_, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":      chatID,
		"chatType": string(constants.ChatTypeGroup),
		"ownerId":  owner,
		"participants": bson.M{
			owner.Hex(): bson.M{"role": string(constants.ChatRoleOwner)},
			m1.Hex():    bson.M{"role": string(constants.ChatRoleMember)},
			m2.Hex():    bson.M{"role": string(constants.ChatRoleMember)},
			watcher.Hex(): bson.M{"role": string(constants.ChatRoleMember),
				"requestStatus": string(constants.StatusPending), "requestedBy": owner},
		},
		"stats":      bson.M{"participantCount": 99}, // wrong on purpose
		"inviteCode": []models.InviteCodeEmbed{invite},
	})
	if err != nil {
		t.Fatalf("insert chat: %v", err)
	}
	if _, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx,
		bson.M{"_id": watcher}, bson.M{"$set": bson.M{"chats." + chatID.Hex(): 0}}); err != nil {
		t.Fatalf("give the watcher a chats entry: %v", err)
	}

	check := func(t *testing.T, step string, want int) {
		t.Helper()
		count, n := reportedCount(t, watcher, chatID)
		if count != want || n != want {
			t.Fatalf("after %s: participantCount = %d, participants = %d; want both %d", step, count, n, want)
		}
	}

	check(t, "a wrong stored count", 4)

	// Someone already in the group, or listed twice, is added and counted once at most.
	if err := AddGroupMember(ctx, owner, chatID, []bson.ObjectID{a1, a2, a1, m1}); err != nil {
		t.Fatalf("add members: %v", err)
	}
	check(t, "adding 2 members", 6)

	if err := JoinGroupByInvite(ctx, joiner, invite.InviteCode); err != nil {
		t.Fatalf("join by invite: %v", err)
	}
	check(t, "a join by invite", 7)

	// A second join by the same user is refused and counts nothing.
	if err := JoinGroupByInvite(ctx, joiner, invite.InviteCode); err == nil {
		t.Fatal("second join succeeded")
	}
	check(t, "a repeated join", 7)

	if _, err := RemoveGroupMember(ctx, owner, chatID, []bson.ObjectID{m1, m2, a1, a2}); err != nil {
		t.Fatalf("remove members: %v", err)
	}
	check(t, "removing 4 members", 3)
}

// #110: a new group stores no count, so none can go stale; the reply still has one.
func TestCreateGroupChatStoresNoCount(t *testing.T) {
	ctx := useTestDB(t)
	owner, member := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, member)

	chat, err := CreateGroupChat(asUser(ctx, owner), "g", "", []bson.ObjectID{member})
	if err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	if chat.Stats.ParticipantCount != 2 {
		t.Fatalf("reply participantCount = %d, want 2", chat.Stats.ParticipantCount)
	}
	n, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx,
		bson.M{"_id": chat.ChatID, "stats.participantCount": bson.M{"$exists": true}})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatal("the new group stores stats.participantCount, want it computed on read")
	}
}
