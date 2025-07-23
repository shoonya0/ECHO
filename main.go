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

	r.Use(gin.Recovery())
	r.Use(gin.Logger())

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
}
