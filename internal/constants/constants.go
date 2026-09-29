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

// User presence statuses.
type UserStatus string

const (
	UserStatusOnline  UserStatus = "online"
	UserStatusOffline UserStatus = "offline"
)

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
