package services

import (
	"context"
	"encoding/json"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// BuildPartialDocument creates a MongoDB update document only with non-nil fields
// This prevents updating fields that weren't provided in the request
func BuildPartialDocument(updateReq interface{}) bson.M {
	updateDoc := bson.M{}

	jsonData, err := json.Marshal(updateReq)
	if err != nil {
		return updateDoc
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(jsonData, &dataMap); err != nil {
		return updateDoc
	}

	buildNestedUpdate("", dataMap, updateDoc)

	return updateDoc
}

func buildNestedUpdate(prefix string, data map[string]interface{}, updateDoc bson.M) {
	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch metaData := value.(type) {
		case map[string]interface{}:
			buildNestedUpdate(fullKey, metaData, updateDoc)
		case nil:
			continue
		default:
			updateDoc[fullKey] = value
		}
	}
}

func contains(slice []bson.ObjectID, item bson.ObjectID) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func findIndex(slice []bson.ObjectID, item bson.ObjectID) int {
	for i, v := range slice {
		if v == item {
			return i
		}
	}
	return -1
}

func removeElement(slice *[]bson.ObjectID, element bson.ObjectID) *[]bson.ObjectID {
	if slice == nil {
		return slice
	}
	result := make([]bson.ObjectID, 0, len(*slice))
	for _, item := range *slice {
		if item != element {
			result = append(result, item)
		}
	}
	return &result
}

// we use UserService of interface type to implement the interface
func FindByID[T any](
	ctx context.Context,
	collection *mongo.Collection,
	filter bson.M,
	projection bson.M,
) (*T, error) {
	var result T
	options := options.FindOne()

	if len(projection) > 0 {
		options.SetProjection(projection)
	}

	err := collection.FindOne(ctx, filter, options).Decode(&result)
	return &result, err
}

// this function will return all the documents from the collection
func FindAll[T any](
	ctx context.Context,
	collection *mongo.Collection,
	filter bson.M,
	projection bson.M,
	sort bson.M,
	limit int64,
	skip int64,
) ([]T, error) {
	var results []T
	options := options.Find()
	if len(projection) > 0 {
		options.SetProjection(projection)
	}
	if len(sort) > 0 {
		options.SetSort(sort)
	}
	if limit > 0 {
		options.SetLimit(limit)
	}
	if skip > 0 {
		options.SetSkip(skip)
	}
	cursor, err := collection.Find(ctx, filter, options)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	err = cursor.All(ctx, &results)
	return results, err
}

// this function will return a cursor of the documents from the collection
func FindMany(
	ctx context.Context,
	collection *mongo.Collection,
	filter bson.M,
	projection bson.M,
	sort bson.M,
	limit int64,
	skip int64,
) (*mongo.Cursor, error) {
	options := options.Find()

	if len(projection) > 0 {
		options.SetProjection(projection)
	}
	if len(sort) > 0 {
		options.SetSort(sort)
	}
	if limit > 0 {
		options.SetLimit(limit)
	}
	if skip > 0 {
		options.SetSkip(skip)
	}

	cursor, err := collection.Find(ctx, filter, options)
	return cursor, err
	//note: we have to close the cursor in the parent function although this will create an dependecy
}

// this function will insert a document into the collection
func InsertOne[T any](
	ctx context.Context,
	coll *mongo.Collection,
	document T,
) (bson.ObjectID, error) {
	// Perform the insert
	res, err := coll.InsertOne(ctx, document)
	if err != nil {
		return bson.ObjectID{}, fmt.Errorf("failed to insert document: %w", err)
	}

	// Try to cast the inserted ID to ObjectID (most common case)
	objectID, ok := res.InsertedID.(bson.ObjectID)
	if !ok {
		return bson.ObjectID{}, fmt.Errorf("inserted ID is not an ObjectID: %v", res.InsertedID)
	}

	return objectID, err
}

// this function will insert multiple documents into the collection
func InsertMany[T any](
	ctx context.Context,
	coll *mongo.Collection,
	documents []T,
) ([]bson.ObjectID, error) {
	// convert the documents to interface{}
	docs := make([]interface{}, len(documents))
	for i, d := range documents {
		docs[i] = d
	}

	// insert the documents
	res, err := coll.InsertMany(ctx, docs)
	if err != nil {
		return nil, fmt.Errorf("failed to insert documents: %w", err)
	}

	// convert the inserted IDs to bson.ObjectID
	insertedIDs := make([]bson.ObjectID, len(res.InsertedIDs))
	for i, id := range res.InsertedIDs {
		insertedIDs[i] = id.(bson.ObjectID)
	}
	return insertedIDs, nil
}

// this function will update a document in the collection
func UpdateOne[T any](
	ctx context.Context,
	coll *mongo.Collection,
	filter bson.M,
	update T,
	opts ...options.Lister[options.UpdateOneOptions],
) (*mongo.UpdateResult, error) {
	res, err := coll.UpdateOne(ctx, filter, update, opts...)
	return res, err
}

// this function will update multiple documents in the collection
func UpdateMany[T any](
	ctx context.Context,
	coll *mongo.Collection,
	filter bson.M,
	update T,
	opts ...options.Lister[options.UpdateManyOptions],
) (*mongo.UpdateResult, error) {
	res, err := coll.UpdateMany(ctx, filter, update, opts...)
	return res, err

}

// FindByFilter finds a single document by filter
func FindByFilter[T any](
	ctx context.Context,
	collection *mongo.Collection,
	filter bson.M,
	projection bson.M,
) (T, error) {
	var result T
	options := options.FindOne()

	if len(projection) > 0 {
		options.SetProjection(projection)
	}

	err := collection.FindOne(ctx, filter, options).Decode(&result)
	return result, err
}
