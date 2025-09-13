package services

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	// redis key
	UserPresenceKey = "presence:user:%s" // presence:user:{userID}
	// pubsub key
	ChatMembersKey    = "chat:members:%s" // chat:members:{chatID}
	UserChatsKey      = "user:chats:%s"   // user:chats:{userID}
	UserNotifyChannel = "notify:user:%s"  // notify:user:{userID}
	// UserInfoKey    = "user:info:%s"    // user:info:{userID}
	// ChatChannel       = "chat:%s"         // chat:{chatID}
	// PresenceChannel   = "presence"       // Global presence channel
)

const (
	PresenceTTL     = 5 * time.Minute
	UserInfoTTL     = 30 * time.Minute
	HeartbeatTTL    = 30 * time.Second
	CleanupInterval = 1 * time.Minute
)

type UserStatus string

type UserPresence struct {
	UserID     bson.ObjectID `json:"user_id"`
	Status     UserStatus    `json:"status"`
	LastSeen   time.Time     `json:"last_seen"`
	ClientID   string        `json:"client_id"`
	DeviceInfo string        `json:"device_info,omitempty"`
}

type PresenceUpdate struct {
	UserID   bson.ObjectID `json:"user_id"`
	Status   UserStatus    `json:"status"`
	LastSeen time.Time     `json:"last_seen"`
}

type PresenceManager struct {
	presence map[bson.ObjectID]UserPresence
}

var PresenceInstance *PresenceManager

func PresenceInstanceInit() *PresenceManager {
	if PresenceInstance != nil {
		return PresenceInstance
	}
	PresenceInstance = &PresenceManager{
		presence: make(map[bson.ObjectID]UserPresence),
	}
	return PresenceInstance
}

func (pm *PresenceManager) Get(userID bson.ObjectID) (UserPresence, error) {
	if pm.presence == nil {
		return UserPresence{}, fmt.Errorf("presence not found")
	}
	return pm.presence[userID], nil
}

func (pm *PresenceManager) Set(user UserPresence) error {
	presence := UserPresence{
		UserID:     user.UserID,
		Status:     user.Status,
		LastSeen:   time.Now(),
		ClientID:   user.ClientID,
		DeviceInfo: user.DeviceInfo,
	}
	pm.presence[user.UserID] = presence
	return nil
}

func (pm *PresenceManager) Update(userUpdate PresenceUpdate) error {
	if pm.presence == nil {
		return fmt.Errorf("presence not found")
	}

	presence, err := pm.Get(userUpdate.UserID)
	if err != nil {
		return fmt.Errorf("presence not found")
	}

	presence.Status = userUpdate.Status
	presence.LastSeen = time.Now()
	pm.presence[presence.UserID] = presence

	return nil
}

func (pm *PresenceManager) Delete(userID bson.ObjectID) error {
	if pm.presence == nil {
		return fmt.Errorf("user is not online")
	}
	delete(pm.presence, userID)
	return nil
}
