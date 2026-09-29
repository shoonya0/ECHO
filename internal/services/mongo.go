package services

import (
	"context"
	"fmt"

	"github.com/shoonya0/ECHO/internal/logger"

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
			log.WithError(err).Error("Failed to find document")
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
		log.WithError(err).Error("Failed to update document")
		return nil, fmt.Errorf("failed to update document: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"matched_count":  res.MatchedCount,
		"modified_count": res.ModifiedCount,
		"upserted_id":    res.UpsertedID,
	}).Debug("Document updated successfully")

	return res, nil
}
