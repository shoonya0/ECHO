package objects

import "go.mongodb.org/mongo-driver/v2/mongo"

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

const (
	ApiVersion           = "v1"
	ApiBasePath          = "/echo/" + ApiVersion + "/"
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
)
