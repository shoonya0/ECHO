package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/testutil"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// markFor stores a server's mark for userID expiring at expires, as the hub
// does, and removes the key when the test ends.
func markFor(t *testing.T, client *redis.Client, userID bson.ObjectID, server string, expires time.Time) {
	t.Helper()
	ctx := context.Background()
	key := constants.OnlineKeyPrefix + userID.Hex()
	if err := client.ZAdd(ctx, key, redis.Z{Score: float64(expires.UnixMilli()), Member: server}).Err(); err != nil {
		t.Fatalf("mark %s: %v", userID.Hex(), err)
	}
	t.Cleanup(func() { _ = client.Del(context.Background(), key).Err() })
}

// #31: a user is connected while any server's mark on online:<id> hasn't
// expired. An expired mark, left by a server that stopped, doesn't count.
// Needs Redis: run with TEST_DB_REQUIRED=1.
func TestConnectedUsers(t *testing.T) {
	client := testutil.UseTestRedis(t)
	live, expired, mixed, none := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	markFor(t, client, live, "one", time.Now().Add(time.Minute))
	markFor(t, client, expired, "one", time.Now().Add(-time.Second))
	markFor(t, client, mixed, "one", time.Now().Add(-time.Second))
	markFor(t, client, mixed, "two", time.Now().Add(time.Minute))

	got, err := ConnectedUsers(context.Background(), []bson.ObjectID{live, expired, mixed, none})
	if err != nil {
		t.Fatalf("ConnectedUsers: %v", err)
	}
	want := map[bson.ObjectID]bool{live: true, mixed: true}
	if len(got) != len(want) || !got[live] || !got[mixed] {
		t.Fatalf("connected = %v, want only %s and %s", got, live.Hex(), mixed.Hex())
	}
}

// With Redis down, REST reads show everyone offline instead of failing.
func TestConnectedUsersRedisDown(t *testing.T) {
	testutil.UseBrokenRedis(t)
	id := bson.NewObjectID()
	if _, err := ConnectedUsers(context.Background(), []bson.ObjectID{id}); err == nil {
		t.Fatal("ConnectedUsers with Redis down returned no error")
	}
	if got := connectedOrNone(context.Background(), []bson.ObjectID{id}); got[id] {
		t.Fatal("user shown connected with Redis down")
	}
}

// #31: the hub saves lastSeen on every offline. Two disconnects saved out of
// order keep the later time.
func TestRecordLastSeenNeverMovesBack(t *testing.T) {
	ctx := useTestDB(t)
	users := database.DB.Collection(string(constants.UserColl))
	id := bson.NewObjectID()
	if _, err := users.InsertOne(ctx, bson.M{"_id": id, "username": "seen", "presence": bson.M{"status": "online"}}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	later := time.Now().UTC().Truncate(time.Millisecond)
	earlier := later.Add(-time.Minute)
	for _, at := range []time.Time{later, earlier} {
		if err := RecordLastSeen(ctx, id, at); err != nil {
			t.Fatalf("RecordLastSeen(%v): %v", at, err)
		}
	}
	var doc struct {
		Presence struct {
			LastSeen time.Time `bson:"lastSeen"`
			Status   string    `bson:"status"`
		} `bson:"presence"`
	}
	if err := users.FindOne(ctx, bson.M{"_id": id}).Decode(&doc); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !doc.Presence.LastSeen.Equal(later) {
		t.Fatalf("lastSeen = %v, want %v", doc.Presence.LastSeen, later)
	}
	if doc.Presence.Status != "online" {
		t.Fatalf("status changed to %q", doc.Presence.Status)
	}
	// A deleted user matches nothing, which isn't an error.
	if err := RecordLastSeen(ctx, bson.NewObjectID(), later); err != nil {
		t.Fatalf("RecordLastSeen for a missing user: %v", err)
	}
}

// #31: the status the hub announces comes from MongoDB, not the token.
func TestGetPresenceStatus(t *testing.T) {
	ctx := useTestDB(t)
	users := database.DB.Collection(string(constants.UserColl))
	away, unset := bson.NewObjectID(), bson.NewObjectID()
	if _, err := users.InsertMany(ctx, []any{
		bson.M{"_id": away, "presence": bson.M{"status": "away"}},
		bson.M{"_id": unset, "username": "nopresence"},
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if got, err := GetPresenceStatus(ctx, away); err != nil || got != constants.UserStatusAway {
		t.Fatalf("away user: %q, %v", got, err)
	}
	if got, err := GetPresenceStatus(ctx, unset); err != nil || got != constants.UserStatusOnline {
		t.Fatalf("user without presence: %q, %v; want online", got, err)
	}
	if _, err := GetPresenceStatus(ctx, bson.NewObjectID()); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing user: err = %v, want ErrUserNotFound", err)
	}
}

// #31: the contacts list shows who is connected now. Before, it showed the
// stored presence, which said online forever.
func TestGetContactsShowsLivePresence(t *testing.T) {
	ctx := useTestDB(t)
	redisClient := testutil.UseTestRedis(t)
	users := database.DB.Collection(string(constants.UserColl))
	seen := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)

	owner := bson.NewObjectID()
	ids := map[string]bson.ObjectID{}
	statuses := map[string]string{"connected": "dnd", "gone": "online", "hidden": "invisible"}
	docs := []any{}
	contacts := bson.A{}
	for name, status := range statuses {
		ids[name] = bson.NewObjectID()
		contacts = append(contacts, ids[name])
		// isOnline stored true for everyone, as the old default did.
		docs = append(docs, bson.M{"_id": ids[name], "username": name, "presence": bson.M{
			"status": status, "isOnline": true, "lastSeen": seen,
		}})
	}
	docs = append(docs, bson.M{"_id": owner, "username": "owner", "contactInfo": bson.M{"contacts": contacts}})
	if _, err := users.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert: %v", err)
	}
	markFor(t, redisClient, ids["connected"], "one", time.Now().Add(time.Minute))
	markFor(t, redisClient, ids["hidden"], "one", time.Now().Add(time.Minute))

	list, _, err := GetContacts(ctx, owner, constants.StatusContact, 1, 10)
	if err != nil {
		t.Fatalf("GetContacts: %v", err)
	}
	want := map[string]struct {
		status   string
		online   bool
		lastSeen bool
	}{
		"connected": {"dnd", true, true},
		"gone":      {"offline", false, true},
		"hidden":    {"offline", false, false},
	}
	if len(list) != len(want) {
		t.Fatalf("got %d contacts, want %d", len(list), len(want))
	}
	for _, c := range list {
		w := want[c.Username]
		if c.Presence.Status != w.status || c.Presence.IsOnline != w.online || c.Presence.LastSeen.IsZero() == w.lastSeen {
			t.Errorf("%s: status %q isOnline %v lastSeen %v; want %q %v shown=%v",
				c.Username, c.Presence.Status, c.Presence.IsOnline, c.Presence.LastSeen, w.status, w.online, w.lastSeen)
		}
	}
}
