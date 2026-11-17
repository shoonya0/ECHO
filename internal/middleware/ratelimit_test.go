package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// limitedEngine serves POST /x behind RateLimitByIP with a limit of max requests,
// reading the client IP the way the server does with header.
func limitedEngine(t *testing.T, header string, max int) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if err := ConfigureClientIP(r, header); err != nil {
		t.Fatalf("ConfigureClientIP: %v", err)
	}
	name := "test-" + t.Name()
	r.POST("/x", RateLimitByIP(name, services.Limit{Max: max, Window: time.Minute}), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	// Clear counts an earlier run left for the addresses these tests use.
	for _, ip := range []string{"192.0.2.1", "192.0.2.2", "198.51.100.7", "203.0.113.9"} {
		_ = database.Redis.Del(context.Background(), services.RateLimitKey(name, ip)).Err()
	}
	return r
}

// post sends POST /x from remoteAddr with the given headers.
func post(r *gin.Engine, remoteAddr string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRateLimitByIPRefusesOverLimit(t *testing.T) {
	testutil.UseTestRedis(t)
	r := limitedEngine(t, "", 2)

	for i := 1; i <= 2; i++ {
		if w := post(r, "192.0.2.1:1000", nil); w.Code != http.StatusOK {
			t.Fatalf("request %d: status %d, want 200", i, w.Code)
		}
	}
	w := post(r, "192.0.2.1:1000", nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("request 3: status %d, want 429", w.Code)
	}
	if s, err := strconv.Atoi(w.Header().Get("Retry-After")); err != nil || s < 1 || s > 60 {
		t.Fatalf("Retry-After = %q, want 1 to 60 seconds", w.Header().Get("Retry-After"))
	}
	var body struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Success || body.Error != "RATE_LIMITED" {
		t.Fatalf("body = %s, want success false and error RATE_LIMITED", w.Body.String())
	}

	if w := post(r, "192.0.2.2:1000", nil); w.Code != http.StatusOK {
		t.Fatalf("another IP: status %d, want 200", w.Code)
	}
}

// Without a configured header the IP is the socket address, so a forged
// X-Forwarded-For can't move a client to a fresh bucket.
func TestRateLimitByIPIgnoresForwardedForWithoutHeader(t *testing.T) {
	testutil.UseTestRedis(t)
	r := limitedEngine(t, "", 1)

	post(r, "192.0.2.1:1000", map[string]string{"X-Forwarded-For": "198.51.100.7"})
	w := post(r, "192.0.2.1:1000", map[string]string{"X-Forwarded-For": "203.0.113.9"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d with a new X-Forwarded-For, want 429", w.Code)
	}
}

// With a configured header the bucket follows that header, whatever
// X-Forwarded-For says and whichever proxy address the request came from.
func TestRateLimitByIPUsesConfiguredHeader(t *testing.T) {
	testutil.UseTestRedis(t)
	r := limitedEngine(t, "True-Client-IP", 1)

	post(r, "192.0.2.1:1000", map[string]string{"True-Client-IP": "198.51.100.7", "X-Forwarded-For": "192.0.2.2"})
	w := post(r, "192.0.2.2:1000", map[string]string{"True-Client-IP": "198.51.100.7", "X-Forwarded-For": "203.0.113.9"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("same True-Client-IP: status %d, want 429", w.Code)
	}
	if w := post(r, "192.0.2.1:1000", map[string]string{"True-Client-IP": "203.0.113.9"}); w.Code != http.StatusOK {
		t.Fatalf("other True-Client-IP behind the same proxy: status %d, want 200", w.Code)
	}
}

// A header value that isn't an IP can't name a bucket: the socket address counts.
func TestRateLimitByIPDropsNonIPHeader(t *testing.T) {
	testutil.UseTestRedis(t)
	r := limitedEngine(t, "True-Client-IP", 1)

	post(r, "192.0.2.1:1000", map[string]string{"True-Client-IP": "random-1"})
	w := post(r, "192.0.2.1:1000", map[string]string{"True-Client-IP": "random-2"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d with a new non-IP header, want 429", w.Code)
	}
}
