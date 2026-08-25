// Package main provides a one-off maintenance tool to reactivate user
// accounts that were unintentionally deactivated by the PUT /profile/ bug.
//
// The bug wrote accountStatus.isActive=false (and zeroed isVerified/isBanned)
// for any user who performed a profile-only update. This tool fixes those
// documents while leaving genuinely banned accounts untouched.
//
// Usage:
//
//	go run ./tools/reactivate           # reactivate all deactivated, non-banned accounts
//	go run ./tools/reactivate -dry-run  # report only, no writes
//	go run ./tools/reactivate -id 6a7c1276c5fa0ceb4fa422ba  # target one user
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"gin/config"
	"gin/internal/db"
	"gin/logger"
	"gin/objects"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	configPath = flag.String("config", "", "Path to the configuration file directory")
	dryRun     = flag.Bool("dry-run", false, "Preview affected accounts without writing")
	userID     = flag.String("id", "", "Reactivate only the given user ObjectID")
)

func main() {
	flag.Parse()

	// Load the same configuration the server uses.
	config.New(*configPath)

	if err := logger.InitLogger("", logrus.DebugLevel); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)

	if err := db.ConnectDB(ctx); err != nil {
		log.WithError(err).Fatal("Failed to connect to MongoDB")
	}
	defer db.DisconnectDB(ctx)

	coll := objects.DB.Collection(string(objects.UserColl))

	filter := reactivationFilter(*userID)

	// Report phase.
	cursor, err := coll.Find(ctx, filter)
	if err != nil {
		log.WithError(err).Fatal("Failed to query deactivated accounts")
	}
	defer cursor.Close(ctx)

	type affected struct {
		ID           bson.ObjectID `bson:"_id"`
		Email        string        `bson:"email"`
		Username     string        `bson:"username"`
		AccountState bson.M        `bson:"accountStatus"`
	}
	var affectedAccounts []affected
	if err := cursor.All(ctx, &affectedAccounts); err != nil {
		log.WithError(err).Fatal("Failed to decode deactivated accounts")
	}

	if len(affectedAccounts) == 0 {
		log.Info("No deactivated (non-banned) accounts found to reactivate")
		return
	}

	log.WithField("count", len(affectedAccounts)).Info("Found deactivated accounts")
	for _, a := range affectedAccounts {
		log.WithFields(logrus.Fields{
			"user":    a.ID.Hex(),
			"email":   a.Email,
			"account": a.AccountState,
		}).Info("  affected")
	}

	if *dryRun {
		log.Warn("Dry run complete — no writes performed")
		return
	}

	// Reactivation phase.
	for _, a := range affectedAccounts {
		update := bson.M{
			"$set": bson.M{
				"accountStatus.isActive": true,
			},
		}
		res, err := coll.UpdateOne(ctx, bson.M{"_id": a.ID}, update)
		if err != nil {
			log.WithError(err).WithField("user", a.ID.Hex()).Error("Failed to reactivate account")
			continue
		}
		if res.MatchedCount == 0 {
			log.WithField("user", a.ID.Hex()).Warn("No matching document during reactivation")
			continue
		}
		log.WithField("user", a.ID.Hex()).Info("Reactivated account (accountStatus.isActive=true)")
	}

	log.WithField("count", len(affectedAccounts)).Info("Reactivation complete")
}

// reactivationFilter builds the filter for accounts to reactivate.
//
// It targets documents where isActive is false and isBanned is false. A real
// ban keeps isBanned=true, so banned accounts are intentionally left alone.
func reactivationFilter(id string) bson.M {
	base := bson.M{
		"accountStatus.isActive": false,
		"accountStatus.isBanned": bson.M{"$ne": true},
	}
	if id == "" {
		return base
	}

	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid -id ObjectID: %v\n", err)
		os.Exit(1)
	}
	return bson.M{
		"_id":                    oid,
		"accountStatus.isActive": false,
		"accountStatus.isBanned": bson.M{"$ne": true},
	}
}
