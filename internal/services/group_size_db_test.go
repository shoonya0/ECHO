package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// insertSizedGroup stores a group owned by owner with members more participants and
// the given settings.maxParticipants (0 leaves the field out, as on old groups), plus
// the given invite codes.
func insertSizedGroup(t *testing.T, ctx context.Context, owner bson.ObjectID, members, maxParticipants int, invites ...models.InviteCodeEmbed) bson.ObjectID {
	t.Helper()
	participants := bson.M{owner.Hex(): bson.M{"role": string(constants.ChatRoleOwner)}}
	for range members {
		participants[bson.NewObjectID().Hex()] = bson.M{"role": string(constants.ChatRoleMember)}
	}
	doc := bson.M{
		"_id":          bson.NewObjectID(),
		"chatType":     string(constants.ChatTypeGroup),
		"ownerId":      owner,
		"participants": participants,
		"stats":        bson.M{"participantCount": len(participants)},
		"inviteCode":   invites,
	}
	if maxParticipants > 0 {
		doc["settings"] = bson.M{"maxParticipants": maxParticipants}
	}
	if invites == nil {
		doc["inviteCode"] = bson.A{}
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, doc); err != nil {
		t.Fatalf("insert group: %v", err)
	}
	return doc["_id"].(bson.ObjectID)
}

func participantsOf(t *testing.T, chatID bson.ObjectID) map[bson.ObjectID]models.ParticipantEmbed {
	t.Helper()
	var chat models.Chat
	if err := database.DB.Collection(string(constants.ChatColl)).FindOne(t.Context(), bson.M{"_id": chatID}).Decode(&chat); err != nil {
		t.Fatalf("load chat: %v", err)
	}
	return chat.Participants
}

func mintInvite(t *testing.T, expires time.Time) models.InviteCodeEmbed {
	t.Helper()
	invite, ok := utils.NewInviteCodeWithDefaults(expires)
	if !ok {
		t.Fatal("mint invite code")
	}
	return invite
}

// #68: adding members never takes a group past settings.maxParticipants. An add that
// would is refused as a whole.
func TestAddGroupMemberRespectsSizeLimit(t *testing.T) {
	ctx := useTestDB(t)
	owner, u1, u2, u3 := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, u1, u2, u3)
	chatID := insertSizedGroup(t, ctx, owner, 1, 3) // 2 of 3

	if err := AddGroupMember(ctx, owner, chatID, []bson.ObjectID{u1, u2}); !errors.Is(err, ErrGroupFull) {
		t.Fatalf("add 2 to a group with 1 free place: err = %v, want ErrGroupFull", err)
	}
	if n := len(participantsOf(t, chatID)); n != 2 {
		t.Fatalf("participants after refused add = %d, want 2 (nobody added)", n)
	}

	if err := AddGroupMember(ctx, owner, chatID, []bson.ObjectID{u1}); err != nil {
		t.Fatalf("add 1 into the last place: %v", err)
	}
	if err := AddGroupMember(ctx, owner, chatID, []bson.ObjectID{u3}); !errors.Is(err, ErrGroupFull) {
		t.Fatalf("add to a full group: err = %v, want ErrGroupFull", err)
	}
	if n := len(participantsOf(t, chatID)); n != 3 {
		t.Fatalf("participants = %d, want 3", n)
	}
}

// #68: a group with no stored limit (from before the field was set) uses the default.
func TestAddGroupMemberDefaultSizeLimit(t *testing.T) {
	ctx := useTestDB(t)
	owner, u1 := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, u1)
	chatID := insertSizedGroup(t, ctx, owner, constants.DefaultMaxGroupParticipants-1, 0)

	if err := AddGroupMember(ctx, owner, chatID, []bson.ObjectID{u1}); !errors.Is(err, ErrGroupFull) {
		t.Fatalf("add to a group at the default limit: err = %v, want ErrGroupFull", err)
	}
}

// #68: a join by invite code into a full group is refused, but a bad code still says so.
func TestJoinGroupByInviteRespectsSizeLimit(t *testing.T) {
	ctx := useTestDB(t)
	owner, joiner := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, owner, joiner)
	live := mintInvite(t, time.Now().Add(time.Hour))
	expired := mintInvite(t, time.Now().Add(-time.Minute))
	chatID := insertSizedGroup(t, ctx, owner, 1, 2, live, expired) // full

	if err := JoinGroupByInvite(ctx, joiner, live.InviteCode); !errors.Is(err, ErrGroupFull) {
		t.Fatalf("join a full group: err = %v, want ErrGroupFull", err)
	}
	if _, ok := participantsOf(t, chatID)[joiner]; ok {
		t.Fatal("joiner was added to a full group")
	}
	if err := JoinGroupByInvite(ctx, joiner, expired.InviteCode); !errors.Is(err, ErrInvalidInvite) {
		t.Fatalf("expired code on a full group: err = %v, want ErrInvalidInvite", err)
	}
}

