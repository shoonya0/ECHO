package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/sirupsen/logrus"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func contains(slice []bson.ObjectID, item bson.ObjectID) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

// logDBError logs a failed database call. A conflict inside a transaction is
// expected under load: the driver runs the transaction again, so it is logged
// at debug here. If the retries run out, database.WithTransaction logs the
// final error (#37).
func logDBError(log *logrus.Entry, err error, msg string) {
	var le mongo.LabeledError
	if errors.As(err, &le) && le.HasErrorLabel("TransientTransactionError") {
		log.WithError(err).Debug(msg + "; the transaction runs again")
		return
	}
	log.WithError(err).Error(msg)
}

// writeInIDOrder runs writeA (to user a) and writeB (to user b), the one for the
// smaller ID first. Two transactions that write the same two users then
// collide at the first write, and only the later one runs again. In the other
// order each held one user and aborted the other, again and again (#37).
func writeInIDOrder(a, b bson.ObjectID, writeA, writeB func() error) error {
	if bytes.Compare(b[:], a[:]) < 0 {
		writeA, writeB = writeB, writeA
	}
	if err := writeA(); err != nil {
		return err
	}
	return writeB()
}

// FindByID decodes the single document matching filter into T.
func FindByID[T any](
	ctx context.Context,
	collection *mongo.Collection,
	filter bson.M,
	projection bson.M,
) (*T, error) {
	// logger
	log := logger.WithContext(ctx)
	log.WithField("collection", collection.Name()).Debug("Finding document by ID")

	var result T
	options := options.FindOne()

	if len(projection) > 0 {
		options.SetProjection(projection)
	}

	err := collection.FindOne(ctx, filter, options).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Debug("Document not found")
		} else {
			logDBError(log, err, "Failed to find document")
		}
		return nil, err
	}

	log.Debug("Document found successfully")
	return &result, nil
}

// UpdateOne applies update to the first document matching filter.
func UpdateOne[T any](
	ctx context.Context,
	coll *mongo.Collection,
	filter bson.M,
	update T,
	opts ...options.Lister[options.UpdateOneOptions],
) (*mongo.UpdateResult, error) {
	log := logger.WithContext(ctx)
	log.WithField("collection", coll.Name()).Debug("Updating document")

	res, err := coll.UpdateOne(ctx, filter, update, opts...)
	if err != nil {
		logDBError(log, err, "Failed to update document")
		return nil, fmt.Errorf("failed to update document: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"matched_count":  res.MatchedCount,
		"modified_count": res.ModifiedCount,
		"upserted_id":    res.UpsertedID,
	}).Debug("Document updated successfully")

	return res, nil
}
