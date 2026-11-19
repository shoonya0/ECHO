package database_test

import (
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// EnsureIndexes builds the chats index the invite routes look codes up by
// (#15), and running it twice is a no-op.
func TestEnsureIndexesBuildsTheInviteCodeIndex(t *testing.T) {
	ctx := testutil.UseTestDB(t)

	for i := 0; i < 2; i++ {
		if err := database.EnsureIndexes(ctx); err != nil {
			t.Fatalf("EnsureIndexes run %d: %v", i+1, err)
		}
	}

	cursor, err := database.DB.Collection(string(constants.ChatColl)).Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	var indexes []bson.M
	if err := cursor.All(ctx, &indexes); err != nil {
		t.Fatalf("read indexes: %v", err)
	}
	for _, index := range indexes {
		if index["name"] == "inviteCode.inviteCode_1" {
			return
		}
	}
	t.Fatalf("no inviteCode.inviteCode_1 index on chats; have %v", indexes)
}

// The case-insensitive email index (#36) is built even when emails stored
// before #36 differ only in case, which makes the unique email_1 fail.
func TestEnsureIndexesBuildsEmailCIDespiteCaseDuplicates(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	users := database.DB.Collection(string(constants.UserColl))
	if _, err := users.InsertMany(ctx, []any{
		bson.M{"email": "dup@example.com"}, bson.M{"email": "dup@example.com"},
		bson.M{"email": "Dup@Example.com"},
	}); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if err := database.EnsureIndexes(ctx); err == nil {
		t.Fatalf("EnsureIndexes: want the email_1 error for the exact duplicates")
	}

	specs, err := users.Indexes().ListSpecifications(ctx)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	for _, s := range specs {
		if s.Name == "email_ci" {
			return
		}
	}
	t.Fatalf("email_ci missing; indexes: %v", specs)
}

// The unique directKey index (#37) is built over direct chats from before #37,
// which have no key, even when a pair has two of them; the backfill keys them
// later. With it, a second chat with the same key is refused.
func TestEnsureIndexesBuildsDirectKeyOverUnkeyedChats(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	chats := database.DB.Collection(string(constants.ChatColl))
	pair := bson.M{"a": bson.M{}, "b": bson.M{}}
	for range 2 {
		if _, err := chats.InsertOne(ctx, bson.M{"chatType": "direct", "participants": pair}); err != nil {
			t.Fatalf("insert chat: %v", err)
		}
	}

	if err := database.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}

	if _, err := chats.InsertOne(ctx, bson.M{"chatType": "direct", "directKey": "a_b"}); err != nil {
		t.Fatalf("insert keyed chat: %v", err)
	}
	if _, err := chats.InsertOne(ctx, bson.M{"chatType": "direct", "directKey": "a_b"}); err == nil {
		t.Fatal("a second chat with the same directKey was stored")
	}
}

// The username index (#36) ignores case and skips documents without a
// username; a second account named like the first in another case is refused,
// and the error names the index so signup can answer 409.
func TestEnsureIndexesUsernameIsUniqueIgnoringCase(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	users := database.DB.Collection(string(constants.UserColl))
	if _, err := users.InsertMany(ctx, []any{
		bson.M{"email": "a@example.com", "username": "alice"},
		bson.M{"email": "b@example.com"}, bson.M{"email": "c@example.com"},
	}); err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if err := database.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}

	_, err := users.InsertOne(ctx, bson.M{"email": "d@example.com", "username": "ALICE"})
	if !database.IsDuplicateKeyOn(err, database.UsernameIndex) {
		t.Fatalf("insert ALICE: err %v, want a duplicate key on %s", err, database.UsernameIndex)
	}
	_, err = users.InsertOne(ctx, bson.M{"email": "a@example.com", "username": "bob"})
	if err == nil || database.IsDuplicateKeyOn(err, database.UsernameIndex) {
		t.Fatalf("insert a taken email: err %v, want a duplicate key on email_1 only", err)
	}
}
