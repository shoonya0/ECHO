package models

import (
	"gin/objects"
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
type ContactRequest struct {
	ChatID      bson.ObjectID         `json:"_id,omitempty" bson:"_id,omitempty"`
	RequestedBy bson.ObjectID         `json:"requestedBy" bson:"requestedBy"` // Who sent the friend request
	RequestedTo bson.ObjectID         `json:"requestedTo" bson:"requestedTo"` // Who received the friend request
	Status      objects.ContactStatus `json:"status" bson:"status"`           // "pending", "accepted", "blocked" , "favorite"

	// timestamps
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" bson:"updatedAt"`
}

type Contact struct {
	ID           bson.ObjectID   `json:"_id,omitempty" bson:"_id,omitempty"`
	Contacts     []bson.ObjectID `json:"contacts" bson:"contacts"`
	CreatedAt    time.Time       `json:"createdAt" bson:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt" bson:"updatedAt"` // this is the last time the contact was updated
	SentRequests []bson.ObjectID `json:"sentRequests" bson:"sentRequests"`
	// AcceptedContacts  []bson.ObjectID `json:"acceptedContacts" bson:"acceptedContacts"`
	// PendingContacts   []bson.ObjectID `json:"pendingContacts" bson:"pendingContacts"`
	// BlockedContacts   []bson.ObjectID `json:"blockedContacts" bson:"blockedContacts"`
	// FavoritesContacts []bson.ObjectID `json:"favoritesContacts" bson:"favoritesContacts"`
}
