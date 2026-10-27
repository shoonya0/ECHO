package database

import (
	"context"
	"fmt"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/logger"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// indexSpecs lists the secondary indexes each collection needs, keyed by the
// queries that use them. The chats collection has none: its lookups either
// filter on _id or on the dynamic participants.<userID> keys, which cannot be
// indexed with a fixed key.
var indexSpecs = map[constants.Collection][]mongo.IndexModel{
	constants.MessageColl: {
		// Chat history: find {chatId} sorted by _id desc, paged by _id < cursor.
		{
			Keys:    bson.D{{Key: "chatId", Value: 1}, {Key: "_id", Value: -1}},
			Options: options.Index().SetName("chatId_1__id_-1"),
		},
	},
	constants.UserColl: {
		// Login and signup look users up by email; emails must be unique.
		{
			Keys:    bson.D{{Key: "email", Value: 1}},
			Options: options.Index().SetName("email_1").SetUnique(true),
		},
		// User suggestions sort by newest first.
		{
			Keys:    bson.D{{Key: "createdAt", Value: -1}},
			Options: options.Index().SetName("createdAt_-1"),
		},
	},
}

// EnsureIndexes creates the indexes in indexSpecs. Creating an index that
// already exists with the same keys and options is a no-op, so this is safe
// to run on every startup.
func EnsureIndexes(ctx context.Context) error {
	log := logger.WithContext(ctx)

	for coll, models := range indexSpecs {
		names, err := DB.Collection(string(coll)).Indexes().CreateMany(ctx, models)
		if err != nil {
			return fmt.Errorf("failed to create indexes on %s: %w", coll, err)
		}
		log.WithField("collection", string(coll)).WithField("indexes", names).Info("Indexes ensured")
	}
	return nil
}
