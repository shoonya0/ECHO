package objects

import (
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type MainConfig struct {
	Port               string `mapstructure:"PORT"`
	Host               string `mapstructure:"HOST"`
	DBUri              string `mapstructure:"DB_URI"`
	DBUserName         string `mapstructure:"DB_USER_NAME"`
	RedisUri           string `mapstructure:"REDIS_URI"`
	RedisPass          string `mapstructure:"REDIS_PASS"`
	AwsRegion          string `mapstructure:"AWS_REGION"`
	AwsAccessKey       string `mapstructure:"AWS_ACCESS_KEY"`
	AwsSecretAccessKey string `mapstructure:"AWS_SECRET_ACCESS_KEY"`
	AwsS3Bucket        string `mapstructure:"AWS_S3_BUCKET"`
	AuthServiceUrl     string `mapstructure:"AUTH_SERVICE_URL"`
	JwtSecret          string `mapstructure:"JWT_SECRET"`
}

// collection
type Database string
type Collection string

// user type
type UserType string

// contact type
type ContactType string
type ContactStatus string
type MessageType string
type UserStatus string

// notification type
type NotificationType string

// cache ttl
type CacheTTL int

// cache keys
type CacheKey string

// chat type
type ChatType string

// permission type
type ChatPermissionType string

type ChatRole string

const (
	// collection names
	DBName      Database   = "Echo"
	UserColl    Collection = "users"
	ChatColl    Collection = "chats"
	MessageColl Collection = "messages"

	// user type
	UserTypeAdmin UserType = "admin"
	UserTypeUser  UserType = "user"

	// contact type
	RecentContacts ContactType = "recent"
	AllContacts    ContactType = "all"

	// contact status
	StatusPending  ContactStatus = "pending"
	StatusAccepted ContactStatus = "accepted"
	StatusBlocked  ContactStatus = "blocked"
	StatusFavorite ContactStatus = "favorite"
	StatusContact  ContactStatus = "contact"
	StatusDeclined ContactStatus = "declined"

	// message type
	MessageTypeText     MessageType = "text"
	MessageTypeImage    MessageType = "image"
	MessageTypeFile     MessageType = "file"
	MessageTypeAudio    MessageType = "audio"
	MessageTypeVideo    MessageType = "video"
	MessageTypeLocation MessageType = "location"

	// user status
	UserStatusOnline    UserStatus = "online"
	UserStatusAway      UserStatus = "away"
	UserStatusDND       UserStatus = "dnd"
	UserStatusInvisible UserStatus = "invisible"
	UserStatusOffline   UserStatus = "offline"

	// notification types
	NotificationTypeMessage       NotificationType = "message"
	NotificationTypeMention       NotificationType = "mention"
	NotificationTypeFriendRequest NotificationType = "friend_request"
	NotificationTypeGroupInvite   NotificationType = "group_invite"
	NotificationTypeCall          NotificationType = "call"

	// cache ttl (seconds)
	CacheUserProfile     CacheTTL = 3600 // 1 hour
	CacheChatList        CacheTTL = 1800 // 30 minutes
	CachePresence        CacheTTL = 300  // 5 minutes
	CacheTypingIndicator CacheTTL = 10   // 10 seconds

	// chat type
	ChatTypeDirect  ChatType = "direct"
	ChatTypeGroup   ChatType = "group"
	ChatTypeChannel ChatType = "channel"

	// chat permission type
	ChatPermissionRead  ChatPermissionType = "read"
	ChatPermissionWrite ChatPermissionType = "write"

	// chat role
	ChatRoleOwner  ChatRole = "owner"
	ChatRoleAdmin  ChatRole = "admin"
	ChatRoleMember ChatRole = "member"
)

// api paths
const (
	ApiVersion1          = "v1"
	ApiBasePath          = "/echo/" + ApiVersion1 + "/"
	AuthBasePath         = ApiBasePath + "auth/"
	WebSocketBasePath    = ApiBasePath + "websocket/"
	MessageBasePath      = WebSocketBasePath + "message/"
	GroupBasePath        = WebSocketBasePath + "group/"
	MediaBasePath        = WebSocketBasePath + "media/"
	NotificationBasePath = WebSocketBasePath + "notification" + "/"
	Version              = "1.0.0"
)

var (
	MainConfiguration MainConfig
	DBClient          *mongo.Client
	DB                *mongo.Database
	RedisClient       *redis.Client
	FileLog           *logrus.Logger
)

type UserData string

// ContextKey is a type for context keys
type ContextKey string

const (
	// TransactionIDKey is the context key for transaction ID
	TransactionIDKey ContextKey = "transaction_id"
	// UserIDKey is the context key for user ID
	UserIDKey ContextKey = "user_id"
	// RequestIDKey is the context key for request ID
	RequestIDKey ContextKey = "request_id"
	// UserDataKey is the context key for user data
	UserDataKey UserData = "user"
)
