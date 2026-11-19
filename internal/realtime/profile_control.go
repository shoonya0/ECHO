package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// A profile change reaches every instance through ProfileControlChannel, so each one
// forgets the user's cached name and avatar and its next live event reads the new
// ones (#36). Like SessionControlChannel, it carries no client message. An instance
// that misses one shows the old name until its cache entry expires (5 minutes).

// ProfileControlChannel carries profileControl messages. Every instance subscribes.
const ProfileControlChannel = "control:profiles"

type profileControl struct {
	UserID string `json:"userId"`
	// Origin is the sending instance, which has already forgotten the entry and
	// ignores its own message.
	Origin string `json:"origin"`
}

// ProfileChanged forgets a user's cached name and avatar here and tells the other
// instances to do the same. Called after a profile update is saved.
func (eh *Hub) ProfileChanged(ctx context.Context, userID string) error {
	eh.pubSubManager.forgetUserInfo(userID)
	return eh.pubSubManager.publishProfileControl(ctx, profileControl{UserID: userID, Origin: eh.pubSubManager.instanceID})
}

// forgetUserInfo drops a user's cache entry on this instance and stops reads already
// in progress from caching what they read.
func (pm *PubSubManager) forgetUserInfo(userID string) {
	pm.hub.Mutex.Lock()
	delete(pm.hub.UserInfoCache, userID)
	delete(pm.hub.CacheExpiry, userID)
	pm.hub.UserInfoEpoch++
	pm.hub.Mutex.Unlock()
}

func (pm *PubSubManager) publishProfileControl(ctx context.Context, msg profileControl) error {
	if pm.redisClient == nil {
		return errors.New("redis client is not initialized")
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal profile control: %w", err)
	}
	if err := pm.redisClient.Publish(ctx, ProfileControlChannel, data).Err(); err != nil {
		return fmt.Errorf("failed to publish profile control: %w", err)
	}
	return nil
}

func (pm *PubSubManager) handleProfileControl(payload string) {
	var msg profileControl
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		pm.logger.WithError(err).Warn("Ignoring a profile control message that isn't JSON")
		return
	}
	if msg.UserID == "" {
		pm.logger.Warn("Ignoring a profile control message without a user")
		return
	}
	if msg.Origin == pm.instanceID {
		return // forgotten already, by ProfileChanged
	}
	pm.forgetUserInfo(msg.UserID)
}
