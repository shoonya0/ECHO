package services

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// storedBlocks is the raw block state of one user document. HasLegacy reports whether
// the pre-#26 blockedChats field is still there.
type storedBlocks struct {
	Blocked   []bson.ObjectID `bson:"blocked"`
	BlockedBy []bson.ObjectID `bson:"blockedBy"`
	HasLegacy bool            `bson:"-"`
}

func loadBlocks(t *testing.T, ctx context.Context, userID bson.ObjectID) storedBlocks {
	t.Helper()
	var doc struct {
		ContactInfo bson.Raw `bson:"contactInfo"`
	}
	if err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, bson.M{"_id": userID}).Decode(&doc); err != nil {
		t.Fatalf("load user: %v", err)
	}
	var b storedBlocks
	if doc.ContactInfo == nil {
		return b
	}
	if err := bson.Unmarshal(doc.ContactInfo, &b); err != nil {
		t.Fatalf("decode contactInfo: %v", err)
	}
	_, err := doc.ContactInfo.LookupErr("blockedChats")
	b.HasLegacy = err == nil
	return b
}

// #26: a block belongs to the person who made it. The blocked person can't see it
// and can't lift it.
func TestBlockIsOneSided(t *testing.T) {
	ctx := useTestDB(t)
	a, b := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, a, b)

	if err := BlockUnblockUser(ctx, a, b, string(constants.StatusBlocked)); err != nil {
		t.Fatalf("A blocks B: %v", err)
	}
	if got := loadBlocks(t, ctx, a); !slices.Equal(got.Blocked, []bson.ObjectID{b}) || len(got.BlockedBy) != 0 {
		t.Fatalf("A after block: blocked=%v blockedBy=%v, want blocked=[B] blockedBy=[]", got.Blocked, got.BlockedBy)
	}
	if got := loadBlocks(t, ctx, b); len(got.Blocked) != 0 || !slices.Equal(got.BlockedBy, []bson.ObjectID{a}) {
		t.Fatalf("B after block: blocked=%v blockedBy=%v, want blocked=[] blockedBy=[A]", got.Blocked, got.BlockedBy)
	}

	// B's "blocked" list must not reveal that A blocked them.
	list, err := GetContacts(ctx, b, constants.StatusBlocked, 10)
	if err != nil {
		t.Fatalf("GetContacts(B, blocked): %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("B's blocked list = %d entries, want 0", len(list))
	}

	// B can't lift A's block.
	if err := BlockUnblockUser(ctx, b, a, string(constants.StatusUnblocked)); !errors.Is(err, ErrAlreadyUnblocked) {
		t.Fatalf("B unblocks A: err = %v, want ErrAlreadyUnblocked", err)
	}
	if got := loadBlocks(t, ctx, a); !slices.Equal(got.Blocked, []bson.ObjectID{b}) {
		t.Fatalf("A's block after B's unblock attempt = %v, want [B]", got.Blocked)
	}

	// The block still stops contact requests in both directions.
	if err := SendContactRequest(ctx, b, a); !errors.Is(err, ErrBlocked) {
		t.Fatalf("B requests A: err = %v, want ErrBlocked", err)
	}
	if err := SendContactRequest(ctx, a, b); !errors.Is(err, ErrBlocked) {
		t.Fatalf("A requests B: err = %v, want ErrBlocked", err)
	}

	// Blocking twice is reported and doesn't add a duplicate.
	if err := BlockUnblockUser(ctx, a, b, string(constants.StatusBlocked)); !errors.Is(err, ErrAlreadyBlocked) {
		t.Fatalf("A blocks B again: err = %v, want ErrAlreadyBlocked", err)
	}
	if got := loadBlocks(t, ctx, b); len(got.BlockedBy) != 1 {
		t.Fatalf("B.blockedBy after double block = %v, want one entry", got.BlockedBy)
	}

	// A can lift their own block, once.
	if err := BlockUnblockUser(ctx, a, b, string(constants.StatusUnblocked)); err != nil {
		t.Fatalf("A unblocks B: %v", err)
	}
	if got := loadBlocks(t, ctx, a); len(got.Blocked) != 0 {
		t.Fatalf("A.blocked after unblock = %v, want []", got.Blocked)
	}
	if got := loadBlocks(t, ctx, b); len(got.BlockedBy) != 0 {
		t.Fatalf("B.blockedBy after unblock = %v, want []", got.BlockedBy)
	}
	if err := BlockUnblockUser(ctx, a, b, string(constants.StatusUnblocked)); !errors.Is(err, ErrAlreadyUnblocked) {
		t.Fatalf("A unblocks B again: err = %v, want ErrAlreadyUnblocked", err)
	}
}

