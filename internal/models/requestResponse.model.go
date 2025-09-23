package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ LOGIN USER MODEL ============
type LoginUserResponse struct {
	ID            bson.ObjectID      `json:"_id" bson:"_id"`
	Email         string             `json:"email" bson:"email"`
	PasswordHash  string             `json:"passwordHash" bson:"passwordHash"`
	Username      string             `json:"username" bson:"username"`
	Profile       UserProfileEmbed   `json:"profile" bson:"profile"`
	AccountStatus AccountStatusEmbed `json:"accountStatus" bson:"accountStatus"`
	Presence      PresenceEmbed      `json:"presence" bson:"presence"`
}

// ============ GET PROFILE MODEL (is in use)============
type GetProfileResponse struct {
	ID            bson.ObjectID      `json:"_id" bson:"_id"`
	Username      string             `json:"username" bson:"username"`
	Email         string             `json:"email" bson:"email"`
	Phone         string             `json:"phone" bson:"phone"`
	Presence      PresenceEmbed      `json:"presence" bson:"presence"`
	Profile       UserProfileEmbed   `json:"profile" bson:"profile"`
	AccountStatus AccountStatusEmbed `json:"accountStatus" bson:"accountStatus"`
}

// ============ SEARCH USER MODEL PROFILE || MODELS FOR USER PROFILE (is in use) ============
type GetUserProfileResponse struct {
	ID            bson.ObjectID      `json:"_id" bson:"_id"`
	Profile       UserProfileEmbed   `json:"profile" bson:"profile"`
	AccountStatus AccountStatusEmbed `json:"accountStatus" bson:"accountStatus"`
}

// ============ UPDATE REQUEST MODELS (is in use) ============
type UpdateUserRequest struct {
	Username      string             `json:"username,omitempty" bson:"username,omitempty"`
	Email         string             `json:"email,omitempty" bson:"email,omitempty"`
	Phone         string             `json:"phone,omitempty" bson:"phone,omitempty"`
	Profile       UserProfileEmbed   `json:"profile" bson:"profile"`
	Presence      PresenceEmbed      `json:"presence" bson:"presence"`
	AccountStatus AccountStatusEmbed `json:"accountStatus" bson:"accountStatus"`
	Settings      UserSettingsEmbed  `json:"settings" bson:"settings"`
	UpdatedAt     time.Time          `json:"updatedAt" bson:"updatedAt"`
}

// ============ CONTACT MODEL (is in use) ============

type ContactRequest struct {
	ID          bson.ObjectID    `json:"_id,omitempty" bson:"_id,omitempty"`
	ContactInfo ContactInfoEmbed `json:"contactInfo,omitempty" bson:"contactInfo,omitempty"`
}

type ContactInfo struct {
	ID         bson.ObjectID    `json:"_id,omitempty" bson:"_id,omitempty"`
	Profile    UserProfileEmbed `json:"profile,omitempty" bson:"profile,omitempty"`
	Username   string           `json:"username,omitempty" bson:"username,omitempty"`
	Presence   PresenceEmbed    `json:"presence,omitempty" bson:"presence,omitempty"`
	IsFavorite bool             `json:"isFavorite,omitempty" bson:"isFavorite,omitempty"`
}

type SingleContactInfo struct {
	PendingIn    bson.ObjectID `json:"pendingIn" bson:"pendingIn"`
	PendingOut   bson.ObjectID `json:"pendingOut" bson:"pendingOut"`
	Favorites    bson.ObjectID `json:"favorites" bson:"favorites"`
	Contacts     bson.ObjectID `json:"contacts" bson:"contacts"`
	BlockedChats bson.ObjectID `json:"blockedChats" bson:"blockedChats"`
	CreatedAt    time.Time     `json:"createdAt" bson:"createdAt"`
	UpdatedAt    time.Time     `json:"updatedAt" bson:"updatedAt"`
}

type SingleContact struct {
	ID          bson.ObjectID     `json:"_id,omitempty" bson:"_id,omitempty"`
	ContactInfo SingleContactInfo `json:"contactInfo,omitempty" bson:"contactInfo,omitempty"`
}

// ============ CHAT MODEL ============
type ChatInfo struct {
	ChatID       bson.ObjectID                      `json:"_id,omitempty" bson:"_id,omitempty"`
	ChatType     string                             `json:"chatType" bson:"chatType"`         // "direct", "group", "channel"
	Participants map[bson.ObjectID]ParticipantEmbed `json:"participants" bson:"participants"` // userID -> participant reference
}
