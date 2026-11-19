package services

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
)

func ensureIndexes(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := database.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}
}

func countDirectChats(t *testing.T, ctx context.Context, a, b bson.ObjectID) int64 {
	t.Helper()
	n, err := database.DB.Collection(string(constants.ChatColl)).CountDocuments(ctx, bson.M{
		"chatType":                "direct",
		"participants." + a.Hex(): bson.M{"$exists": true},
		"participants." + b.Hex(): bson.M{"$exists": true},
	})
	if err != nil {
		t.Fatalf("count chats: %v", err)
	}
	return n
}

// Two people opening their chat at the same time (or a double tap) both found
// no chat and inserted one each: two direct chats for one pair (#37). The pair's
// directKey is unique, so every call now returns the same chat.
func TestConcurrentDirectChatCreatesOne(t *testing.T) {
	ctx := useTestDB(t)
	ensureIndexes(t, ctx)
	// With emails: the unique email index allows only one user without.
	alice := insertAccount(t, ctx, bson.M{"email": "alice@example.com"})
	bob := insertAccount(t, ctx, bson.M{"email": "bob@example.com"})
	as := func(id bson.ObjectID) context.Context {
		return context.WithValue(ctx, constants.UserDataKey, models.LoginUserResponse{ID: id, Username: id.Hex()})
	}

	const calls = 10
	ids := make(chan bson.ObjectID, calls)
	errs := make(chan error, calls)
	var n atomic.Int32
	concurrently(calls, func() {
		caller, target := alice, bob
		if n.Add(1)%2 == 0 { // both sides at once
			caller, target = bob, alice
		}
		chat, err := CreateDirectChat(as(caller), target)
		if err != nil {
			errs <- err
			return
		}
		ids <- chat.ChatID
	})
	close(ids)
	close(errs)

	for err := range errs {
		t.Errorf("CreateDirectChat: %v", err)
	}
	seen := map[bson.ObjectID]bool{}
	for id := range ids {
		seen[id] = true
	}
	if len(seen) != 1 {
		t.Errorf("calls returned %d different chats, want 1", len(seen))
	}
	if n := countDirectChats(t, ctx, alice, bob); n != 1 {
		t.Fatalf("%d direct chats for one pair, want 1", n)
	}
}

