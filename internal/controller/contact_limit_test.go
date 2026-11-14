package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// parseLimit keeps the contact lists' limit in 1..100 (#39): 0, negative and
// unparseable values fall back to the default of 10, larger ones are capped.
func TestParseLimitStaysInRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := map[string]int{
		"":     10,
		"abc":  10,
		"0":    10,
		"-3":   10,
		"1":    1,
		"25":   25,
		"100":  100,
		"101":  100,
		"5000": 100,
	}
	for query, want := range cases {
		t.Run("limit="+query, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/?limit="+query, nil)
			if got := parseLimit(c); got != want {
				t.Fatalf("parseLimit(%q) = %d, want %d", query, got, want)
			}
		})
	}
}
