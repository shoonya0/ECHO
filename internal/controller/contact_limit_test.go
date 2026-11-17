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

// parsePage counts pages from 1: absent, 0, negative and unparseable values
// mean the first page.
func TestParsePage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := map[string]int{
		"": 1, "abc": 1, "0": 1, "-2": 1, "1": 1, "7": 7,
		"10000": 10000, "10001": 10000, "9223372036854775807": 10000,
	}
	for query, want := range cases {
		t.Run("page="+query, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/?page="+query, nil)
			if got := parsePage(c); got != want {
				t.Fatalf("parsePage(%q) = %d, want %d", query, got, want)
			}
		})
	}
}
