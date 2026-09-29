// Package database owns the MongoDB and Redis connections shared by the server.
package database

import (
	"context"
	"fmt"

	"github.com/shoonya0/ECHO/internal/config"
	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/logger"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

var (
	// Client is the MongoDB client; DB is the application database on it.
	Client *mongo.Client
	DB     *mongo.Database
)

// ConnectMongo connects to MongoDB and verifies the connection with a ping.
func ConnectMongo(ctx context.Context) error {
	log := logger.WithContext(ctx)
	log.Info("Connecting to MongoDB...")

	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(config.Cfg.MongoURI).SetServerAPIOptions(serverAPI)

	var err error
	Client, err = mongo.Connect(opts)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB client: %w", err)
	}
	DB = Client.Database(string(constants.DBName))

	if err := Client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	log.WithField("database", string(constants.DBName)).Info("Connected to MongoDB")
	return nil
}

// DisconnectMongo closes the MongoDB connection.
func DisconnectMongo(ctx context.Context) error {
	if Client == nil {
		return nil
	}
	return Client.Disconnect(ctx)
}
