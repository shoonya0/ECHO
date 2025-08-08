package main

import (
	"context"
	"flag"
	"gin/config"
	routes "gin/internal/api"
	"gin/internal/db"
	"gin/internal/middleware"
	"gin/internal/services"
	"gin/objects"
	"log"

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

	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	var (
		configPath = flag.String("config", "", "Path to the configuration file.")
	)

	config.New(*configPath)

	flag.Parse()

	// Initialize the database connection
	ctx := context.Background()
	db.ConnectDB(ctx)

	// Initialize Redis connection
	err := db.ConnectRedis(ctx)
	if err != nil {
		log.Printf("Failed to connect to Redis: %v", err)
		log.Println("Continuing without Redis - presence features will be disabled")
	} else {
		// Initialize presence pub/sub manager
		// err = cache.InitializePresenceManager()
		// if err != nil {
		// 	log.Printf("Failed to initialize presence manager: %v", err)
		// } else {
		// 	// Initialize presence service
		// 	services.InitializePresenceService()
		// 	log.Println("Presence service initialized successfully")
		// }
	}

	// Set the Gin mode based on the environment
	r.Use(middleware.CORSMiddleware())

	// Add presence middleware for activity tracking (only if presence service is available)
	// if services.PresenceServiceInstance != nil {
	// 	r.Use(middleware.PresenceMiddleware())

	// 	// Add cleanup middleware (runs every 10 minutes, marks users inactive after 15 minutes)
	// 	r.Use(middleware.PresenceCleanupMiddleware(15*time.Minute, 10*time.Minute))

	// 	// Add login/logout middleware
	// 	r.Use(middleware.UserLoginMiddleware())
	// 	r.Use(middleware.UserLogoutMiddleware())
	// }

	// r.GET("/", func(c *gin.Context) {
	// 	c.JSON(http.StatusOK, gin.H{"data": "hell world"})
	// })

	if objects.MainConfiguration.Port == "" {
		objects.MainConfiguration.Port = port
	}

	routes.RegisterAPIRoutes(r)

	// auth service configuration
	// authApi := r.Group(objects.AuthBasePath)

	// ============ WEBSOCKET SERVICE CONFIGURATION ============
	// Initialize user lookup service with caching
	services.InitUserLookupService()

	// Start WebSocket hub in a separate goroutine
	go func() {
		log.Println("Starting WebSocket Hub for real-time chat...")
		services.RunHub()
	}()

	// Message service configuration

	// Group service configuration

	// Media service configuration

	// Notification service configuration

	log.Printf("Starting Echo Chat Server on port %s with WebSocket support", objects.MainConfiguration.Port)
	r.Run(":" + objects.MainConfiguration.Port)
}
