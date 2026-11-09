package controller

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestSignupGeneratesTheUserID covers #34. A client-sent _id must never become
// the account's ID: it would let anyone sign up as a deleted user and inherit
// their chats. The reply must carry the ID that was actually stored.
func TestSignupGeneratesTheUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := testutil.UseTestDB(t)

	tests := []struct {
		name, email string
		sendID      bool
	}{
		{"client sends _id", "chosen@example.com", true},
		{"no _id", "plain@example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A deleted user's ID: no document has it any more.
			deleted := bson.NewObjectID()
			extra := ""
			if tt.sendID {
				extra = `"_id":"` + deleted.Hex() + `",`
			}
			body := `{` + extra + `"email":"` + tt.email + `","username":"u","password":"password123"}`
			w := postJSON(Signup, body)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body)
			}
			var resp struct {
				Data struct {
					ID string `json:"_id"`
				} `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			users := database.DB.Collection(string(constants.UserColl))
			// Checked by ID, not only through the email lookup below, so a second
			// document under the chosen ID can't hide behind the generated one.
			if tt.sendID {
				if n, err := users.CountDocuments(ctx, bson.M{"_id": deleted}); err != nil {
					t.Fatalf("count users with the chosen _id: %v", err)
				} else if n != 0 {
					t.Fatalf("a user was stored under the client-chosen _id %s", deleted.Hex())
				}
			}

			var stored models.User
			if err := users.FindOne(ctx, bson.M{"email": tt.email}).Decode(&stored); err != nil {
				t.Fatalf("find created user: %v", err)
			}
			if stored.ID.IsZero() {
				t.Fatalf("stored user has a zero _id")
			}
			if resp.Data.ID != stored.ID.Hex() {
				t.Fatalf("reply _id = %q, want the stored ID %q", resp.Data.ID, stored.ID.Hex())
			}
		})
	}
}
