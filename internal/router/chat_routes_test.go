package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/shoonya0/ECHO/internal/config"
	"github.com/shoonya0/ECHO/internal/constants"
)

// The hub stats route is for operators only (#24). Logging in isn't enough:
// the X-Stats-Token header must match STATS_TOKEN, and the route is off while
// STATS_TOKEN is unset. Every refusal looks like a missing route. The JWT check
// is added by RegisterAPIRoutes, so it isn't part of this test.
func TestHubStatsNeedsStatsToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterChatRoutes(r)

	saved := config.Cfg.StatsToken
	t.Cleanup(func() { config.Cfg.StatsToken = saved })

	const token = "operator-stats-token"
	cases := []struct {
		name       string
		configured string // STATS_TOKEN
		send       bool   // whether the request has an X-Stats-Token header
		header     string
		want       int
	}{
		{"off, no header", "", false, "", http.StatusNotFound},
		{"off, empty header", "", true, "", http.StatusNotFound},
		{"on, no header", token, false, "", http.StatusNotFound},
		{"on, wrong token", token, true, "wrong", http.StatusNotFound},
		{"on, start of the token", token, true, token[:8], http.StatusNotFound},
		{"on, right token", token, true, token, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config.Cfg.StatsToken = tc.configured

			req := httptest.NewRequest(http.MethodGet, constants.ApiBasePath+"chats/hub/stats", nil)
			if tc.send {
				req.Header.Set("X-Stats-Token", tc.header)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, tc.want, w.Body)
			}
			if tc.want != http.StatusOK {
				return
			}
			var body struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if _, ok := body.Data["totalClients"]; !ok {
				t.Fatalf("stats have no totalClients: %s", w.Body)
			}
			if _, ok := body.Data["channels"]; ok {
				t.Fatalf("stats still list channels: %s", w.Body)
			}
		})
	}
}
