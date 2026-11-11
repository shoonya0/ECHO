package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/config"
	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/testutil"
	"github.com/shoonya0/ECHO/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func useTestSecret(t *testing.T) {
	t.Helper()
	prev := config.Cfg.JWTSecret
	config.Cfg.JWTSecret = "test-secret-for-auth-middleware"
	t.Cleanup(func() { config.Cfg.JWTSecret = prev })
}

func withoutRedis(t *testing.T) {
	t.Helper()
	prev := database.Redis
	database.Redis = nil
	t.Cleanup(func() { database.Redis = prev })
}

// tokenFor issues a login token the way Login does, for an account that was active
// and unbanned when it logged in.
func tokenFor(t *testing.T, id bson.ObjectID, displayName string) string {
	t.Helper()
	token, err := utils.GetJWTToken(models.LoginUserResponse{
		ID:            id,
		Email:         "user@example.com",
		Username:      "user-" + id.Hex(),
		Profile:       models.UserProfileEmbed{DisplayName: displayName},
		AccountStatus: models.AccountStatusEmbed{IsActive: true, IsVerified: true},
	}, time.Now().Add(time.Hour).Unix())
	if err != nil {
		t.Fatalf("GetJWTToken: %v", err)
	}
	return token
}

type authReply struct {
	status int
	body   map[string]any
	raw    string
}

func callAuth(t *testing.T, token string) authReply {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", AuthMiddleware, func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	reply := authReply{status: w.Code, raw: w.Body.String()}
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &reply.body); err != nil {
			t.Fatalf("reply is not JSON: %q", w.Body.String())
		}
	}
	return reply
}

// A ban, a deactivation or a deleted account must lock out a token issued before
// it, not 7 days later when the token expires (#33).
func TestAuthRefusesAccountChangedAfterLogin(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	useTestSecret(t)
	withoutRedis(t)
	users := database.DB.Collection(string(constants.UserColl))

	cases := []struct {
		name     string
		status   bson.M // nil: delete the account
		wantCode int
		wantErr  string
	}{
		{"still active", bson.M{"isActive": true, "isVerified": true, "isBanned": false}, http.StatusOK, ""},
		{"banned", bson.M{"isActive": true, "isVerified": true, "isBanned": true}, http.StatusForbidden, "ACCOUNT_BANNED"},
		{"deactivated", bson.M{"isActive": false, "isVerified": true, "isBanned": false}, http.StatusForbidden, "ACCOUNT_INACTIVE"},
		{"deleted", nil, http.StatusUnauthorized, "ACCOUNT_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := bson.NewObjectID()
			if _, err := users.InsertOne(ctx, bson.M{"_id": id, "accountStatus": bson.M{"isActive": true, "isVerified": true}}); err != nil {
				t.Fatalf("insert user: %v", err)
			}
			token := tokenFor(t, id, "Display")

			if tc.status == nil {
				if _, err := users.DeleteOne(ctx, bson.M{"_id": id}); err != nil {
					t.Fatalf("delete user: %v", err)
				}
			} else if _, err := users.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"accountStatus": tc.status}}); err != nil {
				t.Fatalf("update user: %v", err)
			}

			got := callAuth(t, token)
			if got.status != tc.wantCode {
				t.Fatalf("status = %d, want %d (body %s)", got.status, tc.wantCode, got.raw)
			}
			if tc.wantErr != "" && got.body["code"] != tc.wantErr {
				t.Errorf("code = %v, want %s", got.body["code"], tc.wantErr)
			}
		})
	}
}

