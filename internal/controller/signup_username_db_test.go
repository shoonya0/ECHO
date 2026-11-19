package controller

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// Usernames are unique ignoring case and need the new format (#36). Signup
// stores the name trimmed, with its capitals.
func TestSignupUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := testutil.UseTestDB(t)
	if err := database.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}

	tests := []struct {
		name, email, username string
		want                  int
	}{
		{"first", "a@example.com", " Alice ", http.StatusOK},
		{"taken in other case", "b@example.com", "alice", http.StatusConflict},
		{"too short", "c@example.com", "al", http.StatusBadRequest},
		{"bad character", "d@example.com", "al ice", http.StatusBadRequest},
		{"bad name with a taken email", "a@example.com", "x", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := postJSON(Signup, `{"email":"`+tt.email+`","username":"`+tt.username+`","password":"password123"}`)
			if w.Code != tt.want {
				t.Fatalf("status %d, want %d (body %s)", w.Code, tt.want, w.Body)
			}
		})
	}

	var stored models.User
	if err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx, bson.M{"email": "a@example.com"}).Decode(&stored); err != nil {
		t.Fatalf("find the user: %v", err)
	}
	if stored.Username != "Alice" {
		t.Fatalf("stored username %q, want %q", stored.Username, "Alice")
	}
	if n, err := database.DB.Collection(string(constants.UserColl)).CountDocuments(ctx, bson.M{}); err != nil || n != 1 {
		t.Fatalf("users stored: %d (err %v), want 1", n, err)
	}
}
