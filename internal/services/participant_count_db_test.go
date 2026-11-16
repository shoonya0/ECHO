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

// storedCount reads a chat's stats.participantCount and the size of its participants map.
func storedCount(t *testing.T, chatID bson.ObjectID) (count, participants int) {
	t.Helper()
	var chat models.Chat
	if err := database.DB.Collection(string(constants.ChatColl)).FindOne(t.Context(), bson.M{"_id": chatID}).Decode(&chat); err != nil {
		t.Fatalf("load chat: %v", err)
	}
	return chat.Stats.ParticipantCount, len(chat.Participants)
}

// requireInt32Count fails unless stats.participantCount is stored as a 32-bit int, the
// type CreateGroupChat, $inc and the backfill's $size all write, so the field keeps one type.
func requireInt32Count(t *testing.T, chatID bson.ObjectID) {
	t.Helper()
	n, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(t.Context(),
		bson.M{"_id": chatID, "stats.participantCount": bson.M{"$type": "int"}})
	if err != nil {
		t.Fatalf("check count type: %v", err)
	}
	if n != 1 {
		t.Fatalf("chat %s: stats.participantCount isn't stored as int32", chatID.Hex())
	}
}

// #16: participantCount follows every add, join and removal, so it can't drift or go
// negative. The steps are the ones in the issue.
func TestParticipantCountFollowsMembership(t *testing.T) {
	ctx := useTestDB(t)
	owner, m1, m2, a1, a2, joiner := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(),
		bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, m1, m2, a1, a2, joiner)

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
		},
		"stats":      bson.M{"participantCount": 3},
		"inviteCode": []models.InviteCodeEmbed{invite},
	})
	if err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	check := func(t *testing.T, step string, want int) {
		t.Helper()
		count, n := storedCount(t, chatID)
		if count != want || n != want {
			t.Fatalf("after %s: participantCount = %d, participants = %d; want both %d", step, count, n, want)
		}
		requireInt32Count(t, chatID)
	}

	// Someone already in the group, or listed twice, is added and counted once at most.
	if err := AddGroupMember(ctx, owner, chatID, []bson.ObjectID{a1, a2, a1, m1}); err != nil {
		t.Fatalf("add members: %v", err)
	}
	check(t, "adding 2 members", 5)

	if err := JoinGroupByInvite(ctx, joiner, invite.InviteCode); err != nil {
		t.Fatalf("join by invite: %v", err)
	}
	check(t, "a join by invite", 6)

	// A second join by the same user is refused and counts nothing.
	if err := JoinGroupByInvite(ctx, joiner, invite.InviteCode); err == nil {
		t.Fatal("second join succeeded")
	}
	check(t, "a repeated join", 6)

	if _, err := RemoveGroupMember(ctx, owner, chatID, []bson.ObjectID{m1, m2, a1, a2}); err != nil {
		t.Fatalf("remove members: %v", err)
	}
	check(t, "removing 4 members", 2)
}

// #16: groups whose count drifted before the fix get it reset to the size of
// participants. Running it again changes nothing.
func TestBackfillParticipantCounts(t *testing.T) {
	ctx := useTestDB(t)
	chats := database.DB.Collection(string(constants.ChatColl))
	a, b := bson.NewObjectID(), bson.NewObjectID()
	two := bson.M{a.Hex(): bson.M{"role": "owner"}, b.Hex(): bson.M{"role": "member"}}

	negative, missing, correct, direct, malformed := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	docs := []any{
		bson.M{"_id": negative, "chatType": string(constants.ChatTypeGroup), "participants": two, "stats": bson.M{"participantCount": -1}},
		bson.M{"_id": missing, "chatType": string(constants.ChatTypeGroup), "participants": two},
		bson.M{"_id": correct, "chatType": string(constants.ChatTypeGroup), "participants": two, "stats": bson.M{"participantCount": 2}},
		bson.M{"_id": direct, "chatType": string(constants.ChatTypeDirect), "participants": two, "stats": bson.M{"participantCount": 0}},
		// Must be skipped, not fail the update for every other group.
		bson.M{"_id": malformed, "chatType": string(constants.ChatTypeGroup), "participants": bson.A{a, b}, "stats": bson.M{"participantCount": 7}},
	}
	if _, err := chats.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert chats: %v", err)
	}

	n, err := BackfillParticipantCounts(ctx)
	if err != nil {
		t.Fatalf("BackfillParticipantCounts: %v", err)
	}
	if n != 2 {
		t.Fatalf("updated %d groups, want 2", n)
	}
	for _, id := range []bson.ObjectID{negative, missing, correct} {
		if count, _ := storedCount(t, id); count != 2 {
			t.Fatalf("group %s: participantCount = %d, want 2", id.Hex(), count)
		}
		requireInt32Count(t, id)
	}
	var bad struct {
		Stats struct {
			ParticipantCount int `bson:"participantCount"`
		} `bson:"stats"`
	}
	if err := chats.FindOne(ctx, bson.M{"_id": malformed}).Decode(&bad); err != nil {
		t.Fatalf("load malformed group: %v", err)
	}
	if bad.Stats.ParticipantCount != 7 {
		t.Fatalf("malformed group participantCount = %d, want it left at 7", bad.Stats.ParticipantCount)
	}
	if count, _ := storedCount(t, direct); count != 0 {
		t.Fatalf("direct chat participantCount = %d, want it left at 0", count)
	}

	if n, err := BackfillParticipantCounts(ctx); err != nil || n != 0 {
		t.Fatalf("second run = %d, %v; want 0, nil", n, err)
	}
}
