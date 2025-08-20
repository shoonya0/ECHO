package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type ContactInfoEmbed struct {
	BlockedChats []bson.ObjectID `json:"blockedChats" bson:"blockedChats"`
	PendingOut   []bson.ObjectID `json:"pendingOut" bson:"pendingOut"`
	PendingIn    []bson.ObjectID `json:"pendingIn" bson:"pendingIn"`
	Favorites    []bson.ObjectID `json:"favorites" bson:"favorites"`
	Contacts     []bson.ObjectID `json:"contacts" bson:"contacts"`
	CreatedAt    time.Time       `json:"createdAt" bson:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt" bson:"updatedAt"`
}
