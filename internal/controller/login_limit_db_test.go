package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/shoonya0/ECHO/internal/config"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// useLoginLimits sets small login limits for the test and clears any count an
// earlier run left under the login keys.
func useLoginLimits(t *testing.T, pairMax, emailMax int) {
	t.Helper()
	prevPair, prevEmail := services.LoginPairFailLimit, services.LoginEmailFailLimit
	services.LoginPairFailLimit = services.Limit{Max: pairMax, Window: time.Minute}
	services.LoginEmailFailLimit = services.Limit{Max: emailMax, Window: time.Minute}
	t.Cleanup(func() { services.LoginPairFailLimit, services.LoginEmailFailLimit = prevPair, prevEmail })

	ctx := context.Background()
	for _, pattern := range []string{"ratelimit:login-pair:*", "ratelimit:login-email:*"} {
		keys, err := database.Redis.Keys(ctx, pattern).Result()
		if err != nil {
			t.Fatalf("listing %s: %v", pattern, err)
		}
		if len(keys) > 0 {
			if err := database.Redis.Del(ctx, keys...).Err(); err != nil {
				t.Fatalf("clearing %s: %v", pattern, err)
			}
		}
	}

	prevSecret := config.Cfg.JWTSecret
	config.Cfg.JWTSecret = "test-secret-for-login-limit"
	t.Cleanup(func() { config.Cfg.JWTSecret = prevSecret })
}

// loginFrom calls Login with body from the client address ip.
func loginFrom(ip, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.RemoteAddr = ip + ":4000"
	Login(c)
	return w
}

const (
	victimSignup = `{"email":"victim@example.com","username":"victim","password":"password123"}`
	victimRight  = `{"email":"victim@example.com","password":"password123"}`
	victimWrong  = `{"email":"victim@example.com","password":"wrong-guess"}`
)

func setupVictim(t *testing.T, pairMax, emailMax int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	testutil.UseTestDB(t)
	testutil.UseTestRedis(t)
	useLoginLimits(t, pairMax, emailMax)
	if w := postJSON(Signup, victimSignup); w.Code != http.StatusOK {
		t.Fatalf("signup: status %d (body %s)", w.Code, w.Body)
	}
}

// One IP that guesses is stopped on that account, even with the right password,
// but the owner on another IP can still log in (#40).
func TestLoginPairLimitStopsOneIPOnly(t *testing.T) {
	setupVictim(t, 3, 100)

	for i := 1; i <= 3; i++ {
		if w := loginFrom("198.51.100.1", victimWrong); w.Code != http.StatusUnauthorized {
			t.Fatalf("guess %d: status %d, want 401", i, w.Code)
		}
	}
	w := loginFrom("198.51.100.1", victimRight)
	if w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("guessing IP after the limit: status %d, Retry-After %q; want 429 with a wait",
			w.Code, w.Header().Get("Retry-After"))
	}
	if w := loginFrom("203.0.113.5", victimRight); w.Code != http.StatusOK {
		t.Fatalf("owner on another IP: status %d, want 200 (body %s)", w.Code, w.Body)
	}
}

// Guesses spread over many IPs stop at the per-account limit.
func TestLoginEmailLimitStopsSpreadGuessing(t *testing.T) {
	setupVictim(t, 100, 4)

	for i, ip := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3", "198.51.100.4"} {
		if w := loginFrom(ip, victimWrong); w.Code != http.StatusUnauthorized {
			t.Fatalf("guess %d: status %d, want 401", i+1, w.Code)
		}
	}
	if w := loginFrom("198.51.100.5", victimRight); w.Code != http.StatusTooManyRequests {
		t.Fatalf("new IP after the account limit: status %d, want 429", w.Code)
	}
}

// A successful login clears the counts, so earlier typos don't add up.
func TestLoginSuccessClearsTheCount(t *testing.T) {
	setupVictim(t, 3, 100)
	ip := "198.51.100.1"

	for range 2 {
		loginFrom(ip, victimWrong)
	}
	if w := loginFrom(ip, victimRight); w.Code != http.StatusOK {
		t.Fatalf("right password: status %d, want 200", w.Code)
	}
	for range 2 {
		loginFrom(ip, victimWrong)
	}
	if w := loginFrom(ip, victimRight); w.Code != http.StatusOK {
		t.Fatalf("right password after 2 more typos: status %d, want 200 (body %s)", w.Code, w.Body)
	}
}

// Guesses sent together are counted before bcrypt, so no more than the limit
// are checked against the password (review finding on #40).
func TestLoginLimitHoldsForConcurrentGuesses(t *testing.T) {
	setupVictim(t, 3, 100)

	var checked atomic.Int32
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if w := loginFrom("198.51.100.1", victimWrong); w.Code == http.StatusUnauthorized {
				checked.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := checked.Load(); got != 3 {
		t.Fatalf("%d of 20 concurrent guesses reached the password check, want 3", got)
	}
}

// Unknown emails get no per-account count, so made-up emails can't fill Redis.
func TestLoginUnknownEmailStoresNoCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testutil.UseTestDB(t)
	testutil.UseTestRedis(t)
	useLoginLimits(t, 3, 100)

	for range 5 {
		if w := loginFrom("198.51.100.1", `{"email":"nobody@example.com","password":"password123"}`); w.Code != http.StatusUnauthorized {
			t.Fatalf("unknown email: status %d, want 401", w.Code)
		}
	}
	keys, err := database.Redis.Keys(context.Background(), "ratelimit:login-*").Result()
	if err != nil {
		t.Fatalf("listing keys: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("unknown email left keys %v, want none", keys)
	}
}
