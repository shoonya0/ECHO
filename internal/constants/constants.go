// Package constants holds shared names and enum values: database collections,
// API paths, domain statuses, and context keys.
package constants

// ApiBasePath is the prefix for every HTTP and WebSocket route.
const ApiBasePath = "/echo/v1/"

// MongoDB database and collections.
type Database string
type Collection string

const (
	DBName      Database   = "Echo"
	UserColl    Collection = "users"
	ChatColl    Collection = "chats"
	MessageColl Collection = "messages"
	// MigrationColl holds one document per startup migration that needs state (#5).
	MigrationColl Collection = "migrations"
)

// Contact relationship statuses.
type ContactStatus string

const (
	StatusPending    ContactStatus = "pending"
	StatusPendingIn  ContactStatus = "pendingIn"
	StatusPendingOut ContactStatus = "pendingOut"
	StatusAccepted   ContactStatus = "accepted"
	StatusBlocked    ContactStatus = "blocked"
	StatusFavorite   ContactStatus = "favorite"
	StatusContact    ContactStatus = "contact"
	StatusDeclined   ContactStatus = "declined"
	StatusUnblocked  ContactStatus = "unblocked"
)

// User presence statuses. presence.status in MongoDB holds the one the user
// chose; whether they are connected comes from OnlineKeyPrefix (#31).
type UserStatus string

const (
	UserStatusOnline    UserStatus = "online"
	UserStatusAway      UserStatus = "away"
	UserStatusDND       UserStatus = "dnd"
	UserStatusInvisible UserStatus = "invisible" // shown to others as offline
	UserStatusOffline   UserStatus = "offline"
)

// Valid reports whether s is a status a user may choose.
func (s UserStatus) Valid() bool {
	switch s {
	case UserStatusOnline, UserStatusAway, UserStatusDND, UserStatusInvisible, UserStatusOffline:
		return true
	}
	return false
}

// Hidden reports whether a user who chose s is shown to others as offline
// and never announced online: invisible, or offline by choice (#31).
func (s UserStatus) Hidden() bool {
	return s == UserStatusInvisible || s == UserStatusOffline
}

// OnlineKeyPrefix starts the Redis sorted set online:<userID>, which records
// the servers a user is connected to (#31).
const OnlineKeyPrefix = "online:"

// Chat types, permissions, and participant roles.
type ChatType string
type ChatPermissionType string
type ChatRole string

const (
	ChatTypeDirect  ChatType = "direct"
	ChatTypeGroup   ChatType = "group"
	ChatTypeChannel ChatType = "channel"

	ChatPermissionRead  ChatPermissionType = "read"
	ChatPermissionWrite ChatPermissionType = "write"

	ChatRoleOwner  ChatRole = "owner"
	ChatRoleAdmin  ChatRole = "admin"
	ChatRoleMember ChatRole = "member"
)

// DefaultMaxGroupParticipants is the size limit a new group stores in
// settings.maxParticipants, and the one used for groups that store none (#68).
const DefaultMaxGroupParticipants = 100

// Context keys.
type ContextKey string
type UserData string

const (
	TransactionIDKey ContextKey = "transaction_id"
	UserIDKey        ContextKey = "user_id"
	RequestIDKey     ContextKey = "request_id"
	ClientIDKey      ContextKey = "client_id"

	// UserDataKey stores the authenticated models.LoginUserResponse.
	UserDataKey UserData = "user"
	UsernameKey UserData = "username"
)