// #68: the limit holds when many people join at once, because the update filter checks it.
func TestConcurrentJoinsStopAtSizeLimit(t *testing.T) {
	ctx := useTestDB(t)
	owner := bson.NewObjectID()
	joiners := make([]bson.ObjectID, 8)
	for i := range joiners {
		joiners[i] = bson.NewObjectID()
	}
	insertTestUsers(t, ctx, append([]bson.ObjectID{owner}, joiners...)...)
	live := mintInvite(t, time.Now().Add(time.Hour))
	chatID := insertSizedGroup(t, ctx, owner, 1, 4, live) // 2 free places

	var wg sync.WaitGroup
	errs := make([]error, len(joiners))
	for i, j := range joiners {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = JoinGroupByInvite(ctx, j, live.InviteCode)
		}()
	}
	wg.Wait()

	joined := 0
	for i, err := range errs {
		switch {
		case err == nil:
			joined++
		case !errors.Is(err, ErrGroupFull):
			t.Errorf("joiner %d: err = %v, want nil or ErrGroupFull", i, err)
		}
	}
	if n := len(participantsOf(t, chatID)); joined != 2 || n != 4 {
		t.Fatalf("%d joins succeeded and the group has %d participants; want 2 and 4", joined, n)
	}
}

// #68: a group can't be created with more people than the limit.
func TestCreateGroupChatRespectsSizeLimit(t *testing.T) {
	ctx := useTestDB(t)
	creator := bson.NewObjectID()
	invitees := make([]bson.ObjectID, constants.DefaultMaxGroupParticipants)
	for i := range invitees {
		invitees[i] = bson.NewObjectID()
	}
	insertTestUsers(t, ctx, append([]bson.ObjectID{creator}, invitees...)...)
	creatorCtx := context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: creator, Username: creator.Hex()})

	if _, err := CreateGroupChat(creatorCtx, "too big", "", invitees); !errors.Is(err, ErrGroupFull) {
		t.Fatalf("create with %d invitees: err = %v, want ErrGroupFull", len(invitees), err)
	}
	chat, err := CreateGroupChat(creatorCtx, "just right", "", invitees[:constants.DefaultMaxGroupParticipants-1])
	if err != nil {
		t.Fatalf("create at the limit: %v", err)
	}
	if n := len(chat.Participants); n != constants.DefaultMaxGroupParticipants {
		t.Fatalf("participants = %d, want %d", n, constants.DefaultMaxGroupParticipants)
	}
}

// addMember puts id into the group's participants directly, as a concurrent write would.
func addMember(t *testing.T, chatID, id bson.ObjectID) {
	t.Helper()
	_, err := database.DB.Collection(string(constants.ChatColl)).UpdateOne(t.Context(),
		bson.M{"_id": chatID}, bson.M{"$set": bson.M{participantField(id): bson.M{"role": string(constants.ChatRoleMember)}}})
	if err != nil {
		t.Fatalf("add member: %v", err)
	}
}

// From the #68 code review: an add whose update matched nothing is "full" only when
// the group really has no room for the people being added.
func TestAddNotAppliedExplainsTheMiss(t *testing.T) {
	ctx := useTestDB(t)
	owner, u1, u2 := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()

	if err := addNotApplied(ctx, bson.NewObjectID(), []bson.ObjectID{u1}); !errors.Is(err, ErrChatNotFound) {
		t.Fatalf("chat deleted meanwhile: err = %v, want ErrChatNotFound", err)
	}

	// Another admin added u1 first, which filled the group: that isn't "full" for u1.
	raced := insertSizedGroup(t, ctx, owner, 1, 3)
	addMember(t, raced, u1)
	err := addNotApplied(ctx, raced, []bson.ObjectID{u1})
	if errors.Is(err, ErrGroupFull) || !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("target added meanwhile: err = %v, want ErrPermissionDenied (retryable), not ErrGroupFull", err)
	}

	full := insertSizedGroup(t, ctx, owner, 2, 3)
	if err := addNotApplied(ctx, full, []bson.ObjectID{u2}); !errors.Is(err, ErrGroupFull) {
		t.Fatalf("group filled meanwhile: err = %v, want ErrGroupFull", err)
	}
}

// From the #68 code review: a join that missed with a usable code is "full" only when
// the group has no room.
func TestMissedJoinChecksRoom(t *testing.T) {
	ctx := useTestDB(t)
	owner, joiner := bson.NewObjectID(), bson.NewObjectID()
	live := mintInvite(t, time.Now().Add(time.Hour))
	now := time.Now()

	roomy := insertSizedGroup(t, ctx, owner, 1, 4, live)
	err := resolveMissedJoin(ctx, roomy, joiner, live.InviteCode, now)
	if err == nil || errors.Is(err, ErrGroupFull) || errors.Is(err, ErrInvalidInvite) {
		t.Fatalf("missed join into a group with room: err = %v, want a retryable error", err)
	}

	full := insertSizedGroup(t, ctx, owner, 1, 2, live)
	if err := resolveMissedJoin(ctx, full, joiner, live.InviteCode, now); !errors.Is(err, ErrGroupFull) {
		t.Fatalf("missed join into a full group: err = %v, want ErrGroupFull", err)
	}
}
