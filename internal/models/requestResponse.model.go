package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

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

// UpdateUserRequest handles partial updates for user profile
type UpdateUserRequest struct {
	// Basic info - using pointers to distinguish between zero values and unset values
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`

	// Profile fields
	Profile UserProfileEmbed `json:"profile,omitempty"`

	// Presence fields
	Presence PresenceEmbed `json:"presence,omitempty"`

	// Account status
	AccountStatus AccountStatusEmbed `json:"accountStatus,omitempty"`

	// Settings
	Settings UserSettingsEmbed `json:"settings,omitempty"`

	// Timestamps
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// ============ CONTACT MODEL (is in use) ============

type ContactRequest struct {
	ID          bson.ObjectID    `json:"_id,omitempty" bson:"_id,omitempty"`
	ContactInfo ContactInfoEmbed `json:"contactInfo,omitempty" bson:"contactInfo,omitempty"`
}

type GetContactInfo struct {
	ID          bson.ObjectID `json:"_id" bson:"_id"`
	ChatID      bson.ObjectID `json:"chatId" bson:"chatId"`
	Status      string        `json:"status" bson:"status"`
	Username    string        `json:"username" bson:"username"`
	DisplayName string        `json:"displayName" bson:"displayName"`
	Avatar      string        `json:"avatar" bson:"avatar"`
	IsFavorite  bool          `json:"isFavorite" bson:"isFavorite"`
}

// ============ CHAT MODEL ============
type GetChatResponse struct {
	ID          bson.ObjectID `json:"_id" bson:"_id"`
	Type        string        `json:"type" bson:"type"`
	Name        string        `json:"name" bson:"name"`
	Description string        `json:"description" bson:"description"`
	Avatar      string        `json:"avatar" bson:"avatar"`
	Messages    []Message     `json:"messages" bson:"messages"`
}

// ============ BATCH MESSAGE MODEL ============
// get message by chatId in batch
type BatchMessageByChatID struct {
	ChatID   bson.ObjectID `json:"chatId" bson:"chatId"`
	Messages []Message     `json:"messages" bson:"messages"`
}
