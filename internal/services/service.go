package services

import (
	"context"
	"errors"
	"fmt"
	"gin/internal/models"
	"gin/logger"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Sentinel errors for expected domain conditions.
// Controllers map these to HTTP status codes.
var (
	ErrUserNotFound           = errors.New("user not found")
	ErrChatNotFound           = errors.New("chat not found")
	ErrContactNotFound        = errors.New("contact not found")
	ErrContactRequestNotFound = errors.New("contact request not found")
	ErrInvalidInvite          = errors.New("invalid invite")
	ErrNotInContacts          = errors.New("user is not in your contacts")
	ErrNotInFavorites         = errors.New("user is not in your favorites")
	ErrAlreadyInContacts      = errors.New("user is already in your contacts")
	ErrAlreadyInFavorites     = errors.New("user is already in your favorites")
	ErrBlocked                = errors.New("user is blocked")
	ErrAlreadyBlocked         = errors.New("user is already blocked")
	ErrAlreadyUnblocked       = errors.New("user is already unblocked")
	ErrSelfBlock              = errors.New("cannot block yourself")
	ErrRequestAlreadySent     = errors.New("contact request already sent")
	ErrNoValidParticipants    = errors.New("no valid participants to remove")
	ErrNotParticipant         = errors.New("you are not a participant of this chat")
	ErrPermissionDenied       = errors.New("permission denied")
)

func CountActiveUsers(users []models.User) int {
	count := 0
	for _, user := range users {
		if user.AccountStatus.IsActive {
			count++
		}
	}
	return count
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

func RemoveElement(slice *[]bson.ObjectID, element bson.ObjectID) *[]bson.ObjectID {
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
	// logger
	log := logger.WithContext(ctx)
	log.WithField("collection", collection.Name()).Debug("Finding document by ID")

	var result T
	options := options.FindOne()

	if len(projection) > 0 {
		options.SetProjection(projection)
	}

	err := collection.FindOne(ctx, filter, options).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Debug("Document not found")
		} else {
			log.WithError(err).Error("Failed to find document")
		}
		return nil, err
	}

	log.Debug("Document found successfully")
	return &result, nil
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
	// logger
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"collection": collection.Name(),
		"limit":      limit,
		"skip":       skip,
	}).Debug("Finding all documents")

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
		log.WithError(err).Error("Failed to find documents")
		return nil, err
	}
	defer cursor.Close(ctx)

	err = cursor.All(ctx, &results)
	if err != nil {
		log.WithError(err).Error("Failed to decode documents")
		return nil, err
	}

	log.WithField("found_count", len(results)).Debug("Documents found successfully")
	return results, nil
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
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"collection": collection.Name(),
		"limit":      limit,
		"skip":       skip,
	}).Debug("Finding documents with cursor")

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
		log.WithError(err).Error("Failed to find documents")
		return nil, err
	}

	log.Debug("Cursor created successfully")
	return cursor, nil
	//note: we have to close the cursor in the parent function although this will create an dependecy
}

// this function will insert a document into the collection
func InsertOne[T any](
	ctx context.Context,
	coll *mongo.Collection,
	document T,
) (bson.ObjectID, error) {
	log := logger.WithContext(ctx)
	log.WithField("collection", coll.Name()).Debug("Inserting document")

	// Perform the insert
	res, err := coll.InsertOne(ctx, document)
	if err != nil {
		log.WithError(err).Error("Failed to insert document")
		return bson.ObjectID{}, fmt.Errorf("failed to insert document: %w", err)
	}

	// Try to cast the inserted ID to ObjectID (most common case)
	objectID, ok := res.InsertedID.(bson.ObjectID)
	if !ok {
		log.Error("Inserted ID is not an ObjectID")
		return bson.ObjectID{}, fmt.Errorf("inserted ID is not an ObjectID: %v", res.InsertedID)
	}

	log.WithField("inserted_id", objectID.Hex()).Debug("Document inserted successfully")
	return objectID, nil
}

// this function will insert multiple documents into the collection
func InsertMany[T any](
	ctx context.Context,
	coll *mongo.Collection,
	documents []T,
) ([]bson.ObjectID, error) {
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"collection":     coll.Name(),
		"document_count": len(documents),
	}).Debug("Inserting multiple documents")

	// convert the documents to interface{}
	docs := make([]interface{}, len(documents))
	for i, d := range documents {
		docs[i] = d
	}

	// insert the documents
	res, err := coll.InsertMany(ctx, docs)
	if err != nil {
		log.WithError(err).Error("Failed to insert documents")
		return nil, fmt.Errorf("failed to insert documents: %w", err)
	}

	// convert the inserted IDs to bson.ObjectID
	insertedIDs := make([]bson.ObjectID, len(res.InsertedIDs))
	for i, id := range res.InsertedIDs {
		insertedIDs[i] = id.(bson.ObjectID)
	}

	log.WithField("inserted_count", len(insertedIDs)).Debug("Documents inserted successfully")
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
	log := logger.WithContext(ctx)
	log.WithField("collection", coll.Name()).Debug("Updating document")

	res, err := coll.UpdateOne(ctx, filter, update, opts...)
	if err != nil {
		log.WithError(err).Error("Failed to update document")
		return nil, fmt.Errorf("failed to update document: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"matched_count":  res.MatchedCount,
		"modified_count": res.ModifiedCount,
		"upserted_id":    res.UpsertedID,
	}).Debug("Document updated successfully")

	return res, nil
}

// this function will update multiple documents in the collection
func UpdateMany[T any](
	ctx context.Context,
	coll *mongo.Collection,
	filter bson.M,
	update T,
	opts ...options.Lister[options.UpdateManyOptions],
) (*mongo.UpdateResult, error) {
	log := logger.WithContext(ctx)
	log.WithField("collection", coll.Name()).Debug("Updating multiple documents")

	res, err := coll.UpdateMany(ctx, filter, update, opts...)
	if err != nil {
		log.WithError(err).Error("Failed to update documents")
		return nil, fmt.Errorf("failed to update documents: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"matched_count":  res.MatchedCount,
		"modified_count": res.ModifiedCount,
		"upserted_id":    res.UpsertedID,
	}).Debug("Documents updated successfully")

	return res, nil
}

// FindByFilter finds a single document by filter
func FindByFilter[T any](
	ctx context.Context,
	collection *mongo.Collection,
	filter bson.M,
	projection bson.M,
) (T, error) {
	log := logger.WithContext(ctx)
	log.WithField("collection", collection.Name()).Debug("Finding document by filter")

	var result T
	options := options.FindOne()

	if len(projection) > 0 {
		options.SetProjection(projection)
	}

	err := collection.FindOne(ctx, filter, options).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Debug("Document not found")
		} else {
			log.WithError(err).Error("Failed to find document")
		}
		return result, err
	}

	log.Debug("Document found successfully")
	return result, nil
}
