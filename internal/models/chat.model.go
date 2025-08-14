package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ UNIFIED CHAT MODEL WITH REAL-TIME CAPABILITIES ============
// Consolidated model for both persistence and real-time operations
type Chat struct {
	ChatID      bson.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	ChatType    string        `json:"chatType" bson:"chatType"` // "direct", "group", "channel"
	Name        string        `json:"name" bson:"name"`
	Description string        `json:"description,omitempty" bson:"description,omitempty"`
	Avatar      string        `json:"avatar,omitempty" bson:"avatar,omitempty"`

	// Participant references only (no embedded user data)
	Participants map[bson.ObjectID]ParticipantEmbed `json:"participants" bson:"participants"` // userID -> participant reference

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
	ReadReceipts map[bson.ObjectID]time.Time `json:"readReceipts" bson:"readReceipts"` // userID -> last read timestamp

	// Typing indicators (TTL: 10 seconds)
	TypingUsers map[bson.ObjectID]time.Time `json:"typingUsers" bson:"typingUsers"` // userID -> typing timestamp

	// Real-time capabilities (not persisted)
	ActiveClients map[string]*Client `json:"-" bson:"-"` // clientID -> client (in-memory only)
	LastActivity  time.Time          `json:"lastActivity" bson:"lastActivity"`

	// Timestamps
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

// ParticipantRef represents a participant reference without embedded user data
type ParticipantEmbed struct {
	RequestStatus string          `json:"requestStatus" bson:"requestStatus"` // "blocked", "pending", "accepted", "declined"
	OnlineStatus  string          `json:"onlineStatus" bson:"onlineStatus"`   // "online", "away", "dnd", "invisible", "offline"
	RequestedBy   bson.ObjectID   `json:"requestedBy" bson:"requestedBy"`     // the user who requested to join the chat
	Permissions   []string        `json:"permissions" bson:"permissions"`
	IsBlocked     bool            `json:"isBlocked" bson:"isBlocked"`
	UserInfo      ContactUserInfo `json:"userInfo" bson:"userInfo"`
	LastSeen      time.Time       `json:"lastSeen" bson:"lastSeen"`
	IsMuted       bool            `json:"isMuted" bson:"isMuted"`
	Role          string          `json:"role" bson:"role"` // "member", "admin", "owner"

	// Timestamps
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
	JoinedAt  time.Time `json:"joinedAt" bson:"joinedAt"`
	LeftAt    time.Time `json:"leftAt" bson:"leftAt"`
}

type ContactUserInfo struct {
	DisplayName string        `json:"displayName" bson:"displayName"`
	Username    string        `json:"username" bson:"username"`
	UserID      bson.ObjectID `json:"userId" bson:"userId"`
	Avatar      string        `json:"avatar" bson:"avatar"`
}

type ChatStatsEmbed struct {
	ParticipantCount int                   `json:"participantCount" bson:"participantCount"`
	MessageCount     int64                 `json:"messageCount" bson:"messageCount"`
	UnreadCount      map[bson.ObjectID]int `json:"unreadCount" bson:"unreadCount"` // userID -> unread count
	ImageCount       int64                 `json:"imageCount" bson:"imageCount"`
	FileCount        int64                 `json:"fileCount" bson:"fileCount"`
}

type ChatSettingsEmbed struct {
	IsPrivate        bool `json:"isPrivate" bson:"isPrivate"`
	AllowInvites     bool `json:"allowInvites" bson:"allowInvites"`
	AllowFileSharing bool `json:"allowFileSharing" bson:"allowFileSharing"`
	MessageRetention int  `json:"messageRetention" bson:"messageRetention"` // days, 0 = forever
	MaxParticipants  int  `json:"maxParticipants" bson:"maxParticipants"`
}

// no used till now
type LastMessageEmbed struct {
	MessageID   bson.ObjectID `json:"messageId" bson:"messageId"`
	Content     string        `json:"content" bson:"content"`
	SenderID    bson.ObjectID `json:"senderId" bson:"senderId"`
	SenderName  string        `json:"senderName" bson:"senderName"`
	MessageType string        `json:"messageType" bson:"messageType"`
	CreatedAt   time.Time     `json:"createdAt" bson:"createdAt"`
	IsDeleted   bool          `json:"isDeleted" bson:"isDeleted"`
}
