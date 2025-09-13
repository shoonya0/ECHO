package main

import (
	"context"
	"flag"
	"gin/config"
	routes "gin/internal/api"
	"gin/internal/db"
	"gin/internal/middleware"
	"gin/internal/services"
	"gin/logger"
	"gin/objects"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

var Level logrus.Level

var (
	port       string
	ver        bool
	configPath string
)

func init() {
	flag.StringVar(&port, "port", ":8080", "The port to listen on.")
	flag.BoolVar(&ver, "version", true, "Print server version.")
	flag.StringVar(&configPath, "config", "", "Path to the configuration file.")
	Level = logrus.DebugLevel
}

func main() {
	config.New(configPath)
	flag.Parse()

	if err := logger.InitLogger("logs/server.log", Level); err != nil {
		panic(err)
	}

	// Create a new context with transaction ID for the main process
	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)

	log.Info("Initializing Echo Chat Server")

	r := gin.New()
	r.Use(gin.Recovery())

	// Connect to databases
	if err := db.ConnectDB(ctx); err != nil {
		log.WithError(err).Fatal("Failed to connect to database")
	}
	log.Info("Successfully connected to database")

	if err := db.ConnectRedis(ctx); err != nil {
		log.WithError(err).Warn("Failed to connect to Redis - presence features will be disabled")
	} else {
		log.Info("Successfully connected to Redis")
	}

	r.Use(middleware.CORSMiddleware())

	if objects.MainConfiguration.Port == "" {
		objects.MainConfiguration.Port = port
	}

	routes.RegisterAPIRoutes(r)

	// Initialize user lookup service with caching
	services.InitUserLookupService()
	log.Info("User lookup service initialized")

	// Start WebSocket hub in a separate goroutine
	go func() {
		log.Info("Starting WebSocket Hub for real-time chat")
		services.GetHubInstance().Start()
	}()

	log.WithField("port", objects.MainConfiguration.Port).Info("Starting Echo Chat Server with WebSocket support")
	if err := r.Run(":" + objects.MainConfiguration.Port); err != nil {
		log.WithError(err).Fatal("Server failed to start")
	}
}
