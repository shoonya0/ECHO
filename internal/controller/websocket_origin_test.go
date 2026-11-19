package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/shoonya0/ECHO/internal/config"
)

// #40: the upgrade accepts no Origin (the Android app sends none), the server's own
// host and the origins in WS_ALLOWED_ORIGINS, and refuses any other web page.
func TestWebSocketOriginCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return // the upgrader has answered
		}
		_ = conn.Close()
	}))
	t.Cleanup(server.Close)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	saved := config.Cfg.AllowedOrigins
	config.Cfg.AllowedOrigins = []string{"https://app.example.com"}
	t.Cleanup(func() { config.Cfg.AllowedOrigins = saved })

	cases := []struct {
		name   string
		origin string
		want   int
	}{
		{"no origin", "", http.StatusSwitchingProtocols},
		{"own host", server.URL, http.StatusSwitchingProtocols},
		{"listed origin", "https://APP.example.com", http.StatusSwitchingProtocols},
		{"other site", "https://evil.example.com", http.StatusForbidden},
		{"listed host, other scheme", "http://app.example.com", http.StatusForbidden},
		{"unparsable", "://", http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			header := http.Header{}
			if c.origin != "" {
				header.Set("Origin", c.origin)
			}
			conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
			if conn != nil {
				_ = conn.Close()
			}
			if resp == nil {
				t.Fatalf("no response: %v", err)
			}
			if resp.StatusCode != c.want {
				t.Fatalf("status = %d, want %d (err %v)", resp.StatusCode, c.want, err)
			}
		})
	}
}
