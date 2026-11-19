package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shoonya0/ECHO/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// #43: afterSeq must be a number from 0 up, and can't be combined with before,
// which pages the other way. These are refused before any database read.
func TestGetChatMessagesRejectsBadAfterSeq(t *testing.T) {
	gin.SetMode(gin.TestMode)
	chatID := bson.NewObjectID().Hex()
	cases := map[string]string{
		"not a number":  "afterSeq=abc",
		"negative":      "afterSeq=-1",
		"too large":     "afterSeq=99999999999999999999",
		"with a before": "afterSeq=3&before=" + bson.NewObjectID().Hex(),
	}
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest("GET", "/chats/"+chatID+"/messages?"+query, nil)
			c.Params = gin.Params{{Key: "chatID", Value: chatID}}
			c.Set("user", models.LoginUserResponse{ID: bson.NewObjectID()})

			GetChatMessagesHTTP(c)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("?%s: status %d, want 400", query, rec.Code)
			}
		})
	}
}
