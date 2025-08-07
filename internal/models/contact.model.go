package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ OPTIMIZED CONTACT MODEL - DENORMALIZED ============
// Single model for all contact-related operations
type ContactInfoEmbed struct {
	// All contact relationships in one document
	Relationships map[string]ContactRelationship `json:"relationships" bson:"relationships"` // targetUserID -> relationship

	// Quick lookup arrays (duplicated for performance)
	BlockedChats []bson.ObjectID `json:"blockedChats" bson:"blockedChats"`
	PendingOut   []bson.ObjectID `json:"pendingOut" bson:"pendingOut"`
	PendingIn    []bson.ObjectID `json:"pendingIn" bson:"pendingIn"`
	Favorites    []bson.ObjectID `json:"favorites" bson:"favorites"`
	ActiveChats  []bson.ObjectID `json:"activeChats" bson:"activeChats"`

	// Cached stats
	Stats ContactStatsEmbed `json:"stats" bson:"stats"`

	// Recent activity for sorting
	RecentInteractions []RecentInteractionEmbed `json:"recentInteractions" bson:"recentInteractions"`

	// Timestamps
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

type ContactRelationship struct {
	TargetUserID bson.ObjectID `json:"targetUserId" bson:"targetUserId"`
	Status       string        `json:"status" bson:"status"` // "blocked", "pending", "accepted", "declined"
	RequestedBy  bson.ObjectID `json:"requestedBy" bson:"requestedBy"`
	IsFavorite   bool          `json:"isFavorite" bson:"isFavorite"`

	ChatID bson.ObjectID `json:"chatId" bson:"chatId"`

	// Embedded user data for quick access
	UserInfo ContactUserInfo `json:"userInfo" bson:"userInfo"`

	// Interaction stats
	// MessageCount    int64     `json:"messageCount" bson:"messageCount"`
	// LastInteraction time.Time `json:"lastInteraction" bson:"lastInteraction"`

	// Timestamps
	CreatedAt  time.Time  `json:"createdAt" bson:"createdAt"`
	AcceptedAt *time.Time `json:"acceptedAt,omitempty" bson:"acceptedAt,omitempty"`
}

type ContactUserInfo struct {
	Username    string `json:"username" bson:"username"`
	DisplayName string `json:"displayName" bson:"displayName"`
	Avatar      string `json:"avatar" bson:"avatar"`
	// Status      string `json:"status" bson:"status"` // "online", "away", "dnd", "invisible", "offline"
	// IsOnline    bool      `json:"isOnline" bson:"isOnline"`
	// LastSeen    time.Time `json:"lastSeen" bson:"lastSeen"`
}

type ContactStatsEmbed struct {
	TotalContacts   int `json:"totalContacts" bson:"totalContacts"`
	BlockedCount    int `json:"blockedCount" bson:"blockedCount"`
	PendingOutCount int `json:"pendingOutCount" bson:"pendingOutCount"`
	PendingInCount  int `json:"pendingInCount" bson:"pendingInCount"`
	FavoriteCount   int `json:"favoriteCount" bson:"favoriteCount"`
}

type RecentInteractionEmbed struct {
	UserID        bson.ObjectID `json:"userId" bson:"userId"`
	LastMessageAt time.Time     `json:"lastMessageAt" bson:"lastMessageAt"`
	MessageCount  int           `json:"messageCount" bson:"messageCount"` // in last 24h
}
