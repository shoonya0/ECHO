package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/logger"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// collectionIndexes is the set of secondary indexes one collection needs.
type collectionIndexes struct {
	coll   constants.Collection
	models []mongo.IndexModel
}

// indexSpecs lists the secondary indexes each collection needs, keyed by the
// queries that use them. It is a slice so collections are always built in the
// same order. The chats collection has none: its lookups either filter on _id
// or on the dynamic participants.<userID> keys, which cannot be indexed with a
// fixed key.
var indexSpecs = []collectionIndexes{
	{
		coll: constants.MessageColl,
		models: []mongo.IndexModel{
			// Chat history: find {chatId} sorted by _id desc, paged by _id < cursor.
			{
				Keys:    bson.D{{Key: "chatId", Value: 1}, {Key: "_id", Value: -1}},
				Options: options.Index().SetName("chatId_1__id_-1"),
			},
		},
	},
	{
		coll: constants.UserColl,
		models: []mongo.IndexModel{
			// Login and signup look users up by email; emails must be unique.
			{
				Keys:    bson.D{{Key: "email", Value: 1}},
				Options: options.Index().SetName("email_1").SetUnique(true),
			},
		},
	},
}

// EnsureIndexes creates the indexes in indexSpecs. Creating an index that
// already exists with the same keys and options is a no-op, so this is safe
// to run on every startup. A failure on one collection does not stop the
// others from being built; all failures are returned together.
func EnsureIndexes(ctx context.Context) error {
	log := logger.WithContext(ctx)

	var errs []error
	for _, spec := range indexSpecs {
		names, err := DB.Collection(string(spec.coll)).Indexes().CreateMany(ctx, spec.models)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to create indexes on %s: %w", spec.coll, err))
			continue
		}
		log.WithField("collection", string(spec.coll)).WithField("indexes", names).Info("Indexes ensured")
	}
	return errors.Join(errs...)
}