// The ban check sat after the empty-displayName branch of an else-if chain, so it
// was skipped for those tokens, and the displayName fallback changed a copy (#33).
func TestAuthChecksAccountWhenDisplayNameIsEmpty(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	useTestSecret(t)
	withoutRedis(t)

	id := bson.NewObjectID()
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, bson.M{
		"_id": id, "accountStatus": bson.M{"isActive": true, "isBanned": true},
	}); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if got := callAuth(t, tokenFor(t, id, "")); got.status != http.StatusForbidden || got.body["code"] != "ACCOUNT_BANNED" {
		t.Errorf("banned account, empty displayName: status %d body %s, want 403 ACCOUNT_BANNED", got.status, got.raw)
	}

	claims, err := verifyToken(tokenFor(t, id, ""))
	if err != nil {
		t.Fatalf("verifyToken: %v", err)
	}
	if claims.DisplayName != "user-"+id.Hex() {
		t.Errorf("DisplayName = %q, want the username fallback", claims.DisplayName)
	}
}

// If Redis can't say whether a token was revoked, a revoked token must not get
// through (#33). The reply is 503, since the token itself may be fine.
func TestAuthRefusesWhenRevocationCheckFails(t *testing.T) {
	useTestSecret(t)
	testutil.UseBrokenRedis(t)

	got := callAuth(t, tokenFor(t, bson.NewObjectID(), "Display"))
	if got.status != http.StatusServiceUnavailable || got.body["code"] != "AUTH_UNAVAILABLE" {
		t.Errorf("status %d body %s, want 503 AUTH_UNAVAILABLE", got.status, got.raw)
	}
}

// If the client hangs up during the account check, the reply is 503 like any failed
// check (it is logged at Debug, not as an outage), and the request never gets through.
func TestAuthCanceledRequestIsRefused(t *testing.T) {
	testutil.UseTestDB(t)
	useTestSecret(t)
	withoutRedis(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", AuthMiddleware, func(c *gin.Context) { c.Status(http.StatusOK) })

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(reqCtx)
	req.Header.Set("Authorization", "Bearer "+tokenFor(t, bson.NewObjectID(), "d"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "AUTH_UNAVAILABLE") {
		t.Errorf("status %d body %s, want 503 AUTH_UNAVAILABLE", w.Code, w.Body.String())
	}
}

// Logout revokes a token by its ID (jti), so a token without one must be refused.
func TestAuthRefusesTokenWithoutID(t *testing.T) {
	useTestSecret(t)
	withoutRedis(t)

	noID := jwt.NewWithClaims(jwt.SigningMethodHS256, utils.JwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   bson.NewObjectID().Hex(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
		Username: "u", Email: "e@example.com", DisplayName: "d",
	})
	signed, err := noID.SignedString([]byte(config.Cfg.JWTSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if got := callAuth(t, signed); got.status != http.StatusUnauthorized || got.body["code"] != "TOKEN_VERIFICATION_FAILED" {
		t.Errorf("status %d body %s, want 401 TOKEN_VERIFICATION_FAILED", got.status, got.raw)
	}
}

// Token failures returned the parser's error text in "details" (#33, #41).
func TestAuthFailuresDontExposeErrorText(t *testing.T) {
	useTestSecret(t)
	withoutRedis(t)

	badSubject := jwt.NewWithClaims(jwt.SigningMethodHS256, utils.JwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "not-an-object-id",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ID:        "jti",
		},
		Username: "u", Email: "e@example.com", DisplayName: "d",
	})
	signed, err := badSubject.SignedString([]byte(config.Cfg.JWTSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	for name, token := range map[string]string{
		"malformed token":   "not.a.jwt",
		"wrong signature":   tokenFor(t, bson.NewObjectID(), "d") + "x",
		"subject not an ID": signed,
	} {
		t.Run(name, func(t *testing.T) {
			got := callAuth(t, token)
			if got.status != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 (body %s)", got.status, got.raw)
			}
			if _, ok := got.body["details"]; ok {
				t.Errorf("reply has details: %s", got.raw)
			}
			for _, leak := range []string{"token parsing failed", "signature", "hex", "ObjectID", "segments"} {
				if strings.Contains(got.raw, leak) {
					t.Errorf("reply mentions %q: %s", leak, got.raw)
				}
			}
		})
	}
}
