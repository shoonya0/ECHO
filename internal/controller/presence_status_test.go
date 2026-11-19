package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// PUT /profile refuses a presence status the server doesn't know (#31): it
// would reach contacts as is, and a miscased "Invisible" wouldn't hide the
// user. Refused before any database write, so no MongoDB is needed.
func TestUpdateProfileRefusesUnknownStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []string{"Invisible", "busy", "ONLINE"} {
		t.Run(status, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			body := `{"presence":{"status":"` + status + `"}}`
			c.Request = httptest.NewRequest(http.MethodPut, "/profile/", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", struct{}{})
			c.Set("userId", bson.NewObjectID().Hex())
			UpdateProfile(c)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status %q: code %d, want 400; body %s", status, rec.Code, rec.Body)
			}
		})
	}
}

// PUT /profile refuses a privacy scope the server doesn't know (#32): it
// would hide the user from everyone without them knowing why.
func TestUpdateProfileRefusesUnknownPrivacyScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{"settings":{"privacy":{"showOnlineStatus":"friends"}}}`,
		`{"settings":{"privacy":{"allowContactBy":"Everyone"}}}`,
	} {
		t.Run(body, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPut, "/profile/", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("user", struct{}{})
			c.Set("userId", bson.NewObjectID().Hex())
			UpdateProfile(c)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("code %d, want 400; body %s", rec.Code, rec.Body)
			}
		})
	}
}
