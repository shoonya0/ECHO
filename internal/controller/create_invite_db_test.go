package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestCreateInviteReturnsTheCode covers #104. Creating an invite code must
// return it, with its expiry, so the client can share it without listing the
// group's codes and guessing which one is new.
func TestCreateInviteReturnsTheCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := testutil.UseTestDB(t)

	owner, groupID := bson.NewObjectID(), bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.ChatColl)).InsertOne(ctx, bson.M{
		"_id":      groupID,
		"chatType": string(constants.ChatTypeGroup),
		"participants": bson.M{owner.Hex(): bson.M{
			"role": string(constants.ChatRoleOwner), "status": string(constants.StatusAccepted),
		}},
	}); err != nil {
		t.Fatalf("insert group: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Params = gin.Params{{Key: "groupID", Value: groupID.Hex()}}
	c.Set("user", models.LoginUserResponse{ID: owner})
	CreateInvite(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body)
	}
	var resp struct {
		Data *models.InviteCodeEmbed `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Data == nil || resp.Data.InviteCode == "" {
		t.Fatalf("body = %s, want data.inviteCode", w.Body)
	}
	if left := time.Until(resp.Data.ExpiredDate); left < 23*time.Hour || left > 25*time.Hour {
		t.Errorf("expiredDate = %v, want about 24 hours from now", resp.Data.ExpiredDate)
	}

	var chat models.Chat
	if err := database.DB.Collection(string(constants.ChatColl)).FindOne(ctx, bson.M{"_id": groupID}).Decode(&chat); err != nil {
		t.Fatalf("load group: %v", err)
	}
	if len(chat.InviteCode) != 1 || chat.InviteCode[0].InviteCode != resp.Data.InviteCode {
		t.Fatalf("stored codes = %+v, want only the returned %s", chat.InviteCode, resp.Data.InviteCode)
	}
}
