package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/logger"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// EmailCollation compares emails without regard to case (#36). Signup and login
// query users.email with it, so an email stored with capitals before #36 still
// matches, whether or not the startup migration has lowered it yet. The
// email_ci index has the same collation, so those queries can use it.
var EmailCollation = &options.Collation{Locale: "en", Strength: 2}

// UsernameCollation compares usernames without regard to case (#36): Alice and
// alice are one name. Queries that check whether a name is taken use it, so they
// agree with the username_ci index.
var UsernameCollation = &options.Collation{Locale: "en", Strength: 2}

// UsernameIndex is the unique, case-insensitive username index. A write refused
// by it means the name is taken (IsDuplicateKeyOn).
const UsernameIndex = "username_ci"

// IsDuplicateKeyOn reports whether err is a duplicate-key error from the named
// index, so a caller can tell a taken username from a taken email.
func IsDuplicateKeyOn(err error, index string) bool {
	return mongo.IsDuplicateKeyError(err) && strings.Contains(err.Error(), "index: "+index+" ")
}

// collectionIndexes is the set of secondary indexes one collection needs.
type collectionIndexes struct {
	coll   constants.Collection
	models []mongo.IndexModel
}

// indexSpecs lists the secondary indexes each collection needs, keyed by the
// queries that use them. It is a slice so collections are always built in the
// same order. Most chats lookups filter on _id or on the dynamic
// participants.<userID> keys, which cannot be indexed with a fixed key.
var indexSpecs = []collectionIndexes{
	{
		coll: constants.ChatColl,
		models: []mongo.IndexModel{
			// Invite routes find the group by its stored code (#15). Not unique:
			// codes are 16 random bytes, and most chats have no codes at all.
			{
				Keys:    bson.D{{Key: "inviteCode.inviteCode", Value: 1}},
				Options: options.Index().SetName("inviteCode.inviteCode_1"),
			},
		},
	},
	{
		// Its own entry, so chats from before #37 that would break it can't stop
		// the invite-code index.
		coll: constants.ChatColl,
		models: []mongo.IndexModel{
			// One direct chat per pair (#37). Partial: groups and direct chats
			// the startup backfill hasn't keyed yet have no key.
			{
				Keys: bson.D{{Key: "directKey", Value: 1}},
				Options: options.Index().SetName("directKey_1").SetUnique(true).
					SetPartialFilterExpression(bson.M{"directKey": bson.M{"$type": "string"}}),
			},
		},
	},
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
	{
		// Its own entry, so duplicate emails failing email_1 can't stop it.
		coll: constants.UserColl,
		models: []mongo.IndexModel{
			// Signup and login match emails ignoring case (#36). Not unique: emails
			// stored before #36 can differ only in case, and that mustn't stop the
			// build. Uniqueness comes from storing emails lower case under email_1.
			{
				Keys:    bson.D{{Key: "email", Value: 1}},
				Options: options.Index().SetName("email_ci").SetCollation(EmailCollation),
			},
		},
	},
	{
		// Its own entry, so duplicate emails failing email_1 can't stop it.
		coll: constants.UserColl,
		models: []mongo.IndexModel{
			// One account per username, ignoring case (#36). The startup
			// RenameDuplicateUsernames runs first, so names shared before #36
			// can't stop the build. Partial: documents without a username
			// (test fixtures, broken seeds) aren't all one "missing" name.
			{
				Keys: bson.D{{Key: "username", Value: 1}},
				Options: options.Index().SetName(UsernameIndex).SetUnique(true).
					SetCollation(UsernameCollation).
					SetPartialFilterExpression(bson.M{"username": bson.M{"$type": "string"}}),
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
