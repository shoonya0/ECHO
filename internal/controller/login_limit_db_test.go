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
	"golang.org/x/crypto/bcrypt"

	"github.com/shoonya0/ECHO/internal/config"
	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/testutil"
	"github.com/shoonya0/ECHO/internal/utils"
)

// useLoginLimits sets small login limits for the test and clears any count an
// earlier run left under the login keys.
func useLoginLimits(t *testing.T, pairMax, emailMax int) {
	t.Helper()
	prevPair, prevEmail := services.LoginPairFailLimit, services.LoginAccountLimit
	services.LoginPairFailLimit = services.Limit{Max: pairMax, Window: time.Minute}
	services.LoginAccountLimit = services.Limit{Max: emailMax, Window: time.Minute}
	t.Cleanup(func() { services.LoginPairFailLimit, services.LoginAccountLimit = prevPair, prevEmail })

	ctx := context.Background()
	for _, pattern := range []string{"ratelimit:login-*"} {
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
func TestLoginAccountLimitStopsSpreadGuessing(t *testing.T) {
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

// An attacker's lookalike account must not reset the victim's count (security
// review of #40). Since #36 signup refuses an email that differs only in case,
// and login lowercases the email, so even a lookalike stored before #36 can't be
// logged into: its password is checked against the victim's account and counted.
func TestLoginLookalikeAccountKeepsVictimCount(t *testing.T) {
	setupVictim(t, 3, 100)
	if w := postJSON(Signup, `{"email":"VICTIM@example.com","username":"decoy","password":"decoy-pass-1"}`); w.Code != http.StatusConflict {
		t.Fatalf("decoy signup: status %d, want 409 (body %s)", w.Code, w.Body)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("decoy-pass-1"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	legacy := utils.NewUserWithDefaults("VICTIM@example.com", "decoy", string(hash))
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(context.Background(), legacy); err != nil {
		t.Fatalf("insert legacy lookalike: %v", err)
	}
	ip := "198.51.100.1"

	for range 2 {
		loginFrom(ip, victimWrong)
	}
	if w := loginFrom(ip, `{"email":"VICTIM@example.com","password":"decoy-pass-1"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("login with the lookalike's password: status %d, want 401 (body %s)", w.Code, w.Body)
	}
	// That was the third failure on the victim's account: at the limit.
	if w := loginFrom(ip, victimRight); w.Code != http.StatusTooManyRequests {
		t.Fatalf("next try after the lookalike login: status %d, want 429", w.Code)
	}
}
