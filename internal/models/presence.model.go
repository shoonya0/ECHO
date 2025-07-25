package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ PRESENCE MODELS ============

// PresenceStatus represents comprehensive user presence information
type PresenceStatus struct {
	UserID   string `json:"user_id" bson:"user_id"`
	IsOnline bool   `json:"is_online" bson:"is_online"`
	Status   string `json:"status" bson:"status"` // "online", "away", "dnd", "invisible"
	// CustomStatus string    `json:"custom_status,omitempty" bson:"custom_status,omitempty"` // Custom status message
	LastSeen time.Time `json:"last_seen" bson:"last_seen"`
	// LastActivity time.Time `json:"last_activity" bson:"last_activity"`                 // Last activity timestamp
	DeviceInfo *string   `json:"device_info,omitempty" bson:"device_info,omitempty"` // Device information
	Location   *string   `json:"location,omitempty" bson:"location,omitempty"`       // Location if shared
	CreatedAt  time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" bson:"updated_at"`
}

// PresenceEvent represents an event to be published via Redis pub/sub
type PresenceEvent struct {
	Type      string         `json:"type"` // "presence_update", "custom_status_update", "user_login", "user_logout"
	UserID    string         `json:"user_id"`
	Presence  PresenceStatus `json:"presence"`
	Timestamp time.Time      `json:"timestamp"`
	EventID   string         `json:"event_id"` // Unique event identifier
}

// PresenceHistory represents historical presence data
type PresenceHistory struct {
	ID        bson.ObjectID  `json:"id" bson:"_id,omitempty"`
	UserID    string         `json:"user_id" bson:"user_id"`
	Status    string         `json:"status" bson:"status"`
	Presence  PresenceStatus `json:"presence" bson:"presence"`
	Timestamp time.Time      `json:"timestamp" bson:"timestamp"`
	Duration  *time.Duration `json:"duration,omitempty" bson:"duration,omitempty"` // How long they were in this status
}
