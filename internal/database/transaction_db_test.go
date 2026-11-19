package database_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The test MongoDB is a replica set, so the startup check passes (#37). On a
// standalone server it returns ErrNoTransactions; that case is checked by hand
// (see the PR), as CI has no standalone server.
func TestRequireTransactionsOnReplicaSet(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	if err := database.RequireTransactions(ctx); err != nil {
		t.Fatalf("RequireTransactions: %v", err)
	}
}

// fn's error aborts every write fn made and comes back unchanged.
func TestWithTransactionRollsBackOnError(t *testing.T) {
	ctx := testutil.UseTestDB(t)
	coll := database.DB.Collection("tx_check")

	errStop := errors.New("stop")
	err := database.WithTransaction(ctx, func(ctx context.Context) error {
		if _, err := coll.InsertOne(ctx, bson.M{"n": 1}); err != nil {
			return err
		}
		return errStop
	})
	if !errors.Is(err, errStop) {
		t.Fatalf("WithTransaction returned %v, want %v", err, errStop)
	}

	if err := database.WithTransaction(ctx, func(ctx context.Context) error {
		_, err := coll.InsertOne(ctx, bson.M{"n": 2})
		return err
	}); err != nil {
		t.Fatalf("committed transaction: %v", err)
	}

	n, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("collection holds %d documents, want 1 (the aborted insert must not stay)", n)
	}
}
