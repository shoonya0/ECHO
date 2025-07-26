package models

import (
	"time"
)

// ============ SEARCH USER MODEL ============
type SearchUser struct {
	ID            *string `json:"_id" bson:"_id"`
	Username      *string `json:"name" bson:"name"`
	Avatar        *string `json:"avatar" bson:"avatar"`
	DisplayName   *string `json:"displayName" bson:"displayName"`
	StatusMessage *string `json:"statusMessage" bson:"statusMessage"`
	IsVerified    *bool   `json:"isVerified" bson:"isVerified"`
}

// ============ REQUEST/RESPONSE MODELS FOR USER ============

type UserProfile struct {
	Username      *string `json:"username" bson:"username,omitempty"`
	Email         *string `json:"email" bson:"email,omitempty"`
	Phone         *string `json:"phone,omitempty" bson:"phone,omitempty"`
	Avatar        *string `json:"avatar,omitempty" bson:"avatar,omitempty"`
	DisplayName   *string `json:"displayName,omitempty" bson:"displayName,omitempty"`
	StatusMessage *string `json:"statusMessage,omitempty" bson:"statusMessage,omitempty"`
	IsVerified    *bool   `json:"isVerified,omitempty" bson:"isVerified,omitempty"`
}

// ============ REQUEST/RESPONSE MODELS FOR PRESENCE ============

// UpdatePresenceRequest represents a request to update presence
type UpdatePresenceRequest struct {
	Status       string  `json:"status" binding:"required"` // "online", "away", "dnd", "invisible"
	CustomStatus *string `json:"custom_status,omitempty"`
	DeviceInfo   *string `json:"device_info,omitempty"`
	Location     *string `json:"location,omitempty"`
}

// SetCustomStatusRequest represents a request to set custom status
type SetCustomStatusRequest struct {
	CustomStatus string `json:"custom_status" binding:"required"`
}

// UpdateAvailabilityRequest represents a request to update availability
type UpdateAvailabilityRequest struct {
	Status string `json:"status" binding:"required"` // "online", "away", "dnd", "invisible"
}

// PresenceResponse represents a presence response
type PresenceResponse struct {
	UserID       string    `json:"user_id"`
	IsOnline     bool      `json:"is_online"`
	Status       string    `json:"status"`
	CustomStatus string    `json:"custom_status,omitempty"`
	LastSeen     time.Time `json:"last_seen"`
	LastActivity time.Time `json:"last_activity"`
}

// PresenceHistoryResponse represents a presence history response
type PresenceHistoryResponse struct {
	History    []PresenceHistory `json:"history"`
	TotalCount int64             `json:"total_count"`
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
}

// Constants for presence status
const (
	StatusOnline    = "online"
	StatusAway      = "away"
	StatusDND       = "dnd" // Do Not Disturb
	StatusInvisible = "invisible"
	StatusOffline   = "offline"
)

// Constants for presence event types
const (
	EventPresenceUpdate     = "presence_update"
	EventCustomStatusUpdate = "custom_status_update"
	EventUserLogin          = "user_login"
	EventUserLogout         = "user_logout"
	EventUserActivity       = "user_activity"
)

// ============ REQUEST/RESPONSE MODELS FOR CHAT ============
type SendMessageRequest struct {
	ChatID      string  `json:"chat_id" binding:"required"`
	Content     string  `json:"content" binding:"required"`
	MessageType string  `json:"message_type"` // "text", "image", "file", "audio", "video"
	ParentID    *string `json:"parent_id,omitempty"`
	FileID      *string `json:"file_id,omitempty"`
}

type Pagination struct {
	Page      int    `json:"page"`
	Limit     int    `json:"limit"`
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`
}

// ============ TYPING INDICATOR MODEL ============
type TypingIndicator struct {
	ChatID    string    `json:"chat_id" bson:"chat_id"`
	UserID    string    `json:"user_id" bson:"user_id"`
	IsTyping  bool      `json:"is_typing" bson:"is_typing"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// ============ USER MODEL ============
type UserRegister struct {
	FirstName string `json:"first_name" binding:"required" bson:"first_name"`
	LastName  string `json:"last_name" binding:"required" bson:"last_name"`
	Email     string `json:"email" binding:"required" bson:"email"`
	Password  string `json:"password" binding:"required" bson:"password"`
}

type UserLogin struct {
	Email    string `json:"email" binding:"required" bson:"email"`
	Password string `json:"password" binding:"required" bson:"password"`
}
