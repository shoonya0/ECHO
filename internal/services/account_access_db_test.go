package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/testutil"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// withoutRedis runs the test with no Redis client, so only MongoDB is read.
func withoutRedis(t *testing.T) {
	t.Helper()
	prev := database.Redis
	database.Redis = nil
	t.Cleanup(func() { database.Redis = prev })
}

func insertAccount(t *testing.T, ctx context.Context, doc bson.M) bson.ObjectID {
	t.Helper()
	id := bson.NewObjectID()
	doc["_id"] = id
	doc["username"] = "user-" + id.Hex()
	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(ctx, doc); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func setAccountStatus(t *testing.T, ctx context.Context, id bson.ObjectID, status bson.M) {
	t.Helper()
	res, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx,
		bson.M{"_id": id}, bson.M{"$set": bson.M{"accountStatus": status}})
	if err != nil || res.MatchedCount != 1 {
		t.Fatalf("set accountStatus: matched %v, err %v", res, err)
	}
}

var (
	activeStatus   = bson.M{"isActive": true, "isVerified": true, "isBanned": false}
	bannedStatus   = bson.M{"isActive": true, "isVerified": true, "isBanned": true}
	inactiveStatus = bson.M{"isActive": false, "isVerified": true, "isBanned": false}
)

// The token carried the account status from login for 7 days, so a ban, a
// deactivation or a deleted account didn't take effect (#33). The status must come
// from the stored account, with the same rules as Login.
func TestCheckAccountAccessReadsStoredAccount(t *testing.T) {
	ctx := useTestDB(t)
	withoutRedis(t)

	cases := []struct {
		name string
		doc  bson.M
		want error
	}{
		{"active", bson.M{"accountStatus": activeStatus}, nil},
		{"active but not verified", bson.M{"accountStatus": bson.M{"isActive": true}}, nil},
		{"no accountStatus (legacy account, as Login allows)", bson.M{}, nil},
		{"accountStatus null (Login refuses it as inactive)", bson.M{"accountStatus": nil}, ErrAccountInactive},
		{"banned", bson.M{"accountStatus": bannedStatus}, ErrAccountBanned},
		{"inactive", bson.M{"accountStatus": inactiveStatus}, ErrAccountInactive},
		{"banned and inactive", bson.M{"accountStatus": bson.M{"isActive": false, "isBanned": true}}, ErrAccountBanned},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := insertAccount(t, ctx, tc.doc)
			if err := CheckAccountAccess(ctx, id); !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
				t.Errorf("CheckAccountAccess = %v, want %v", err, tc.want)
			}
		})
	}

	t.Run("deleted", func(t *testing.T) {
		if err := CheckAccountAccess(ctx, bson.NewObjectID()); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("CheckAccountAccess = %v, want ErrUserNotFound", err)
		}
	})
}

// The result is cached so most requests skip MongoDB, but only for a short time,
// so a ban made by editing the database takes effect within AccountAccessCacheTTL.
func TestCheckAccountAccessCacheIsShortLived(t *testing.T) {
	ctx := useTestDB(t)
	rdb := testutil.UseTestRedis(t)

	id := insertAccount(t, ctx, bson.M{"accountStatus": activeStatus})
	key := accountAccessKey(id)
	t.Cleanup(func() { rdb.Del(context.Background(), key) })

	if err := CheckAccountAccess(ctx, id); err != nil {
		t.Fatalf("first check: %v", err)
	}
	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 || ttl > AccountAccessCacheTTL {
		t.Fatalf("cache TTL = %v, want between 0 and %v", ttl, AccountAccessCacheTTL)
	}
	if AccountAccessCacheTTL > time.Minute {
		t.Errorf("AccountAccessCacheTTL = %v; a ban must take effect within a minute", AccountAccessCacheTTL)
	}

	setAccountStatus(t, ctx, id, bannedStatus)
	if err := CheckAccountAccess(ctx, id); err != nil {
		t.Fatalf("check while cached = %v, want the cached nil", err)
	}

	rdb.Del(ctx, key) // what expiry does
	if err := CheckAccountAccess(ctx, id); !errors.Is(err, ErrAccountBanned) {
		t.Errorf("check after expiry = %v, want ErrAccountBanned", err)
	}
	if err := CheckAccountAccess(ctx, id); !errors.Is(err, ErrAccountBanned) {
		t.Errorf("cached refusal = %v, want ErrAccountBanned", err)
	}
}

// Deleting your profile must lock the token out at once, not after the cache expires,
// even if a request that read the account just before the delete writes its "ok"
// into the cache just after it.
func TestDeleteProfileLocksOutAtOnce(t *testing.T) {
	ctx := useTestDB(t)
	rdb := testutil.UseTestRedis(t)

	id := insertAccount(t, ctx, bson.M{"accountStatus": activeStatus})
	key := accountAccessKey(id)
	t.Cleanup(func() { rdb.Del(context.Background(), key) })

	if err := CheckAccountAccess(ctx, id); err != nil {
		t.Fatalf("check before delete: %v", err)
	}
	if err := DeleteProfile(ctx, id.Hex()); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if err := CheckAccountAccess(ctx, id); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("check after delete = %v, want ErrUserNotFound", err)
	}

	// The in-flight request's cache write, landing after the delete.
	cacheAccountAccess(ctx, key, accessAllowed)
	if err := CheckAccountAccess(ctx, id); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("check after a late \"ok\" write = %v, want ErrUserNotFound", err)
	}
}

// The logout list is read through IsTokenRevoked, with the key LogoutUser writes.
// A Redis error must come back as an error, never as "not revoked".
func TestIsTokenRevoked(t *testing.T) {
	ctx := context.Background()
	rdb := testutil.UseTestRedis(t)

	jti := "test-" + bson.NewObjectID().Hex()
	t.Cleanup(func() { rdb.Del(context.Background(), tokenBlacklistKey(jti)) })

	if revoked, err := IsTokenRevoked(ctx, jti); err != nil || revoked {
		t.Fatalf("before logout: revoked=%v err=%v, want false, nil", revoked, err)
	}
	if err := LogoutUser(ctx, jti, time.Now().Add(time.Minute)); err != nil {
		t.Fatalf("LogoutUser: %v", err)
	}
	if revoked, err := IsTokenRevoked(ctx, jti); err != nil || !revoked {
		t.Errorf("after logout: revoked=%v err=%v, want true, nil", revoked, err)
	}

	testutil.UseBrokenRedis(t)
	if _, err := IsTokenRevoked(ctx, jti); err == nil {
		t.Error("with Redis down: err = nil, want an error")
	}
}

// When Redis fails, the check reads MongoDB instead of letting the request through.
func TestCheckAccountAccessFallsBackToMongoWhenRedisFails(t *testing.T) {
	ctx := useTestDB(t)
	testutil.UseBrokenRedis(t)

	active := insertAccount(t, ctx, bson.M{"accountStatus": activeStatus})
	banned := insertAccount(t, ctx, bson.M{"accountStatus": bannedStatus})

	if err := CheckAccountAccess(ctx, active); err != nil {
		t.Errorf("active account with Redis down = %v, want nil", err)
	}
	if err := CheckAccountAccess(ctx, banned); !errors.Is(err, ErrAccountBanned) {
		t.Errorf("banned account with Redis down = %v, want ErrAccountBanned", err)
	}
}
