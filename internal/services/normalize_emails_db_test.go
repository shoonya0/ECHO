package services

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/testutil"
)

func storedEmail(t *testing.T, id bson.ObjectID) string {
	t.Helper()
	var u struct {
		Email string `bson:"email"`
	}
	if err := database.DB.Collection(string(constants.UserColl)).FindOne(t.Context(), bson.M{"_id": id}).Decode(&u); err != nil {
		t.Fatalf("read user %s: %v", id.Hex(), err)
	}
	return u.Email
}

// Emails stored before #36 keep their capitals; the startup migration lowers
// them. One that would collide with an existing lower-case account is left as
// it is and reported, since only a person can decide how to merge the two.
// Run without indexes: the unique email index may be missing, and the
// migration must still not give two accounts one email (code review of #36).
func TestNormalizeEmails(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	mixed := insertAccount(t, ctx, bson.M{"email": " Mixed@Case.COM"})
	unicode := insertAccount(t, ctx, bson.M{"email": "Ärger@X.de"})
	plain := insertAccount(t, ctx, bson.M{"email": "plain@example.com"})
	owner := insertAccount(t, ctx, bson.M{"email": "victim@example.com"})
	lookalike := insertAccount(t, ctx, bson.M{"email": "VICTIM@example.com"})

	updated, conflicts, err := NormalizeEmails(ctx)
	if err != nil {
		t.Fatalf("NormalizeEmails: %v", err)
	}
	if updated != 2 || len(conflicts) != 1 || conflicts[0] != lookalike {
		t.Fatalf("updated %d, conflicts %v; want 2 and [%s]", updated, conflicts, lookalike.Hex())
	}
	for id, want := range map[bson.ObjectID]string{
		mixed:     "mixed@case.com",
		unicode:   "ärger@x.de",
		plain:     "plain@example.com",
		owner:     "victim@example.com",
		lookalike: "VICTIM@example.com",
	} {
		if got := storedEmail(t, id); got != want {
			t.Errorf("user %s: email %q, want %q", id.Hex(), got, want)
		}
	}

	// A second run changes nothing and reports the same conflict.
	updated, conflicts, err = NormalizeEmails(ctx)
	if err != nil || updated != 0 || len(conflicts) != 1 {
		t.Fatalf("second run: updated %d, conflicts %v, err %v; want 0, 1 conflict, nil", updated, conflicts, err)
	}
}
