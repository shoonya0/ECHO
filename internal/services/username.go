package services

import (
	"context"
	"fmt"
	"strconv"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/utils"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// CheckNewUsername checks a username someone wants to take, at signup or in a
// profile change (#36): it must be normalized (utils.NormalizeUsername), have the
// format of utils.ValidUsername, and not be any other account's name in any case.
// self is the account taking it, so changing only the case of one's own name
// works; signup passes a zero ID. Returns ErrInvalidUsername or ErrUsernameTaken.
// Two requests can both pass the check; the username_ci index refuses the second
// write, and callers map that to ErrUsernameTaken too.
func CheckNewUsername(ctx context.Context, username string, self bson.ObjectID) error {
	if !utils.ValidUsername(username) {
		return ErrInvalidUsername
	}
	taken, err := usernameTaken(ctx, username, self)
	if err != nil {
		return err
	}
	if taken {
		return ErrUsernameTaken
	}
	return nil
}

// usernameFilter matches accounts other than self named username. It states the
// $type of username_ci's partial filter, without which the planner can't use the
// index and scans every user (code review of #36).
func usernameFilter(username string, self bson.ObjectID) bson.M {
	return bson.M{"username": bson.M{"$eq": username, "$type": "string"}, "_id": bson.M{"$ne": self}}
}

// usernameTaken reports whether an account other than self has the name in any case.
func usernameTaken(ctx context.Context, username string, self bson.ObjectID) (bool, error) {
	n, err := database.DB.Collection(string(constants.UserColl)).CountDocuments(ctx,
		usernameFilter(username, self),
		options.Count().SetCollation(database.UsernameCollation).SetLimit(1))
	if err != nil {
		return false, fmt.Errorf("check username: %w", err)
	}
	return n > 0, nil
}

// UsernameRename is one account RenameDuplicateUsernames renamed.
type UsernameRename struct {
	UserID bson.ObjectID
	From   string
	To     string
}

// maxRenameSuffix bounds the search for a free name, so a run always ends.
const maxRenameSuffix = 1000

// RenameDuplicateUsernames gives every account a username no other account has in
// any case, so the unique username_ci index can be built (#36). Of the accounts
// sharing a name, the oldest (smallest _id) keeps it; each later one gets the
// first free name of the form <name>_2, <name>_3, ... It runs on every start before
// the indexes, since an older server copy can still sign up a taken name during a
// deploy. Once the index exists no duplicate can be written, so the run stops there
// without reading the users (code review of #36). Names are grouped by MongoDB with
// the index's collation, so the migration and the index agree on what a duplicate
// is. Each rename is filtered on the old name, so a change in between isn't
// overwritten. Two servers starting together before the index exists can both pick
// one free <name>_<n> for different accounts; the index build then fails and is
// logged, and the next start renames the pair. The copies of the old name in chats
// and messages stay as they are, like those of any profile change (#36). A failure on
// one account stops the run; the next start continues. Returns the renames done.
func RenameDuplicateUsernames(ctx context.Context) ([]UsernameRename, error) {
	users := database.DB.Collection(string(constants.UserColl))
	specs, err := users.Indexes().ListSpecifications(ctx)
	if err != nil {
		return nil, fmt.Errorf("list user indexes: %w", err)
	}
	for _, s := range specs {
		if s.Name == database.UsernameIndex {
			return nil, nil
		}
	}
	cur, err := users.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"username": bson.M{"$type": "string"}}}},
		{{Key: "$sort", Value: bson.M{"_id": 1}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$username",
			"users": bson.M{"$push": bson.M{"id": "$_id", "name": "$username"}},
		}}},
		{{Key: "$match", Value: bson.M{"users.1": bson.M{"$exists": true}}}},
	}, options.Aggregate().SetCollation(database.UsernameCollation))
	if err != nil {
		return nil, fmt.Errorf("find duplicate usernames: %w", err)
	}
	var groups []struct {
		Users []struct {
			ID   bson.ObjectID `bson:"id"`
			Name string        `bson:"name"`
		} `bson:"users"`
	}
	if err := cur.All(ctx, &groups); err != nil {
		return nil, fmt.Errorf("read duplicate usernames: %w", err)
	}

	var renames []UsernameRename
	for _, g := range groups {
		for _, u := range g.Users[1:] { // the oldest keeps the name
			to, err := renameToFreeName(ctx, u.ID, u.Name)
			if err != nil {
				return renames, err
			}
			if to != "" {
				renames = append(renames, UsernameRename{UserID: u.ID, From: u.Name, To: to})
			}
		}
	}
	return renames, nil
}

// renameToFreeName renames one account to the first free <name>_<n>. It returns ""
// when the account no longer has the name (changed or deleted meanwhile).
func renameToFreeName(ctx context.Context, id bson.ObjectID, name string) (string, error) {
	users := database.DB.Collection(string(constants.UserColl))
	for n := 2; n <= maxRenameSuffix; n++ {
		candidate := name + "_" + strconv.Itoa(n)
		taken, err := usernameTaken(ctx, candidate, id)
		if err != nil {
			return "", err
		}
		if taken {
			continue
		}
		res, err := users.UpdateOne(ctx,
			bson.M{"_id": id, "username": name},
			bson.M{"$set": bson.M{"username": candidate}})
		if database.IsDuplicateKeyOn(err, database.UsernameIndex) {
			continue // taken since the check
		}
		if err != nil {
			return "", fmt.Errorf("rename user %s: %w", id.Hex(), err)
		}
		if res.MatchedCount == 0 {
			return "", nil
		}
		return candidate, nil
	}
	return "", fmt.Errorf("no free username for user %s after %s_%d", id.Hex(), name, maxRenameSuffix)
}
