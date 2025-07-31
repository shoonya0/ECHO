package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Notification struct {
	ID      bson.ObjectID `json:"id" bson:"_id,omitempty"`
	UserID  string        `json:"user_id" bson:"user_id"`
	Type    string        `json:"type" bson:"type"` // "message", "mention", "friend_request", "group_invite"
	Title   string        `json:"title" bson:"title"`
	Message string        `json:"message" bson:"message"`

	// Related entities
	RelatedID *string `json:"related_id,omitempty" bson:"related_id,omitempty"` // Message ID, User ID, etc.
	ChatID    *string `json:"chat_id,omitempty" bson:"chat_id,omitempty"`
	SenderID  *string `json:"sender_id,omitempty" bson:"sender_id,omitempty"`

	// Status
	IsRead bool       `json:"is_read" bson:"is_read"`
	ReadAt *time.Time `json:"read_at,omitempty" bson:"read_at,omitempty"`

	// Action buttons for rich notifications (accept, decline, view, reply)
	Actions string `json:"actions,omitempty" bson:"actions,omitempty"` // "accept", "decline", "view", "reply"

	// Timestamps
	CreatedAt time.Time  `json:"created_at" bson:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" bson:"expires_at,omitempty"`
}

// ============ BATCH OPERATIONS MODELS ============
// Models designed for efficient batch operations means that we can send multiple messages at once by batching them into a single request

type BatchNotification struct {
	UserID        bson.ObjectID      `json:"userId" bson:"userId"`
	Notifications []NotificationItem `json:"notifications" bson:"notifications"`
	TotalCount    int                `json:"totalCount" bson:"totalCount"`
	UnreadCount   int                `json:"unreadCount" bson:"unreadCount"`
	CreatedAt     time.Time          `json:"createdAt" bson:"createdAt"`
}

type NotificationItem struct {
	ID        bson.ObjectID  `json:"id" bson:"id"`
	Type      string         `json:"type" bson:"type"`
	Title     string         `json:"title" bson:"title"`
	Message   string         `json:"message" bson:"message"`
	ChatID    *bson.ObjectID `json:"chatId,omitempty" bson:"chatId,omitempty"`
	SenderID  *bson.ObjectID `json:"senderId,omitempty" bson:"senderId,omitempty"`
	IsRead    bool           `json:"isRead" bson:"isRead"`
	CreatedAt time.Time      `json:"createdAt" bson:"createdAt"`
	ExpiresAt *time.Time     `json:"expiresAt,omitempty" bson:"expiresAt,omitempty"`
}
