package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/models"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// AccountAccessCacheTTL is how long an account check is cached in Redis. A ban or
// deactivation made by editing the database takes effect within this time. Deleting
// a profile through the API marks the cache entry deleted, so it takes effect at once.
const AccountAccessCacheTTL = 30 * time.Second

// Values stored in the cache, one per outcome of CheckAccountAccess.
const (
	accessAllowed  = "ok"
	accessBanned   = "banned"
	accessInactive = "inactive"
	accessMissing  = "missing"
)

func accountAccessKey(userID bson.ObjectID) string {
	return "auth:account:" + userID.Hex()
}

// CheckAccountAccess says whether the stored account may still use its token. It
// returns nil, ErrUserNotFound (deleted), ErrAccountBanned or ErrAccountInactive,
// with the same rules as Login: a missing accountStatus counts as active, a null one
// as inactive, and isVerified isn't required. Any other error means the account couldn't be read.
//
// The result is cached in Redis for AccountAccessCacheTTL. If Redis fails, MongoDB
// is read instead, so a Redis outage never lets a banned account through.
func CheckAccountAccess(ctx context.Context, userID bson.ObjectID) error {
	log := logger.WithContext(ctx)
	key := accountAccessKey(userID)

	if database.Redis != nil {
		cached, err := database.Redis.Get(ctx, key).Result()
		switch {
		case err == nil:
			if ok, accessErr := accessFromCache(cached); ok {
				return accessErr
			}
			log.WithField("value", cached).Warn("Unknown account access cache value; reading MongoDB")
		case errors.Is(err, redis.Nil):
			// Not cached yet.
		default:
			log.WithError(err).Warn("Account access cache read failed; reading MongoDB")
		}
	}

	value, err := loadAccountAccess(ctx, userID)
	if err != nil {
		return err
	}

	// SET NX: never overwrite an entry written meanwhile, such as the "missing"
	// entry DeleteProfile writes while this request was reading the account.
	cacheAccountAccess(ctx, key, value)

	_, accessErr := accessFromCache(value)
	return accessErr
}

func cacheAccountAccess(ctx context.Context, key, value string) {
	if database.Redis == nil {
		return
	}
	if err := database.Redis.SetNX(ctx, key, value, AccountAccessCacheTTL).Err(); err != nil {
		logger.WithContext(ctx).WithError(err).Warn("Account access cache write failed")
	}
}

// MarkAccountDeleted caches the account as deleted, so its token stops working at
// once rather than when a cached "ok" expires. Clearing the entry instead would let
// a request that read the account just before the delete write "ok" back. It is best
// effort: if Redis fails, the entry expires on its own.
func MarkAccountDeleted(ctx context.Context, userID bson.ObjectID) {
	if database.Redis == nil {
		return
	}
	if err := database.Redis.Set(ctx, accountAccessKey(userID), accessMissing, AccountAccessCacheTTL).Err(); err != nil {
		logger.WithContext(ctx).WithError(err).Warn("Failed to mark the account deleted in the access cache")
	}
}

func loadAccountAccess(ctx context.Context, userID bson.ObjectID) (string, error) {
	raw, err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx,
		bson.M{"_id": userID},
		options.FindOne().SetProjection(bson.M{"accountStatus": 1}),
	).Raw()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return accessMissing, nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to read account status: %w", err)
	}

	// As in Login: a missing accountStatus (legacy account) counts as active, but a
	// stored null decodes to the zero value, which is inactive.
	field, err := raw.LookupErr("accountStatus")
	if err != nil {
		return accessAllowed, nil
	}
	var status models.AccountStatusEmbed
	if field.Type != bson.TypeNull {
		if err := field.Unmarshal(&status); err != nil {
			return "", fmt.Errorf("failed to decode account status: %w", err)
		}
	}
	switch {
	case status.IsBanned:
		return accessBanned, nil
	case !status.IsActive:
		return accessInactive, nil
	default:
		return accessAllowed, nil
	}
}

func accessFromCache(value string) (bool, error) {
	switch value {
	case accessAllowed:
		return true, nil
	case accessBanned:
		return true, ErrAccountBanned
	case accessInactive:
		return true, ErrAccountInactive
	case accessMissing:
		return true, ErrUserNotFound
	default:
		return false, nil
	}
}
