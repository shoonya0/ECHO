package controller

import (
	"net/http"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/utils"
)

// Emails are compared without regard to case (#36): signup stores the address in
// lower case, so a second signup with other capitals is the same account.
func TestSignupStoresEmailLowercaseAndRejectsOtherCase(t *testing.T) {
	setupVictim(t, 100, 100) // victim@example.com

	w := postJSON(Signup, `{"email":"Victim@Example.COM","username":"other","password":"password123"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("second signup with other capitals: status %d, want 409 (body %s)", w.Code, w.Body)
	}

	w = postJSON(Signup, `{"email":"Mixed@Case.com","username":"mixed","password":"password123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("signup Mixed@Case.com: status %d (body %s)", w.Code, w.Body)
	}
	var stored models.User
	users := database.DB.Collection(string(constants.UserColl))
	if err := users.FindOne(t.Context(), bson.M{"email": "mixed@case.com"}).Decode(&stored); err != nil {
		t.Fatalf("find the user under the lower-case email: %v", err)
	}
}

// Logging in with other capitals than at signup finds the account (#36).
func TestLoginIgnoresEmailCase(t *testing.T) {
	setupVictim(t, 100, 100)
	w := loginFrom("192.0.2.10", `{"email":"VICTIM@example.com","password":"password123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("login with other capitals: status %d, want 200 (body %s)", w.Code, w.Body)
	}
}

// Wrong guesses under different capitals count toward the same account's limit
// (#36, #40): changing the case must not reset the count.
func TestLoginLimitCountsAcrossEmailCase(t *testing.T) {
	setupVictim(t, 2, 100)
	for _, email := range []string{"victim@example.com", "Victim@Example.com"} {
		if w := loginFrom("192.0.2.11", `{"email":"`+email+`","password":"wrong-guess"}`); w.Code != http.StatusUnauthorized {
			t.Fatalf("wrong guess as %s: status %d, want 401", email, w.Code)
		}
	}
	if w := loginFrom("192.0.2.11", `{"email":"VICTIM@EXAMPLE.COM","password":"wrong-guess"}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("third guess with yet other capitals: status %d, want 429", w.Code)
	}
}

// insertLegacyUser stores a user with its email exactly as given, the way
// signup did before #36.
func insertLegacyUser(t *testing.T, email, password string) bson.ObjectID {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	u := utils.NewUserWithDefaults(email, "legacy", string(hash))
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(t.Context(), u); err != nil {
		t.Fatalf("insert legacy user %s: %v", email, err)
	}
	return u.ID
}

// A user stored with capitals before #36 can log in before the startup
// migration has lowered their email, or if it never runs (code review of #36).
func TestLoginFindsLegacyMixedCaseEmail(t *testing.T) {
	setupVictim(t, 100, 100)
	insertLegacyUser(t, "Legacy@Case.com", "password123")
	if w := loginFrom("192.0.2.12", `{"email":"legacy@case.com","password":"password123"}`); w.Code != http.StatusOK {
		t.Fatalf("login to a legacy mixed-case account: status %d, want 200 (body %s)", w.Code, w.Body)
	}
}

// Signup can't create a second account for a mailbox stored with capitals
// before #36, which would take over its logins (code review of #36).
func TestSignupRefusesLegacyMixedCaseEmail(t *testing.T) {
	setupVictim(t, 100, 100)
	insertLegacyUser(t, "Owner@Case.com", "password123")
	if w := postJSON(Signup, `{"email":"owner@case.com","username":"taker","password":"password123"}`); w.Code != http.StatusConflict {
		t.Fatalf("signup over a legacy mixed-case email: status %d, want 409 (body %s)", w.Code, w.Body)
	}
}
