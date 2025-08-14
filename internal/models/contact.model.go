package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ OPTIMIZED CONTACT MODEL - DENORMALIZED ============
// Single model for all contact-related operations
type ContactInfoEmbed struct {
	// these all are the map of userID -> ChatID
	BlockedChats map[bson.ObjectID]bson.ObjectID `json:"blockedChats" bson:"blockedChats"`
	PendingOut   map[bson.ObjectID]bson.ObjectID `json:"pendingOut" bson:"pendingOut"`
	PendingIn    map[bson.ObjectID]bson.ObjectID `json:"pendingIn" bson:"pendingIn"`
	Favorites    map[bson.ObjectID]bson.ObjectID `json:"favorites" bson:"favorites"`
	Contacts     map[bson.ObjectID]bson.ObjectID `json:"contacts" bson:"contacts"`       // Recently contacted user IDs
	UnreadCount  map[bson.ObjectID]int           `json:"unreadCount" bson:"unreadCount"` // Total unread messages for each chat

	// Timestamps
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}
