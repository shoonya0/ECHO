package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestUserSuggestionsHugePage covers #131. A page far past the end used to
// overflow the skip into a negative number, which MongoDB refuses, so the
// request failed with 500. It must answer with an empty page instead.
func TestUserSuggestionsHugePage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testutil.UseTestDB(t)

	user := bson.NewObjectID()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?page=9223372036854775807&limit=50", nil)
	c.Set("user", models.LoginUserResponse{ID: user})
	c.Set("userId", user.Hex())
	GetUserSuggestions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body)
	}
}
