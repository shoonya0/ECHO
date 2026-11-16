// Package testutil holds helpers shared by tests in different packages.
package testutil

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// UseTestDB points database.DB at a throwaway database on the MongoDB at
// TEST_DB_URI (default: the docker-compose one) and drops it afterwards.
// The test is skipped when MongoDB isn't reachable, or fails when TEST_DB_REQUIRED
// is set (CI), so a broken DB service can't turn the DB tests into silent skips.
func UseTestDB(t *testing.T) context.Context {
	t.Helper()
	return useTestDB(t, nil)
}

// ReadCounter counts the read commands (find, aggregate, count, distinct) the
// test database receives, per collection.
type ReadCounter struct {
	mu    sync.Mutex
	reads map[string]int
}

// Reads returns how many read commands named collection since the last Reset.
func (c *ReadCounter) Reads(collection string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads[collection]
}

// Reset sets every count back to zero, so a test can count only the code under test.
func (c *ReadCounter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads = map[string]int{}
}

func (c *ReadCounter) started(_ context.Context, e *event.CommandStartedEvent) {
	switch e.CommandName {
	case "find", "aggregate", "count", "distinct":
	default:
		return
	}
	coll, ok := e.Command.Lookup(e.CommandName).StringValueOK()
	if !ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads[coll]++
}

// UseTestDBCountingReads is UseTestDB with a ReadCounter attached, for tests that
// check how many times a request reads a collection.
func UseTestDBCountingReads(t *testing.T) (context.Context, *ReadCounter) {
	t.Helper()
	counter := &ReadCounter{reads: map[string]int{}}
	ctx := useTestDB(t, &event.CommandMonitor{Started: counter.started})
	return ctx, counter
}

func useTestDB(t *testing.T, monitor *event.CommandMonitor) context.Context {
	t.Helper()
	uri := os.Getenv("TEST_DB_URI")
	if uri == "" {
		uri = "mongodb://echo:echo@localhost:27017"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	unavailable := t.Skipf
	if os.Getenv("TEST_DB_REQUIRED") != "" {
		unavailable = t.Fatalf
	}

	opts := options.Client().ApplyURI(uri).SetServerSelectionTimeout(2 * time.Second)
	if monitor != nil {
		opts.SetMonitor(monitor)
	}
	client, err := mongo.Connect(opts)
	if err != nil {
		unavailable("MongoDB unavailable: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		unavailable("MongoDB unavailable: %v", err)
	}

	prev := database.DB
	database.DB = client.Database("echo_test_" + bson.NewObjectID().Hex())
	t.Cleanup(func() {
		_ = database.DB.Drop(context.Background())
		_ = client.Disconnect(context.Background())
		database.DB = prev
	})
	return ctx
}
