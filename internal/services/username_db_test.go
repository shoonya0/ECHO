package services

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// insertNamed stores a user with the given username, IDs in insertion order.
func insertNamed(t *testing.T, ctx context.Context, username string) bson.ObjectID {
	t.Helper()
	id := bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx,
		bson.M{"_id": id, "email": id.Hex() + "@example.com", "username": username}); err != nil {
		t.Fatalf("insert user %q: %v", username, err)
	}
	return id
}

func storedUsername(t *testing.T, ctx context.Context, id bson.ObjectID) string {
	t.Helper()
	var u struct {
		Username string `bson:"username"`
	}
	if err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, bson.M{"_id": id}).Decode(&u); err != nil {
		t.Fatalf("read user %s: %v", id.Hex(), err)
	}
	return u.Username
}

// Changing the username (#36): the new name needs the format and must be free in
// any case; one's own name in other capitals is allowed; the stored name sent back
// unchanged passes even when it is from before #36 and lacks the format.
func TestUpdateProfileUsername(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	if err := database.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}
	insertNamed(t, ctx, "alice")
	bob := insertNamed(t, ctx, "bob")
	legacy := insertNamed(t, ctx, "old name")

	tests := []struct {
		name    string
		user    bson.ObjectID
		sent    any
		wantErr error
		want    string
	}{
		{"taken in other case", bob, "ALICE", ErrUsernameTaken, "bob"},
		{"bad format", bob, "b", ErrInvalidUsername, "bob"},
		{"not a string", bob, 42, ErrInvalidUsername, "bob"},
		{"own name, other case, trimmed", bob, "  Bob ", nil, "Bob"},
		{"free name", bob, "bobby", nil, "bobby"},
		{"legacy name sent back", legacy, "old name", nil, "old name"},
		{"legacy name changed must have the format", legacy, "old name 2", ErrInvalidUsername, "old name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UpdateProfile(ctx, tt.user, map[string]interface{}{"username": tt.sent})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("UpdateProfile: err %v, want %v", err, tt.wantErr)
			}
			if got := storedUsername(t, ctx, tt.user); got != tt.want {
				t.Fatalf("username = %q, want %q", got, tt.want)
			}
		})
	}
}

// Accounts sharing a name before #36 (in any case) are renamed so the unique
// index can be built: the oldest keeps it, the others get the first free
// <name>_<n>. A second run renames nothing, and the index then builds.
func TestRenameDuplicateUsernames(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	first := insertNamed(t, ctx, "alice")
	second := insertNamed(t, ctx, "Alice")
	insertNamed(t, ctx, "alice_2") // so the second one can't take _2
	third := insertNamed(t, ctx, "ALICE")
	solo := insertNamed(t, ctx, "bob")

	renames, err := RenameDuplicateUsernames(ctx)
	if err != nil {
		t.Fatalf("RenameDuplicateUsernames: %v", err)
	}
	if len(renames) != 2 {
		t.Fatalf("renames %v, want 2", renames)
	}
	for id, want := range map[bson.ObjectID]string{
		first:  "alice",
		second: "Alice_3",
		third:  "ALICE_4",
		solo:   "bob",
	} {
		if got := storedUsername(t, ctx, id); got != want {
			t.Errorf("user %s: username %q, want %q", id.Hex(), got, want)
		}
	}

	if renames, err := RenameDuplicateUsernames(ctx); err != nil || len(renames) != 0 {
		t.Fatalf("second run: renames %v, err %v; want none", renames, err)
	}
	if err := database.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes after the renames: %v", err)
	}
}
