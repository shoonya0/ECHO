package objects

import (
	"github.com/redis/go-redis/v9"
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

type Collection string
type Database string

// collection names
const (
	DBName             Database   = "Echo"
	UserColl           Collection = "users"
	ChatColl           Collection = "chats"
	MessageColl        Collection = "messages"
	GroupColl          Collection = "groups"
	ContactColl        Collection = "contacts"
	ContactRequestColl Collection = "contactRequests"
)

// contact type
type ContactType string

type ContactStatus string

const (
	RecentContacts ContactType = "recent"
	AllContacts    ContactType = "all"

	StatusPending  ContactStatus = "pendingContacts"
	StatusAccepted ContactStatus = "acceptedContacts"
	StatusBlocked  ContactStatus = "blockedContacts"
	StatusFavorite ContactStatus = "favoritesContacts"
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
)
