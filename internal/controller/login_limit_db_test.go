package controller

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/shoonya0/ECHO/internal/config"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// useLoginEmailLimit sets a small per-email failed-login limit and clears any
// count an earlier run left for email.
func useLoginEmailLimit(t *testing.T, max int, email string) {
	t.Helper()
	prev := services.LoginEmailFailLimit
	services.LoginEmailFailLimit = services.Limit{Max: max, Window: time.Minute}
	t.Cleanup(func() { services.LoginEmailFailLimit = prev })
	key := services.RateLimitKey("login-email", services.EmailRateLimitID(email))
	if err := database.Redis.Del(context.Background(), key).Err(); err != nil {
		t.Fatalf("clearing %s: %v", key, err)
	}
}

func useLoginSecret(t *testing.T) {
	t.Helper()
	prev := config.Cfg.JWTSecret
	config.Cfg.JWTSecret = "test-secret-for-login-limit"
	t.Cleanup(func() { config.Cfg.JWTSecret = prev })
}

// TestLoginEmailLimitStopsGuessing covers #40: after Max wrong passwords for one
// email, even the right password is refused with 429 until the window ends, and
// a different spelling of the email shares the count.
func TestLoginEmailLimitStopsGuessing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testutil.UseTestDB(t)
	testutil.UseTestRedis(t)
	useLoginSecret(t)
	useLoginEmailLimit(t, 3, "victim@example.com")

	if w := postJSON(Signup, `{"email":"victim@example.com","username":"victim","password":"password123"}`); w.Code != http.StatusOK {
		t.Fatalf("signup: status %d (body %s)", w.Code, w.Body)
	}
	if w := postJSON(Login, `{"email":"victim@example.com","password":"password123"}`); w.Code != http.StatusOK {
		t.Fatalf("login before any failure: status %d, want 200 (body %s)", w.Code, w.Body)
	}

	for i := 1; i <= 3; i++ {
		if w := postJSON(Login, `{"email":"victim@example.com","password":"wrong-guess"}`); w.Code != http.StatusUnauthorized {
			t.Fatalf("wrong password %d: status %d, want 401", i, w.Code)
		}
	}
	w := postJSON(Login, `{"email":"Victim@Example.COM","password":"password123"}`)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("right password after the limit: status %d, want 429 (body %s)", w.Code, w.Body)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("429 without Retry-After")
	}
}

// TestLoginEmailLimitCountsUnknownEmails checks that an unknown email counts the
// same as a wrong password, so the limit doesn't tell which emails exist.
func TestLoginEmailLimitCountsUnknownEmails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testutil.UseTestDB(t)
	testutil.UseTestRedis(t)
	useLoginEmailLimit(t, 2, "nobody@example.com")

	for i := 1; i <= 2; i++ {
		if w := postJSON(Login, `{"email":"nobody@example.com","password":"password123"}`); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d, want 401", i, w.Code)
		}
	}
	if w := postJSON(Login, `{"email":"nobody@example.com","password":"password123"}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt 3: status %d, want 429", w.Code)
	}
}
