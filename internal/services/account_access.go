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
// a profile through the API clears the cache entry, so it takes effect at once.
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
// with the same rules as Login: a missing accountStatus counts as active, and
// isVerified isn't required. Any other error means the account couldn't be read.
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

	if database.Redis != nil {
		if err := database.Redis.Set(ctx, key, value, AccountAccessCacheTTL).Err(); err != nil {
			log.WithError(err).Warn("Account access cache write failed")
		}
	}

	_, accessErr := accessFromCache(value)
	return accessErr
}

// ForgetAccountAccess removes the cached check, so the next request reads MongoDB.
// It is best effort: if Redis fails, the entry expires on its own.
func ForgetAccountAccess(ctx context.Context, userID bson.ObjectID) {
	if database.Redis == nil {
		return
	}
	if err := database.Redis.Del(ctx, accountAccessKey(userID)).Err(); err != nil {
		logger.WithContext(ctx).WithError(err).Warn("Failed to clear account access cache")
	}
}

func loadAccountAccess(ctx context.Context, userID bson.ObjectID) (string, error) {
	var doc struct {
		AccountStatus *models.AccountStatusEmbed `bson:"accountStatus"`
	}
	err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx,
		bson.M{"_id": userID},
		options.FindOne().SetProjection(bson.M{"accountStatus": 1}),
	).Decode(&doc)
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return accessMissing, nil
	case err != nil:
		return "", fmt.Errorf("failed to read account status: %w", err)
	case doc.AccountStatus == nil:
		return accessAllowed, nil // legacy account without accountStatus, as in Login
	case doc.AccountStatus.IsBanned:
		return accessBanned, nil
	case !doc.AccountStatus.IsActive:
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
