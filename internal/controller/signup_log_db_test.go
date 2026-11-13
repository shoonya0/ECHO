package controller

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"

	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// TestSignupLogsOnlyTheUserID covers #65. The "user created" log line must
// name the account by ID only: not its email, and never the password hash.
func TestSignupLogsOnlyTheUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	testutil.UseTestDB(t)

	hook := logtest.NewLocal(logger.Base())
	t.Cleanup(func() {
		logger.Base().ReplaceHooks(make(logrus.LevelHooks))
	})

	const email = "logged@example.com"
	w := postJSON(Signup, `{"email":"`+email+`","username":"u","password":"password123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body)
	}

	var created *logrus.Entry
	for _, e := range hook.AllEntries() {
		if e.Message == "user created successfully" {
			created = e
		}
	}
	if created == nil {
		t.Fatal("no \"user created successfully\" log entry")
	}
	id, ok := created.Data["user"].(string)
	if !ok || len(id) != 24 {
		t.Fatalf("user field = %#v, want the 24-character hex ID", created.Data["user"])
	}
	for key, value := range created.Data {
		text := fmt.Sprintf("%+v", value)
		if strings.Contains(text, email) || strings.Contains(text, "$2a$") {
			t.Fatalf("log field %q leaks the email or password hash: %s", key, text)
		}
	}
}
