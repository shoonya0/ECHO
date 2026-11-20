package router

import (
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/config"
	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"
	"github.com/shoonya0/ECHO/internal/utils"
)

// An authenticated request leaves one "HTTP Request" line, not one per logger,
// and it names the user that auth found (#172).
func TestAuthenticatedRequestLeavesOneAccessLogWithUser(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	withoutRedis(t)
	prevSecret := config.Cfg.JWTSecret
	config.Cfg.JWTSecret = "test-secret-for-router-logs"
	t.Cleanup(func() { config.Cfg.JWTSecret = prevSecret })

	id := bson.NewObjectID()
	email := "logs-" + id.Hex() + "@example.com"
	username := "logs-" + id.Hex()
	users := database.DB.Collection(string(constants.UserColl))
	if _, err := users.InsertOne(ctx, bson.M{
		"_id":           id,
		"email":         email,
		"username":      username,
		"profile":       bson.M{"displayName": "Logs"},
		"accountStatus": bson.M{"isActive": true, "isVerified": true},
	}); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	token, err := utils.GetJWTToken(models.LoginUserResponse{
		ID:            id,
		Email:         email,
		Username:      username,
		Profile:       models.UserProfileEmbed{DisplayName: "Logs"},
		AccountStatus: models.AccountStatusEmbed{IsActive: true, IsVerified: true},
	}, time.Now().Add(time.Hour).Unix())
	if err != nil {
		t.Fatalf("GetJWTToken: %v", err)
	}

	hook := captureLogs(t)
	path := constants.ApiBasePath + "profile/"
	w := serve(t, http.MethodGet, path, "", map[string]string{"Authorization": "Bearer " + token})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body)
	}
	e := requireOneAccessLog(t, hook, path, http.StatusOK)
	if e.Data["user_id"] != id.Hex() {
		t.Fatalf("logged user_id = %v, want %s", e.Data["user_id"], id.Hex())
	}
}
