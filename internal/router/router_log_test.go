package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/logger"
)

// captureLogs records every entry the shared logger writes during the test.
func captureLogs(t *testing.T) *logtest.Hook {
	t.Helper()
	base := logger.Base()
	prevLevel := base.GetLevel()
	base.SetLevel(logrus.DebugLevel)
	hook := logtest.NewLocal(base)
	t.Cleanup(func() {
		base.SetLevel(prevLevel)
		base.ReplaceHooks(make(logrus.LevelHooks))
	})
	return hook
}

// withoutRedis turns the rate limiter and token blacklist off for the test.
func withoutRedis(t *testing.T) {
	t.Helper()
	prev := database.Redis
	database.Redis = nil
	t.Cleanup(func() { database.Redis = prev })
}

// accessLogs returns the "HTTP Request" entries the request left.
func accessLogs(hook *logtest.Hook) []*logrus.Entry {
	var out []*logrus.Entry
	for _, e := range hook.AllEntries() {
		if e.Message == "HTTP Request" {
			out = append(out, e)
		}
	}
	return out
}

// serve sends one request through the full API router and returns its reply.
func serve(t *testing.T, method, path, body string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterAPIRoutes(r)

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// requireOneAccessLog checks the request left exactly one "HTTP Request" entry,
// with the status it was answered with.
func requireOneAccessLog(t *testing.T, hook *logtest.Hook, path string, status int) *logrus.Entry {
	t.Helper()
	logs := accessLogs(hook)
	if len(logs) != 1 {
		t.Fatalf("%s left %d \"HTTP Request\" entries, want 1", path, len(logs))
	}
	e := logs[0]
	if e.Data["status"] != status {
		t.Fatalf("logged status = %v, want %d", e.Data["status"], status)
	}
	if e.Data["path"] != path {
		t.Fatalf("logged path = %v, want %s", e.Data["path"], path)
	}
	if _, ok := e.Data["request_id"]; !ok {
		t.Fatalf("access log has no request_id: %v", e.Data)
	}
	return e
}

// Login, signup and requests refused by auth never reached the access logger,
// so they left no "HTTP Request" line (#172).
func TestEveryRequestLeavesOneAccessLog(t *testing.T) {
	withoutRedis(t)
	const password = "hunter2-do-not-log"
	creds := fmt.Sprintf(`{"email":"","password":%q}`, password)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		header map[string]string
		want   int
	}{
		{"login", http.MethodPost, constants.ApiBasePath + "login", creds, nil, http.StatusBadRequest},
		{"signup", http.MethodPost, constants.ApiBasePath + "signup", creds, nil, http.StatusBadRequest},
		{"no token", http.MethodGet, constants.ApiBasePath + "profile/", "", nil, http.StatusUnauthorized},
		{"bad token", http.MethodGet, constants.ApiBasePath + "profile/", "", map[string]string{"Authorization": "Bearer nope"}, http.StatusUnauthorized},
		{"websocket, no token", http.MethodGet, constants.ApiBasePath + "ws/chat", "", nil, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hook := captureLogs(t)
			w := serve(t, tc.method, tc.path, tc.body, tc.header)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, tc.want, w.Body)
			}
			requireOneAccessLog(t, hook, tc.path, tc.want)

			for _, e := range hook.AllEntries() {
				line, err := e.String()
				if err != nil {
					t.Fatalf("format entry: %v", err)
				}
				if strings.Contains(line, password) {
					t.Fatalf("a log entry holds the request's password: %s", line)
				}
			}
		})
	}
}
