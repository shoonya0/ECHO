package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ UNIFIED CHAT MODEL WITH REAL-TIME CAPABILITIES ============
// Consolidated model for both persistence and real-time operations
type Chat struct {
	ChatID      bson.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	Type        string        `json:"type" bson:"type"` // "direct", "group", "channel"
	Name        string        `json:"name,omitempty" bson:"name,omitempty"`
	Description string        `json:"description,omitempty" bson:"description,omitempty"`
	Avatar      string        `json:"avatar,omitempty" bson:"avatar,omitempty"`

	// Participant references only (no embedded user data)
	Participants map[string]ParticipantRef `json:"participants" bson:"participants"` // userID -> participant reference

	// Owner/Admin info
	OwnerID  bson.ObjectID   `json:"ownerId,omitempty" bson:"ownerId,omitempty"`
	AdminIDs []bson.ObjectID `json:"adminIds" bson:"adminIds"`

	// Last message embedded for quick access
	LastMessageID bson.ObjectID `json:"lastMessageId" bson:"lastMessageId"`

	// Aggregated data
	Stats ChatStatsEmbed `json:"stats" bson:"stats"`

	// Settings
	Settings ChatSettingsEmbed `json:"settings" bson:"settings"`

	// Read receipts aggregated
	ReadReceipts map[string]time.Time `json:"readReceipts" bson:"readReceipts"` // userID -> last read timestamp

	// Typing indicators (TTL: 10 seconds)
	TypingUsers map[string]time.Time `json:"typingUsers" bson:"typingUsers"` // userID -> typing timestamp

	// Real-time capabilities (not persisted)
	ActiveClients map[string]*Client `json:"-" bson:"-"` // clientID -> client (in-memory only)
	IsActive      bool               `json:"isActive" bson:"isActive"`
	LastActivity  time.Time          `json:"lastActivity" bson:"lastActivity"`

	// Timestamps
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

// ParticipantRef represents a participant reference without embedded user data
type ParticipantRef struct {
	UserID      bson.ObjectID `json:"userId" bson:"userId"`
	Role        string        `json:"role" bson:"role"` // "member", "admin", "owner"
	IsBlocked   bool          `json:"isBlocked" bson:"isBlocked"`
	JoinedAt    time.Time     `json:"joinedAt" bson:"joinedAt"`
	LastActive  time.Time     `json:"lastActive" bson:"lastActive"`
	IsMuted     bool          `json:"isMuted" bson:"isMuted"`
	Permissions []string      `json:"permissions" bson:"permissions"`
}

// ParticipantEmbed kept for backward compatibility and response models
type ParticipantEmbed struct {
	UserID      bson.ObjectID `json:"userId" bson:"userId"`
	Username    string        `json:"username" bson:"username"`
	DisplayName string        `json:"displayName" bson:"displayName"`
	Avatar      string        `json:"avatar" bson:"avatar"`
	Role        string        `json:"role" bson:"role"` // "member", "admin", "owner"
	IsBlocked   bool          `json:"isBlocked" bson:"isBlocked"`
	JoinedAt    time.Time     `json:"joinedAt" bson:"joinedAt"`
	LastActive  time.Time     `json:"lastActive" bson:"lastActive"`
	IsMuted     bool          `json:"isMuted" bson:"isMuted"`
	Permissions []string      `json:"permissions" bson:"permissions"`
}

type LastMessageEmbed struct {
	MessageID   bson.ObjectID `json:"messageId" bson:"messageId"`
	Content     string        `json:"content" bson:"content"`
	SenderID    bson.ObjectID `json:"senderId" bson:"senderId"`
	SenderName  string        `json:"senderName" bson:"senderName"`
	MessageType string        `json:"messageType" bson:"messageType"`
	CreatedAt   time.Time     `json:"createdAt" bson:"createdAt"`
	IsDeleted   bool          `json:"isDeleted" bson:"isDeleted"`
}

type ChatStatsEmbed struct {
	MessageCount     int64          `json:"messageCount" bson:"messageCount"`
	ParticipantCount int            `json:"participantCount" bson:"participantCount"`
	FileCount        int64          `json:"fileCount" bson:"fileCount"`
	ImageCount       int64          `json:"imageCount" bson:"imageCount"`
	UnreadCount      map[string]int `json:"unreadCount" bson:"unreadCount"` // userID -> unread count
}

type ChatSettingsEmbed struct {
	IsPrivate        bool `json:"isPrivate" bson:"isPrivate"`
	AllowInvites     bool `json:"allowInvites" bson:"allowInvites"`
	AllowFileSharing bool `json:"allowFileSharing" bson:"allowFileSharing"`
	MessageRetention int  `json:"messageRetention" bson:"messageRetention"` // days, 0 = forever
	MaxParticipants  int  `json:"maxParticipants" bson:"maxParticipants"`
}
