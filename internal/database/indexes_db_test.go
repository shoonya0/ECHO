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
