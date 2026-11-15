package services

import (
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A reaction's count must always equal its number of users. Before #25, add always
// added 1 and remove always took 1 away, so repeating an add inflated the count, and
// a remove by someone who never reacted could bring it to 0, which deleted everyone
// else's reaction.
func TestReactionCountFollowsUsers(t *testing.T) {
	ctx := useTestDB(t)

	alice, bob, carol := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	chatID := bson.NewObjectID()
	participants := bson.M{}
	for _, id := range []bson.ObjectID{alice, bob, carol} {
		if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{"_id": id, "username": "user-" + id.Hex()}); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		participants[id.Hex()] = bson.M{"status": "accepted"}
	}
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{"_id": chatID, "participants": participants}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	type reaction struct {
		Count   int               `bson:"count"`
		Users   []bson.ObjectID   `bson:"users"`
		Details map[string]string `bson:"details"`
	}
	newMessage := func(t *testing.T, reactions bson.M) bson.ObjectID {
		t.Helper()
		id := bson.NewObjectID()
		if _, err := database.DB.Collection(string(constants.MessageColl)).InsertOne(ctx, bson.M{
			"_id": id, "chatId": chatID, "senderId": alice, "content": "hi", "reactions": reactions,
		}); err != nil {
			t.Fatalf("insert message: %v", err)
		}
		return id
	}
	thumbs := func(t *testing.T, messageID bson.ObjectID) (reaction, bool) {
		t.Helper()
		var doc struct {
			Reactions map[string]reaction `bson:"reactions"`
		}
		if err := database.DB.Collection(string(constants.MessageColl)).FindOne(ctx, bson.M{"_id": messageID}).Decode(&doc); err != nil {
			t.Fatalf("find message: %v", err)
		}
		r, ok := doc.Reactions["👍"]
		return r, ok
	}
	want := func(t *testing.T, messageID bson.ObjectID, users ...bson.ObjectID) {
		t.Helper()
		got, ok := thumbs(t, messageID)
		if !ok {
			t.Fatalf("👍 is gone, want users %v", users)
		}
		if got.Count != len(users) || len(got.Users) != len(users) || len(got.Details) != len(users) {
			t.Fatalf("👍 = %+v, want count %d with users %v", got, len(users), users)
		}
		for i, u := range users {
			if got.Users[i] != u {
				t.Fatalf("👍 users = %v, want %v", got.Users, users)
			}
			if _, ok := got.Details[u.Hex()]; !ok {
				t.Fatalf("👍 details = %v, want an entry for %s", got.Details, u.Hex())
			}
		}
	}
	add := func(t *testing.T, messageID, userID bson.ObjectID, wantChanged bool) {
		t.Helper()
		changed, err := AddMessageReaction(chatID, messageID, userID, "👍")
		if err != nil {
			t.Fatalf("add: %v", err)
		}
		if changed != wantChanged {
			t.Fatalf("add reported changed = %v, want %v", changed, wantChanged)
		}
	}
	remove := func(t *testing.T, messageID, userID bson.ObjectID, wantChanged bool) {
		t.Helper()
		changed, err := RemoveMessageReaction(chatID, messageID, userID, "👍")
		if err != nil {
			t.Fatalf("remove: %v", err)
		}
		if changed != wantChanged {
			t.Fatalf("remove reported changed = %v, want %v", changed, wantChanged)
		}
	}

	t.Run("adding again doesn't count twice", func(t *testing.T) {
		m := newMessage(t, bson.M{})
		add(t, m, alice, true)
		add(t, m, alice, false)
		add(t, m, alice, false)
		want(t, m, alice)
	})

	t.Run("a remove by someone who never reacted keeps the others", func(t *testing.T) {
		m := newMessage(t, bson.M{})
		add(t, m, alice, true)
		add(t, m, bob, true)
		remove(t, m, carol, false)
		remove(t, m, carol, false)
		want(t, m, alice, bob)
	})

	t.Run("removing twice takes one reaction away", func(t *testing.T) {
		m := newMessage(t, bson.M{})
		add(t, m, alice, true)
		add(t, m, bob, true)
		remove(t, m, bob, true)
		remove(t, m, bob, false)
		want(t, m, alice)
	})

	t.Run("the last remove deletes the emoji", func(t *testing.T) {
		m := newMessage(t, bson.M{})
		add(t, m, alice, true)
		remove(t, m, alice, true)
		if got, ok := thumbs(t, m); ok {
			t.Fatalf("👍 = %+v, want it gone", got)
		}
	})

	t.Run("a count that already drifted is corrected on the next change", func(t *testing.T) {
		m := newMessage(t, bson.M{"👍": bson.M{
			"count": 5, "users": bson.A{alice}, "details": bson.M{alice.Hex(): ""},
		}})
		add(t, m, bob, true)
		want(t, m, alice, bob)
		remove(t, m, bob, true)
		want(t, m, alice)
	})

	t.Run("a drifted count at 0 still has a user to keep", func(t *testing.T) {
		m := newMessage(t, bson.M{"👍": bson.M{
			"count": 0, "users": bson.A{alice, bob}, "details": bson.M{alice.Hex(): "", bob.Hex(): ""},
		}})
		remove(t, m, bob, true)
		want(t, m, alice)
	})
}
