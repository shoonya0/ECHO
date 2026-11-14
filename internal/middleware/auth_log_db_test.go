package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/testutil"
)

// TestAuthLogsOnlyTheUserID covers #84. The "authenticated" log line must name
// the user by ID only, not by email or username, even at Debug.
func TestAuthLogsOnlyTheUserID(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	useTestSecret(t)
	withoutRedis(t)

	base := logger.Base()
	prevLevel := base.GetLevel()
	base.SetLevel(logrus.DebugLevel)
	hook := logtest.NewLocal(base)
	t.Cleanup(func() {
		base.SetLevel(prevLevel)
		base.ReplaceHooks(make(logrus.LevelHooks))
	})

	id := bson.NewObjectID()
	users := database.DB.Collection(string(constants.UserColl))
	if _, err := users.InsertOne(ctx, bson.M{"_id": id, "accountStatus": bson.M{"isActive": true, "isVerified": true}}); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if got := callAuth(t, tokenFor(t, id, "Display")); got.status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", got.status, got.raw)
	}

	var authed *logrus.Entry
	for _, e := range hook.AllEntries() {
		if e.Message == "User authenticated successfully" {
			authed = e
		}
	}
	if authed == nil {
		t.Fatal("no \"User authenticated successfully\" log entry")
	}
	if authed.Data["userId"] != id.Hex() {
		t.Fatalf("userId = %#v, want %s", authed.Data["userId"], id.Hex())
	}
	for key, value := range authed.Data {
		text := fmt.Sprintf("%v", value)
		if strings.Contains(text, "user@example.com") || strings.Contains(text, "user-"+id.Hex()) {
			t.Fatalf("log field %q leaks the email or username: %s", key, text)
		}
	}
}