// #26: when both people block each other, each block is separate.
func TestMutualBlocksAreIndependent(t *testing.T) {
	ctx := useTestDB(t)
	a, b := bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, a, b)

	for _, step := range []struct{ from, to bson.ObjectID }{{a, b}, {b, a}} {
		if err := BlockUnblockUser(ctx, step.from, step.to, string(constants.StatusBlocked)); err != nil {
			t.Fatalf("block: %v", err)
		}
	}
	if err := BlockUnblockUser(ctx, b, a, string(constants.StatusUnblocked)); err != nil {
		t.Fatalf("B unblocks A: %v", err)
	}
	if got := loadBlocks(t, ctx, a); !slices.Equal(got.Blocked, []bson.ObjectID{b}) || len(got.BlockedBy) != 0 {
		t.Fatalf("A: blocked=%v blockedBy=%v, want blocked=[B] blockedBy=[]", got.Blocked, got.BlockedBy)
	}
	if got := loadBlocks(t, ctx, b); len(got.Blocked) != 0 || !slices.Equal(got.BlockedBy, []bson.ObjectID{a}) {
		t.Fatalf("B: blocked=%v blockedBy=%v, want blocked=[] blockedBy=[A]", got.Blocked, got.BlockedBy)
	}
}

func TestBlockUnknownUser(t *testing.T) {
	ctx := useTestDB(t)
	a := bson.NewObjectID()
	insertTestUsers(t, ctx, a)

	if err := BlockUnblockUser(ctx, a, bson.NewObjectID(), string(constants.StatusBlocked)); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("block unknown user: err = %v, want ErrUserNotFound", err)
	}
}

// #26: suggestions hide a block from both sides, without telling the blocked
// person why.
func TestUserSuggestionsHideBlocksBothWays(t *testing.T) {
	ctx := useTestDB(t)
	a, b, c := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	insertTestUsers(t, ctx, a, b, c)

	if err := BlockUnblockUser(ctx, a, b, string(constants.StatusBlocked)); err != nil {
		t.Fatalf("A blocks B: %v", err)
	}

	suggested := func(user bson.ObjectID) []bson.ObjectID {
		t.Helper()
		got, _, err := GetUserSuggestions(ctx, user, 1, 50, nil)
		if err != nil {
			t.Fatalf("GetUserSuggestions: %v", err)
		}
		ids := make([]bson.ObjectID, 0, len(got))
		for _, s := range got {
			ids = append(ids, s.ID)
		}
		return ids
	}

	if ids := suggested(a); slices.Contains(ids, b) || !slices.Contains(ids, c) {
		t.Fatalf("A's suggestions = %v, want C and not B", ids)
	}
	if ids := suggested(b); slices.Contains(ids, a) || !slices.Contains(ids, c) {
		t.Fatalf("B's suggestions = %v, want C and not A", ids)
	}
}

// #26: blocks stored before the fix were written to both users, so the direction
// is lost. The migration keeps them as a mutual block and drops the old field.
func TestMigrateLegacyBlocks(t *testing.T) {
	ctx := useTestDB(t)
	users := database.DB.Collection(string(constants.UserColl))
	a, b, c, d := bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID(), bson.NewObjectID()
	docs := []any{
		bson.M{"_id": a, "username": "a", "contactInfo": bson.M{"blockedChats": bson.A{b}, "contacts": bson.A{}}},
		bson.M{"_id": b, "username": "b", "contactInfo": bson.M{"blockedChats": bson.A{a}, "contacts": bson.A{}}},
		// One-sided legacy data still ends up mutual.
		bson.M{"_id": c, "username": "c", "contactInfo": bson.M{"blockedChats": bson.A{d}}},
		bson.M{"_id": d, "username": "d", "contactInfo": bson.M{"contacts": bson.A{}}},
	}
	if _, err := users.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert users: %v", err)
	}

	n, err := MigrateLegacyBlocks(ctx)
	if err != nil {
		t.Fatalf("MigrateLegacyBlocks: %v", err)
	}
	if n != 3 {
		t.Fatalf("migrated users = %d, want 3 (A, B, C)", n)
	}

	for _, pair := range []struct{ user, other bson.ObjectID }{{a, b}, {b, a}, {c, d}, {d, c}} {
		got := loadBlocks(t, ctx, pair.user)
		want := []bson.ObjectID{pair.other}
		if !slices.Equal(got.Blocked, want) || !slices.Equal(got.BlockedBy, want) || got.HasLegacy {
			t.Fatalf("user %s: blocked=%v blockedBy=%v legacy=%v, want both [%s] and no legacy field",
				pair.user.Hex(), got.Blocked, got.BlockedBy, got.HasLegacy, pair.other.Hex())
		}
	}

	// Either side can lift their own half.
	if err := BlockUnblockUser(ctx, b, a, string(constants.StatusUnblocked)); err != nil {
		t.Fatalf("B unblocks A after migration: %v", err)
	}
	if got := loadBlocks(t, ctx, a); !slices.Equal(got.Blocked, []bson.ObjectID{b}) {
		t.Fatalf("A.blocked after B's unblock = %v, want [B]", got.Blocked)
	}

	// Running it again changes nothing.
	if n, err := MigrateLegacyBlocks(ctx); err != nil || n != 0 {
		t.Fatalf("second run = %d, %v; want 0, nil", n, err)
	}
}
