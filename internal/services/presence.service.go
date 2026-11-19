package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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

// Presence has two sources (#31). presence.status in MongoDB is the status the
// user chose (online, away, dnd, invisible). Whether they are connected comes
// from the Redis sorted set online:<userID>, which every server holding them
// renews. presence.isOnline in MongoDB is never read: REST answers compute it.

// presenceReadTimeout bounds the Redis read, so a slow Redis can't hold a
// REST request for long.
const presenceReadTimeout = time.Second

// ConnectedUsers reports which of userIDs at least one server holds now. A
// server's mark counts until it expires, so a server that stopped without
// removing its marks stops counting within the mark's lifetime.
func ConnectedUsers(ctx context.Context, userIDs []bson.ObjectID) (map[bson.ObjectID]bool, error) {
	connected := make(map[bson.ObjectID]bool, len(userIDs))
	if len(userIDs) == 0 {
		return connected, nil
	}
	if database.Redis == nil {
		return connected, errors.New("connected users: redis not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, presenceReadTimeout)
	defer cancel()

	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	pipe := database.Redis.Pipeline()
	counts := make([]*redis.IntCmd, len(userIDs))
	for i, id := range userIDs {
		// Marks scored after now haven't expired.
		counts[i] = pipe.ZCount(ctx, constants.OnlineKeyPrefix+id.Hex(), "("+now, "+inf")
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return connected, fmt.Errorf("connected users: %w", err)
	}
	for i, id := range userIDs {
		if counts[i].Val() > 0 {
			connected[id] = true
		}
	}
	return connected, nil
}

// connectedOrNone is ConnectedUsers for REST reads: when Redis can't answer,
// everyone is shown offline, which is better than failing the request.
func connectedOrNone(ctx context.Context, userIDs []bson.ObjectID) map[bson.ObjectID]bool {
	connected, err := ConnectedUsers(ctx, userIDs)
	if err != nil {
		logger.WithContext(ctx).WithError(err).Warn("Could not read who is online; showing users as offline")
	}
	return connected
}

// chosenStatus is the status the user picked, "online" when they never did.
func chosenStatus(p models.PresenceEmbed) constants.UserStatus {
	if p.Status == "" {
		return constants.UserStatusOnline
	}
	return constants.UserStatus(p.Status)
}

// ShowPresence turns stored presence into what a viewer sees. The user
// themselves sees their chosen status and whether they are connected. Others
// see the chosen status only while the user is connected and not invisible;
// otherwise "offline", with lastSeen unless the user chose to hide (invisible
// or offline).
func ShowPresence(p *models.PresenceEmbed, connected, self bool) {
	status := chosenStatus(*p)
	if self {
		p.Status = string(status)
		p.IsOnline = connected
		return
	}
	hiding := status == constants.UserStatusInvisible || status == constants.UserStatusOffline
	if connected && !hiding {
		p.Status = string(status)
		p.IsOnline = true
		return
	}
	p.Status = string(constants.UserStatusOffline)
	p.IsOnline = false
	p.LastActivity = time.Time{}
	if hiding {
		p.LastSeen = time.Time{}
	}
}

// ShowOwnPresence fills in whether userID is connected on their own presence.
func ShowOwnPresence(ctx context.Context, userID bson.ObjectID, p *models.PresenceEmbed) {
	connected := connectedOrNone(ctx, []bson.ObjectID{userID})
	ShowPresence(p, connected[userID], true)
}

// RecordLastSeen stores at as the time the user's last connection closed. It
// never moves lastSeen back, so two disconnects saved out of order keep the
// later time. A deleted user matches nothing, which isn't an error.
func RecordLastSeen(ctx context.Context, userID bson.ObjectID, at time.Time) error {
	_, err := database.DB.Collection(string(constants.UserColl)).UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$max": bson.M{"presence.lastSeen": at}},
	)
	if err != nil {
		return fmt.Errorf("record last seen: %w", err)
	}
	return nil
}

// GetPresenceStatus returns the status userID chose, read from MongoDB rather
// than from their token, which can be days old (#31).
func GetPresenceStatus(ctx context.Context, userID bson.ObjectID) (constants.UserStatus, error) {
	var user struct {
		Presence models.PresenceEmbed `bson:"presence"`
	}
	err := database.DB.Collection(string(constants.UserColl)).FindOne(ctx,
		bson.M{"_id": userID},
		options.FindOne().SetProjection(bson.M{"presence.status": 1}),
	).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", fmt.Errorf("%w: %s", ErrUserNotFound, userID.Hex())
		}
		return "", fmt.Errorf("get presence status: %w", err)
	}
	return chosenStatus(user.Presence), nil
}
