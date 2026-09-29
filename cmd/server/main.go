// Command server runs the ECHO chat API: REST endpoints plus a WebSocket hub.
package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shoonya0/ECHO/internal/config"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/middleware"
	"github.com/shoonya0/ECHO/internal/realtime"
	"github.com/shoonya0/ECHO/internal/router"
	"github.com/shoonya0/ECHO/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func main() {
	envFile := flag.String("env", ".env", "optional KEY=VALUE file loaded before reading the environment")
	flag.Parse()

	if err := config.Load(*envFile); err != nil {
		logrus.WithError(err).Fatal("Invalid configuration")
	}

	level, err := logrus.ParseLevel(config.Cfg.LogLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	if err := logger.InitLogger(config.Cfg.LogFile, level); err != nil {
		logrus.WithError(err).Warn("Log file unavailable, logging to stdout")
	}

	ctx := logger.WithTransactionID(context.Background())
	log := logger.WithContext(ctx)
	log.Info("Initializing Echo Chat Server")

	if err := database.ConnectMongo(ctx); err != nil {
		log.WithError(err).Fatal("Failed to connect to MongoDB")
	}
	// Redis backs token revocation and WebSocket pub/sub, so it is required.
	if err := database.ConnectRedis(ctx); err != nil {
		log.WithError(err).Fatal("Failed to connect to Redis")
	}

	hub := realtime.GetHubInstance()
	services.SetBroadcaster(hub)
	if err := hub.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start WebSocket hub")
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CORSMiddleware())
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	router.RegisterAPIRoutes(r)

	srv := &http.Server{Addr: ":" + config.Cfg.Port, Handler: r}
	go func() {
		log.WithField("port", config.Cfg.Port).Info("Echo Chat Server listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.WithError(err).Fatal("Server failed")
		}
	}()

	// Wait for Ctrl+C or a platform shutdown signal (Render sends SIGTERM on deploy).
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Info("Shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("HTTP shutdown did not complete cleanly")
	}
	database.CloseRedis()
	database.DisconnectMongo(shutdownCtx)
}
