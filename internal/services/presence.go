package services

import (
	"context"
	"gin/objects"
	"sync"
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
	Status     UserStatus    `json:"status"` // "online", "away", "dnd", "invisible", "offline"
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
	mutex    sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

var (
	Presence            *PresenceManager
	PresenceManagerOnce sync.Once
)

func GetPresenceInstance() *PresenceManager {
	PresenceManagerOnce.Do(func() {
		Presence = &PresenceManager{
			presence: make(map[bson.ObjectID]UserPresence),
		}
		ctx, cancel := context.WithCancel(context.Background())
		Presence.ctx = ctx
		Presence.cancel = cancel
	})
	return Presence
}

func (pm *PresenceManager) Get(userID bson.ObjectID) (UserPresence, bool) {
	presence, exists := pm.presence[userID]
	return presence, exists
}

func (pm *PresenceManager) Set(user UserPresence) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	presence := UserPresence{
		UserID:     user.UserID,
		Status:     user.Status,
		LastSeen:   time.Now(),
		ClientID:   user.ClientID,
		DeviceInfo: user.DeviceInfo,
	}
	pm.presence[user.UserID] = presence
}

func (pm *PresenceManager) Update(userUpdate PresenceUpdate) bool {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	presence, exists := pm.Get(userUpdate.UserID)
	if !exists {
		return false
	}

	presence.Status = userUpdate.Status
	if presence.Status == UserStatus(objects.UserStatusOnline) {
		presence.LastSeen = time.Now()
	}

	pm.presence[presence.UserID] = presence
	return true
}

func (pm *PresenceManager) Delete(userID bson.ObjectID) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	delete(pm.presence, userID)
}
