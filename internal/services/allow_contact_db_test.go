package services

import (
	"context"
	"errors"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// #32: a contact request goes through only if the target's
// settings.privacy.allowContactBy lets the sender in. "contacts" means the
// sender shares a group with the target as a member, not a pending invitee
// and not just a direct chat (anyone can open one).
func TestSendContactRequestAppliesAllowContactBy(t *testing.T) {
	ctx := useTestDB(t)
	users := database.DB.Collection(string(constants.UserColl))
	chats := database.DB.Collection(string(constants.ChatColl))

	// shared puts sender and target in one chat of chatType, with these
	// participant request statuses.
	shared := func(t *testing.T, chatType string, sender, target bson.ObjectID, senderStatus, targetStatus string) {
		t.Helper()
		chatID := bson.NewObjectID()
		if _, err := chats.InsertOne(ctx, bson.M{"_id": chatID, "chatType": chatType, "participants": bson.M{
			sender.Hex(): bson.M{"requestStatus": senderStatus},
			target.Hex(): bson.M{"requestStatus": targetStatus},
		}}); err != nil {
			t.Fatalf("insert chat: %v", err)
		}
		for _, id := range []bson.ObjectID{sender, target} {
			if _, err := users.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"chats." + chatID.Hex(): 0}}); err != nil {
				t.Fatalf("chats entry: %v", err)
			}
		}
	}
	group, direct := string(constants.ChatTypeGroup), string(constants.ChatTypeDirect)
	accepted, pending := string(constants.StatusAccepted), string(constants.StatusPending)

	cases := []struct {
		name    string
		scope   any // nil: no settings at all
		setup   func(t *testing.T, sender, target bson.ObjectID)
		allowed bool
	}{
		{"everyone", "everyone", nil, true},
		{"never set", nil, nil, true},
		{"none", "none", nil, false},
		{"none, shares a group", "none", func(t *testing.T, s, tg bson.ObjectID) { shared(t, group, s, tg, accepted, accepted) }, false},
		{"contacts, shares nothing", "contacts", nil, false},
		{"contacts, shares a group", "contacts", func(t *testing.T, s, tg bson.ObjectID) { shared(t, group, s, tg, accepted, accepted) }, true},
		{"contacts, legacy member with no status", "contacts", func(t *testing.T, s, tg bson.ObjectID) { shared(t, group, s, tg, "", accepted) }, true},
		{"contacts, only invited", "contacts", func(t *testing.T, s, tg bson.ObjectID) { shared(t, group, s, tg, pending, accepted) }, false},
		{"contacts, only a direct chat", "contacts", func(t *testing.T, s, tg bson.ObjectID) { shared(t, direct, s, tg, "", "") }, false},
		{"unknown scope", "friends", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sender, target := bson.NewObjectID(), bson.NewObjectID()
			targetDoc := bson.M{"_id": target, "username": "target"}
			if tc.scope != nil {
				targetDoc["settings"] = bson.M{"privacy": bson.M{"allowContactBy": tc.scope}}
			}
			if _, err := users.InsertMany(ctx, []any{bson.M{"_id": sender, "username": "sender"}, targetDoc}); err != nil {
				t.Fatalf("insert users: %v", err)
			}
			if tc.setup != nil {
				tc.setup(t, sender, target)
			}

			err := SendContactRequest(ctx, sender, target)
			if tc.allowed && err != nil {
				t.Fatalf("request refused: %v", err)
			}
			if !tc.allowed && !errors.Is(err, ErrContactNotAllowed) {
				t.Fatalf("err = %v, want ErrContactNotAllowed", err)
			}
			if got := pendingInCount(t, ctx, target, sender); got != map[bool]int64{true: 1, false: 0}[tc.allowed] {
				t.Fatalf("target's pendingIn holds the sender %d times", got)
			}
		})
	}
}

func pendingInCount(t *testing.T, ctx context.Context, target, sender bson.ObjectID) int64 {
	t.Helper()
	n, err := database.DB.Collection(string(constants.UserColl)).CountDocuments(ctx,
		bson.M{"_id": target, "contactInfo.pendingIn": sender})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}
