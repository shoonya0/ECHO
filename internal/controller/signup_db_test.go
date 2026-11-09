package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

	// A deleted user's ID: no document has it any more.
	deleted := bson.NewObjectID()

	tests := []struct {
		name, email, extra string
	}{
		{"client sends _id", "chosen@example.com", `"_id":"` + deleted.Hex() + `",`},
		{"no _id", "plain@example.com", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{` + tt.extra + `"email":"` + tt.email + `","username":"u","password":"password123"}`
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			Signup(c)

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

			var stored models.User
			if err := database.DB.Collection(string(constants.UserColl)).
				FindOne(ctx, bson.M{"email": tt.email}).Decode(&stored); err != nil {
				t.Fatalf("find created user: %v", err)
			}
			if stored.ID == deleted {
				t.Fatalf("signup stored the client-chosen _id %s", deleted.Hex())
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
