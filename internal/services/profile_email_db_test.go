package services

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// The email can't be changed through the profile update (#36): it had no
// password check, no format check and answered 500 on a taken email. Other
// fields in the same update are still applied.
func TestUpdateProfileKeepsEmail(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	id := insertAccount(t, ctx, bson.M{"email": "keep@example.com", "phone": "1"})

	if err := UpdateProfile(ctx, id, map[string]interface{}{
		"email": "taken-over@example.com",
		"phone": "2",
	}); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	var got struct {
		Email string `bson:"email"`
		Phone string `bson:"phone"`
	}
	if err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, bson.M{"_id": id}).Decode(&got); err != nil {
		t.Fatalf("read user: %v", err)
	}
	if got.Email != "keep@example.com" {
		t.Fatalf("email = %q, want it unchanged", got.Email)
	}
	if got.Phone != "2" {
		t.Fatalf("phone = %q, want the update applied", got.Phone)
	}
}

// Only the email field itself is immutable, not every key that starts with
// "email" (code review of #36).
func TestIsImmutableUpdateKeyEmailIsExact(t *testing.T) {
	if !isImmutableUpdateKey("email") {
		t.Fatalf("email: want immutable")
	}
	if isImmutableUpdateKey("emailVerified") {
		t.Fatalf("emailVerified: want it left to the whitelist")
	}
}
