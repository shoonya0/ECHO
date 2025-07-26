package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// here user id 1 and user id 2 save the refference of the user from the user collection
// type Contact struct {
// 	ID          bson.ObjectID `json:"id" bson:"_id,omitempty"`
// 	ID1         string        `json:"id_1" bson:"id_1"`                 // this is the user id of the user (reff ID) who is the contact
// 	ID2         string        `json:"id_2" bson:"id_2"`                 // this is the user id of the user (reff ID) who is the contact
// 	Status      string        `json:"status" bson:"status"`             // "pending", "accepted", "blocked"
// 	RequestedBy string        `json:"requested_by" bson:"requested_by"` // Who sent the friend request

// 	// Timestamps
// 	CreatedAt  time.Time  `json:"created_at" bson:"created_at"`
// 	AcceptedAt *time.Time `json:"accepted_at,omitempty" bson:"accepted_at,omitempty"`
// }

// if this is an sub type of Contact then is this also contain it's own _id in mongo db?
type Invite struct {
	ChatID      string `json:"_id" bson:"_id"`
	Status      string `json:"status" bson:"status"`             // "pending", "accepted", "blocked"
	RequestedBy string `json:"requested_by" bson:"requested_by"` // Who sent the friend request

	// timestamps
	CreatedAt time.Time  `json:"created_at" bson:"created_at"`
	ExpiresAt *time.Time `json:"expires_at" bson:"expires_at"` // Expiration time for the invite
}

type Contact struct {
	ID           bson.ObjectID     `json:"_id,omitempty" bson:"_id,omitempty"`
	Contacts     map[string]Invite `json:"contacts" bson:"contacts"`
	SentRequests []string          `json:"sentRequests" bson:"sentRequests"`
	CreatedAt    time.Time         `json:"createdAt" bson:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt" bson:"updatedAt"`
}
