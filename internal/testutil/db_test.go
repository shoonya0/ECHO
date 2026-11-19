package testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/shoonya0/ECHO/internal/database"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The test MongoDB must be a replica set, as Atlas is, or multi-document
// transactions fail (#37). A standalone server rejects the first write with
// "Transaction numbers are only allowed on a replica set member or mongos".
func TestTestDBSupportsTransactions(t *testing.T) {
	ctx := UseTestDB(t)

	sess, err := database.DB.Client().StartSession()
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	defer sess.EndSession(context.Background())

	_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := database.DB.Collection("tx_a").InsertOne(ctx, bson.M{"n": 1}); err != nil {
			return nil, err
		}
		_, err := database.DB.Collection("tx_b").InsertOne(ctx, bson.M{"n": 1})
		return nil, err
	})
	if err != nil {
		t.Fatalf("committed transaction: %v", err)
	}

	errRollback := errors.New("roll back")
	_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		if _, err := database.DB.Collection("tx_a").InsertOne(ctx, bson.M{"n": 2}); err != nil {
			return nil, err
		}
		return nil, errRollback
	})
	if !errors.Is(err, errRollback) {
		t.Fatalf("aborted transaction returned %v, want %v", err, errRollback)
	}

	for name, want := range map[string]int64{"tx_a": 1, "tx_b": 1} {
		got, err := database.DB.Collection(name).CountDocuments(ctx, bson.M{})
		if err != nil {
			t.Fatalf("count %s: %v", name, err)
		}
		if got != want {
			t.Errorf("%s holds %d documents, want %d (the aborted insert must not stay)", name, got, want)
		}
	}
}
