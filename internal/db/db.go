package db

import (
	"context"
	"fmt"
	"gin/objects"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// var DB *DBConnection

// (*DBConnection, error)
func ConnectDB(ctx context.Context) {
	// Use the SetServerAPIOptions() method to set the version of the Stable API on the client
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(objects.MainConfiguration.DBUri).SetServerAPIOptions(serverAPI)
	// Create a new client and connect to the server

	var err error
	objects.DBClient, err = mongo.Connect(opts)
	if err != nil {
		panic(err)
	}

	objects.DB = objects.DBClient.Database(objects.DBName)

	// defer func() {
	// 	if err = client.Disconnect(ctx); err != nil {
	// 		fmt.Println("Failed to disconnect from MongoDB:", err)
	// 	} else {
	// 		fmt.Println("Disconnected from MongoDB successfully.")
	// 	}
	// }()

	if err := objects.DBClient.Ping(ctx, readpref.Primary()); err != nil {
		fmt.Println("failed to connect to MongoDB: %w", err)
	}
}
