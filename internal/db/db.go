package db

import (
	"context"
	"fmt"
	"gin/logger"
	"gin/objects"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// ConnectDB establishes a connection to MongoDB
func ConnectDB(ctx context.Context) error {
	log := logger.WithContext(ctx)
	log.Info("Connecting to MongoDB...")

	// Use the SetServerAPIOptions() method to set the version of the Stable API on the client
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(objects.MainConfiguration.DBUri).SetServerAPIOptions(serverAPI)
	// Create a new client and connect to the server
	var err error
	objects.DBClient, err = mongo.Connect(opts)
	if err != nil {
		log.WithError(err).Error("Failed to create MongoDB client")
		return fmt.Errorf("failed to create MongoDB client: %w", err)
	}

	objects.DB = objects.DBClient.Database(string(objects.DBName))
	log.WithField("database", string(objects.DBName)).Info("Selected MongoDB database")

	// Test connection with ping
	if err := objects.DBClient.Ping(ctx, readpref.Primary()); err != nil {
		log.WithError(err).Error("Failed to ping MongoDB")
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	log.Info("Successfully connected to MongoDB")
	return nil
}

// DisconnectDB closes the MongoDB connection
func DisconnectDB(ctx context.Context) error {
	log := logger.WithContext(ctx)
	if objects.DBClient != nil {
		if err := objects.DBClient.Disconnect(ctx); err != nil {
			log.WithError(err).Error("Failed to disconnect from MongoDB")
			return fmt.Errorf("failed to disconnect from MongoDB: %w", err)
		}
		log.Info("Successfully disconnected from MongoDB")
	}
	return nil
}
