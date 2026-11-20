package realtime

import (
	"testing"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// A closing connection is unregistered twice, once by the close path (session
// revoke, logout) and once by its read loop. Only the first one does anything,
// so only it logs at info (#173).
func TestUnregisterTwiceLogsInfoOnce(t *testing.T) {
	hub := newTestHub(t)
	hook := logtest.NewLocal(hub.logger)
	hub.logger.SetLevel(logrus.DebugLevel)

	client := newTestClient("c1", bson.NewObjectID())
	addClient(hub, client)

	unregisterAndWait(hub, client)
	unregisterAndWait(hub, client)

	infos := 0
	for _, entry := range hook.AllEntries() {
		if entry.Message == "Unregistering client" && entry.Level == logrus.InfoLevel {
			infos++
		}
	}
	if infos != 1 {
		t.Fatalf("got %d info \"Unregistering client\" entries, want 1", infos)
	}
}
