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
	r := gin.New()

	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	var (
		configPath = flag.String("config", "", "Path to the configuration file.")
	)

	config.New(*configPath)

	flag.Parse()

	ctx := context.Background()
	db.ConnectDB(ctx)

	err := db.ConnectRedis(ctx)
	if err != nil {
		log.Printf("Failed to connect to Redis: %v", err)
		log.Println("Continuing without Redis - presence features will be disabled")
	} else {
	}

	r.Use(middleware.CORSMiddleware())

	if objects.MainConfiguration.Port == "" {
		objects.MainConfiguration.Port = port
	}

	routes.RegisterAPIRoutes(r)

	// Initialize user lookup service with caching
	services.InitUserLookupService()

	// Start WebSocket hub in a separate goroutine
	go func() {
		log.Println("Starting WebSocket Hub for real-time chat...")
		services.RunHub()
	}()

	log.Printf("Starting Echo Chat Server on port %s with WebSocket support", objects.MainConfiguration.Port)
	r.Run(":" + objects.MainConfiguration.Port)
}
