package realtime

import (
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// UserStatus is a presence status such as "online" or "offline".
type UserStatus string

// UserPresence is the in-memory presence record for a connected user.
type UserPresence struct {
	UserID     bson.ObjectID `json:"user_id"`
	Status     UserStatus    `json:"status"`
	LastSeen   time.Time     `json:"last_seen"`
	ClientID   string        `json:"client_id"`
	DeviceInfo string        `json:"device_info,omitempty"`
}

// PresenceManager keeps presence for users connected to this instance.
type PresenceManager struct {
	presence map[bson.ObjectID]UserPresence
	mutex    sync.RWMutex
}

var (
	presenceInstance *PresenceManager
	presenceOnce     sync.Once
)

// GetPresenceInstance returns the process-wide presence manager.
func GetPresenceInstance() *PresenceManager {
	presenceOnce.Do(func() {
		presenceInstance = &PresenceManager{presence: make(map[bson.ObjectID]UserPresence)}
	})
	return presenceInstance
}

func (pm *PresenceManager) Get(userID bson.ObjectID) (UserPresence, bool) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()
	presence, exists := pm.presence[userID]
	return presence, exists
}

// Set stores presence for a user, stamping LastSeen with the current time.
func (pm *PresenceManager) Set(user UserPresence) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	user.LastSeen = time.Now()
	pm.presence[user.UserID] = user
}

func (pm *PresenceManager) Delete(userID bson.ObjectID) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	delete(pm.presence, userID)
}
