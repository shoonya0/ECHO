// Package testutil holds helpers shared by tests in different packages.
package testutil

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// UseTestDB points database.DB at a throwaway database on the MongoDB at
// TEST_DB_URI (default: the docker-compose one) and drops it afterwards.
// The test is skipped when MongoDB isn't reachable, or fails when TEST_DB_REQUIRED
// is set (CI), so a broken DB service can't turn the DB tests into silent skips.
func UseTestDB(t *testing.T) context.Context {
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

	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetServerSelectionTimeout(2 * time.Second))
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
