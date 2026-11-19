package database

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

// ErrNoTransactions means the MongoDB server is standalone, so multi-document
// transactions fail. It must be a replica set (as Atlas is) or a sharded cluster.
var ErrNoTransactions = errors.New("MongoDB is not a replica set, so transactions don't work: run it as one (see _docs/getting-started.md)")

// RequireTransactions returns ErrNoTransactions unless the server behind DB
// supports multi-document transactions (#37).
func RequireTransactions(ctx context.Context) error {
	var hello struct {
		SetName string `bson:"setName"`
		Msg     string `bson:"msg"`
	}
	if err := DB.Client().Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		return fmt.Errorf("ask MongoDB what it is: %w", err)
	}
	if hello.SetName == "" && hello.Msg != "isdbgrid" {
		return ErrNoTransactions
	}
	return nil
}

// WithTransaction runs fn in one multi-document transaction: its writes commit
// together or not at all (#37). Every database call in fn must use the context
// fn is given, or it runs outside the transaction.
//
// fn can run more than once: when another transaction writes the same
// document first, MongoDB aborts this one and the driver runs fn again. So
// fn must do its reads and checks too, not only the writes, and must have no
// effect outside the database. fn's own error aborts the transaction and is
// returned as is.
func WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	sess, err := DB.Client().StartSession()
	if err != nil {
		return fmt.Errorf("start MongoDB session: %w", err)
	}
	defer sess.EndSession(context.WithoutCancel(ctx))

	opts := options.Transaction().
		SetReadConcern(readconcern.Snapshot()).
		SetWriteConcern(writeconcern.Majority())
	_, err = sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return nil, fn(ctx)
	}, opts)
	return err
}
