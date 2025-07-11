package main

import (
	"context"
	"flag"
	"gin/config"
	routes "gin/internal/api"
	"gin/internal/db"
	"gin/internal/middleware"
	"gin/objects"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	port string
	ver  bool
)

func init() {
	flag.StringVar(&port, "port", ":8080", "The port to listen on.")
	flag.BoolVar(&ver, "version", true, "Print server version.")
}

func main() {
	// main service configuration
	r := gin.New()

	var (
		configPath = flag.String("config", "", "Path to the configuration file.")
	)

	config.New(*configPath)

	flag.Parse()

	// Initialize the database connection
	db.ConnectDB(context.Background())

	// Set the Gin mode based on the environment
	r.Use(middleware.CORSMiddleware())

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": "hell world"})
	})

	if objects.MainConfiguration.Port == "" {
		objects.MainConfiguration.Port = port
	}

	routes.RegisterAPIRoutes(r)

	// auth service configuration
	// authApi := r.Group(objects.AuthBasePath)

	// websocket service configuration

	// Message service configuration

	// Group service configuration

	// Media service configuration

	// Notification service configuration
	r.Run(":" + objects.MainConfiguration.Port)

	// initialize zerolog

	// zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	// log.Print("logging initialized")
	// // log.Panic().Msg("This is a panic message")
	// // log.Fatal().Msg("Fatal error occurred")
	// log.Error().Msg("An error occurred")
	// log.Warn().Msg("This is a warning")
	// log.Info().Msg("This is an info message")
	// log.Debug().Msg("This is a debug message")
	// log.Trace().Msg("This is a trace message")

	// // set defaults
	// viper.SetDefault("server.port", "9000")
	// viper.SetDefault("server.host", "localhost")

	// viper.SetConfigName("config")
	// viper.SetConfigType("env")
	// viper.AddConfigPath("./../../")
	// if err := viper.ReadInConfig(); err != nil {

	// 	if _, ok := err.(viper.ConfigFileNotFoundError); ok {
	// 		fmt.Println("Config file not found, using defaults")
	// 	} else {
	// 		fmt.Println("Error reading config file: ", err)
	// 	}
	// 	return
	// }

	// viper.OnConfigChange(func(in fsnotify.Event) {
	// 	fmt.Print("Config file changed: ", in.Name)
	// })
	// viper.WatchConfig()

	// an simple simple server example using gin
	// r := gin.Default()

	// r.GET("/", func(c *gin.Context) {
	// 	c.JSON(200, gin.H{"message": "Welcome to the Main Server"})
	// })

	// port := viper.GetString("server.port")
	// if port == "" {
	// 	port = "9000" // default port
	// }

	// if err := r.Run(fmt.Sprintf(":%s", port)); err != nil {
	// 	log.Fatal().Err(err).Msg("Failed to start server")
	// 	return
	// }
}
