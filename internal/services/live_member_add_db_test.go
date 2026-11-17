package services

import (
	"context"
	"slices"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// recordingAdder is a Broadcaster that also records the users each chat's open
// connections were told to follow.
type recordingAdder struct {
	recordingBroadcaster
	added map[string][]string
}

func (a *recordingAdder) AddMembersToChat(chatID string, userIDs []string) error {
	a.added[chatID] = append(a.added[chatID], userIDs...)
	return nil
}

// #49: a user's open connections follow every chat they gain: a new direct chat
// (both people), a new group (its creator) and a group joined with a code.
func TestChatsGainedAreFollowedLive(t *testing.T) {
	ctx := useTestDB(t)
	rec := &recordingAdder{added: map[string][]string{}}
	SetBroadcaster(rec)
	t.Cleanup(func() { SetBroadcaster(nil) })

	alice, bob, carol := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, alice, bob, carol)
	as := func(id bson.ObjectID) context.Context {
		return context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: id, Username: id.Hex()})
	}
	assertAdded := func(t *testing.T, chatID bson.ObjectID, want ...bson.ObjectID) {
		t.Helper()
		got := rec.added[chatID.Hex()]
		wantHex := make([]string, len(want))
		for i, id := range want {
			wantHex[i] = id.Hex()
		}
		slices.Sort(got)
		slices.Sort(wantHex)
		if !slices.Equal(got, wantHex) {
			t.Errorf("chat %s: connections of %v were told to follow it, want %v", chatID.Hex(), got, wantHex)
		}
	}

	t.Run("a new direct chat, for both people, and an existing one not again", func(t *testing.T) {
		chat, err := CreateDirectChat(as(alice), bob)
		if err != nil {
			t.Fatalf("CreateDirectChat: %v", err)
		}
		assertAdded(t, chat.ChatID, alice, bob)

		again, err := CreateDirectChat(as(bob), alice)
		if err != nil {
			t.Fatalf("CreateDirectChat again: %v", err)
		}
		if again.ChatID != chat.ChatID {
			t.Fatalf("got a second direct chat %s", again.ChatID.Hex())
		}
		assertAdded(t, chat.ChatID, alice, bob)
	})

	t.Run("a new group, for its creator only; a code join, for the joiner", func(t *testing.T) {
		group, err := CreateGroupChat(as(alice), "g", "", []bson.ObjectID{bob})
		if err != nil {
			t.Fatalf("CreateGroupChat: %v", err)
		}
		assertAdded(t, group.ChatID, alice) // bob is only invited (pending)

		if err := JoinGroupByInvite(ctx, carol, group.InviteCode[0].InviteCode); err != nil {
			t.Fatalf("JoinGroupByInvite: %v", err)
		}
		assertAdded(t, group.ChatID, alice, carol)

		// bob accepts his invitation by joining with the code (#103).
		if err := JoinGroupByInvite(ctx, bob, group.InviteCode[0].InviteCode); err != nil {
			t.Fatalf("JoinGroupByInvite (pending member): %v", err)
		}
		assertAdded(t, group.ChatID, alice, carol, bob)
	})
}
