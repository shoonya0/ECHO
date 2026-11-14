package services

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestJoinGroupByInviteRequiresStoredInvite(t *testing.T) {
	ctx := useTestDB(t)

	// newUser stores a user with the given chats list. Every subtest that
	// joins uses its own user, so each one can run alone.
	newUser := func(t *testing.T, chats bson.M) bson.ObjectID {
		t.Helper()
		id := bson.NewObjectID()
		doc := bson.M{
			"_id":      id,
			"username": id.Hex(),
			"profile":  bson.M{"displayName": "user_" + id.Hex()},
		}
		if chats != nil {
			doc["chats"] = chats
		}
		if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, doc); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		return id
	}

	owner := newUser(t, nil)
	groupID, directID := bson.NewObjectID(), bson.NewObjectID()
	// Both members are in participants. unlisted lost its chats entry, as
	// after a join that stopped halfway. listed has 5 unread messages.
	unlisted := newUser(t, nil)
	listed := newUser(t, bson.M{groupID.Hex(): 5})

	creator := models.LoginUserResponse{ID: owner}
	mint := func(t *testing.T, chatID bson.ObjectID) models.InviteCodeEmbed {
		t.Helper()
		invite, ok := utils.NewInviteCodeWithDefaults(creator, chatID, time.Now().Add(time.Hour))
		if !ok {
			t.Fatal("mint invite code")
		}
		return invite
	}
	live, deleted, expired := mint(t, groupID), mint(t, groupID), mint(t, groupID)
	deleted.Deleted = true
	// The code still says it is valid for an hour. Only the stored expiry has
	// passed, and that is the one that counts.
	expired.ExpiredDate = time.Now().Add(-time.Minute)
	onDirect := mint(t, directID)

	insertChat := func(t *testing.T, id bson.ObjectID, chatType constants.ChatType, members []bson.ObjectID, invites ...models.InviteCodeEmbed) {
		t.Helper()
		participants := bson.M{}
		for _, m := range members {
			participants[m.Hex()] = bson.M{"role": string(constants.ChatRoleMember)}
		}
		_, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
			"_id":          id,
			"chatType":     string(chatType),
			"participants": participants,
			"inviteCode":   invites,
		})
		if err != nil {
			t.Fatalf("insert chat: %v", err)
		}
	}
	insertChat(t, groupID, constants.ChatTypeGroup, []bson.ObjectID{owner, unlisted, listed}, live, deleted, expired)
	insertChat(t, directID, constants.ChatTypeDirect, []bson.ObjectID{owner}, onDirect)

	inParticipants := func(t *testing.T, chatID, userID bson.ObjectID) bool {
		t.Helper()
		n, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, chatMemberFilter(chatID, userID))
		if err != nil {
			t.Fatalf("count chat: %v", err)
		}
		return n > 0
	}
	// chatsEntry returns userID's unread count for chatID and whether the
	// chat is in their chats list at all (what the hub subscribes to).
	chatsEntry := func(t *testing.T, userID, chatID bson.ObjectID) (int, bool) {
		t.Helper()
		var u struct {
			Chats map[string]int `bson:"chats"`
		}
		if err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, bson.M{"_id": userID}).Decode(&u); err != nil {
			t.Fatalf("find user: %v", err)
		}
		unread, ok := u.Chats[chatID.Hex()]
		return unread, ok
	}

	rejected := []struct {
		name   string
		chatID bson.ObjectID
		code   string
	}{
		{"code that isn't stored", groupID, mint(t, groupID).InviteCode},
		{"deleted code", groupID, deleted.InviteCode},
		{"stored code past its stored expiry", groupID, expired.InviteCode},
		{"code stored on a direct chat", directID, onDirect.InviteCode},
		{"garbage code", groupID, "not-a-hex-code"},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			joiner := newUser(t, nil)
			err := JoinGroupByInvite(ctx, joiner, tc.code)
			if !errors.Is(err, ErrInvalidInvite) {
				t.Fatalf("err = %v, want ErrInvalidInvite", err)
			}
			_, listedChat := chatsEntry(t, joiner, tc.chatID)
			if p := inParticipants(t, tc.chatID, joiner); p || listedChat {
				t.Fatalf("joiner got in: inParticipants=%v inChats=%v", p, listedChat)
			}
		})
	}

	t.Run("stored live code joins", func(t *testing.T) {
		joiner := newUser(t, nil)
		if err := JoinGroupByInvite(ctx, joiner, live.InviteCode); err != nil {
			t.Fatalf("JoinGroupByInvite: %v", err)
		}
		_, listedChat := chatsEntry(t, joiner, groupID)
		if p := inParticipants(t, groupID, joiner); !p || !listedChat {
			t.Fatalf("joiner not added: inParticipants=%v inChats=%v", p, listedChat)
		}

		// The join is recorded on the invite that was used, not on the other
		// invites stored next to it.
		var chat struct {
			InviteCode []models.InviteCodeEmbed `bson:"inviteCode"`
		}
		if err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, bson.M{"_id": groupID}).Decode(&chat); err != nil {
			t.Fatalf("find chat: %v", err)
		}
		for _, inv := range chat.InviteCode {
			want := inv.InviteCode == live.InviteCode
			if got := slices.Contains(inv.UserIDs, joiner); got != want {
				t.Fatalf("invite deleted=%v expires=%v has joiner = %v, want %v", inv.Deleted, inv.ExpiredDate, got, want)
			}
		}
	})

	t.Run("joining twice is a conflict", func(t *testing.T) {
		joiner := newUser(t, nil)
		if err := JoinGroupByInvite(ctx, joiner, live.InviteCode); err != nil {
			t.Fatalf("first join: %v", err)
		}
		err := JoinGroupByInvite(ctx, joiner, live.InviteCode)
		if !errors.Is(err, ErrAlreadyParticipant) {
			t.Fatalf("err = %v, want ErrAlreadyParticipant", err)
		}
	})

	t.Run("member missing the chats entry gets it back", func(t *testing.T) {
		err := JoinGroupByInvite(ctx, unlisted, live.InviteCode)
		if !errors.Is(err, ErrAlreadyParticipant) {
			t.Fatalf("err = %v, want ErrAlreadyParticipant", err)
		}
		if unread, ok := chatsEntry(t, unlisted, groupID); !ok || unread != 0 {
			t.Fatalf("chats entry = %d, %v; want 0, true", unread, ok)
		}
	})

	t.Run("member keeps their unread count", func(t *testing.T) {
		err := JoinGroupByInvite(ctx, listed, live.InviteCode)
		if !errors.Is(err, ErrAlreadyParticipant) {
			t.Fatalf("err = %v, want ErrAlreadyParticipant", err)
		}
		if unread, ok := chatsEntry(t, listed, groupID); !ok || unread != 5 {
			t.Fatalf("chats entry = %d, %v; want 5, true", unread, ok)
		}
	})
}