// The key is the same whichever side opens the chat, and clients never see it.
func TestDirectKeyIsOrderFreeAndHidden(t *testing.T) {
	a, b := bson.NewObjectID(), bson.NewObjectID()
	if directChatKey(a, b) != directChatKey(b, a) {
		t.Fatal("directChatKey depends on the order")
	}
	raw, err := json.Marshal(models.Chat{DirectKey: directChatKey(a, b)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "directKey") {
		t.Fatalf("chat JSON shows directKey: %s", raw)
	}
}

// Direct chats from before #37 have no key. The startup backfill gives each pair's
// oldest chat the key and reports later duplicates, which a person merges.
// Idempotent.
func TestBackfillDirectKeys(t *testing.T) {
	ctx := useTestDB(t)
	chats := database.DB.Collection(string(constants.ChatColl))
	a, b, c, d, e, f := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	pair := func(x, y bson.ObjectID) bson.M { return bson.M{x.Hex(): bson.M{}, y.Hex(): bson.M{}} }

	older, newer, single, broken, group := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	// Two chats keyed alike: made while directKey_1 was missing.
	keyedFirst, keyedSecond := bson.NewObjectID(), bson.NewObjectID()
	for _, doc := range []bson.M{
		{"_id": older, "chatType": "direct", "participants": pair(a, b)},
		{"_id": newer, "chatType": "direct", "participants": pair(b, a)},
		{"_id": single, "chatType": "direct", "participants": pair(c, d)},
		{"_id": broken, "chatType": "direct", "participants": bson.M{a.Hex(): bson.M{}}},
		{"_id": group, "chatType": "group", "participants": pair(c, d)},
		{"_id": keyedFirst, "chatType": "direct", "participants": pair(e, f), "directKey": directChatKey(e, f)},
		{"_id": keyedSecond, "chatType": "direct", "participants": pair(e, f), "directKey": directChatKey(e, f)},
	} {
		if _, err := chats.InsertOne(ctx, doc); err != nil {
			t.Fatalf("insert chat: %v", err)
		}
	}
	keyOf := func(id bson.ObjectID) string {
		var doc struct {
			DirectKey string `bson:"directKey"`
		}
		if err := chats.FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
			t.Fatalf("load chat: %v", err)
		}
		return doc.DirectKey
	}

	for run := range 2 {
		n, duplicates, err := BackfillDirectKeys(ctx)
		if err != nil {
			t.Fatalf("run %d: BackfillDirectKeys: %v", run, err)
		}
		if want := int64(2 * (1 - run)); n != want {
			t.Errorf("run %d: keyed %d chats, want %d", run, n, want)
		}
		got := map[bson.ObjectID]bool{}
		for _, id := range duplicates {
			got[id] = true
		}
		if len(duplicates) != 2 || !got[newer] || !got[keyedSecond] {
			t.Errorf("run %d: duplicates %v, want %s and %s", run, duplicates, newer.Hex(), keyedSecond.Hex())
		}
	}
	if keyOf(older) != directChatKey(a, b) || keyOf(single) != directChatKey(c, d) || keyOf(keyedFirst) != directChatKey(e, f) {
		t.Error("a pair's oldest chat doesn't have its key")
	}
	if keyOf(newer) != "" || keyOf(keyedSecond) != "" || keyOf(broken) != "" || keyOf(group) != "" {
		t.Error("a duplicate, a chat without two participants or a group has a key")
	}
	// With the clash gone, the unique index builds.
	if err := database.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes after the backfill: %v", err)
	}
}

// A pair with a keyed chat and a later unkeyed duplicate keeps using the keyed
// one, and with only unkeyed duplicates, the oldest.
func TestDirectChatLookupPrefersKeyedThenOldest(t *testing.T) {
	ctx := useTestDB(t)
	ensureIndexes(t, ctx)
	chats := database.DB.Collection(string(constants.ChatColl))
	alice := insertAccount(t, ctx, bson.M{"email": "alice@example.com"})
	bob := insertAccount(t, ctx, bson.M{"email": "bob@example.com"})
	carol := insertAccount(t, ctx, bson.M{"email": "carol@example.com"})
	pair := func(x, y bson.ObjectID) bson.M { return bson.M{x.Hex(): bson.M{}, y.Hex(): bson.M{}} }
	as := models.LoginUserResponse{ID: alice, Username: "alice"}

	first, keyed, second := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	for _, doc := range []bson.M{
		{"_id": first, "chatType": "direct", "participants": pair(alice, carol)},
		{"_id": keyed, "chatType": "direct", "participants": pair(alice, bob), "directKey": directChatKey(alice, bob)},
		{"_id": second, "chatType": "direct", "participants": pair(alice, carol)},
		{"_id": bson.NewObjectID(), "chatType": "direct", "participants": pair(bob, alice)}, // unkeyed, newer than keyed
	} {
		if _, err := chats.InsertOne(ctx, doc); err != nil {
			t.Fatalf("insert chat: %v", err)
		}
	}

	got, err := findOrCreateDirectChat(ctx, as, bob)
	if err != nil || got.created || got.chat.ChatID != keyed {
		t.Fatalf("alice-bob: got %+v, %v; want the keyed chat %s", got.chat, err, keyed.Hex())
	}
	got, err = findOrCreateDirectChat(ctx, as, carol)
	if err != nil || got.created || got.chat.ChatID != first {
		t.Fatalf("alice-carol: got %+v, %v; want the oldest chat %s", got.chat, err, first.Hex())
	}
}
