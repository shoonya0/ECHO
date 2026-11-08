package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type ContactInfoEmbed struct {
	// Blocked lists the users this user blocked. Only this user can lift these.
	Blocked []bson.ObjectID `json:"blocked" bson:"blocked"`
	// BlockedBy lists the users who blocked this user (#26). It backs the block
	// checks and is never sent to clients, so nobody learns who blocked them.
	BlockedBy  []bson.ObjectID `json:"-" bson:"blockedBy"`
	PendingOut []bson.ObjectID `json:"pendingOut" bson:"pendingOut"`
	PendingIn  []bson.ObjectID `json:"pendingIn" bson:"pendingIn"`
	Favorites  []bson.ObjectID `json:"favorites" bson:"favorites"`
	Contacts   []bson.ObjectID `json:"contacts" bson:"contacts"`
	CreatedAt  time.Time       `json:"createdAt" bson:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt" bson:"updatedAt"`
}
