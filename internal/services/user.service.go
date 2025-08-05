package services

import (
	"context"
	"fmt"
	"gin/internal/models"
	"gin/objects"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func UpdateProfile(userID bson.ObjectID, updateReq models.UpdateUserRequest) (*mongo.UpdateResult, error) {
	_, err := FindByID[models.User](context.Background(), objects.DB.Collection(string(objects.UserColl)), bson.M{"_id": userID}, bson.M{})
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	updateDoc := BuildPartialUpdateDocument(updateReq)

	filter := bson.M{"_id": userID}

	// due to below code the updatedAt field will be always returned ModifiedCount = 1
	updateDoc["updatedAt"] = time.Now().UTC()
	updateDoc = bson.M{"$set": updateDoc}

	res, err := UpdateOne(context.Background(), objects.DB.Collection(string(objects.UserColl)), filter, updateDoc)
	if err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return res, nil
}

func GetUserProfile(userID string) (models.GetUserProfileResponse, error) {
	if userID == "" {
		return models.GetUserProfileResponse{}, fmt.Errorf("user id is required")
	}

	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return models.GetUserProfileResponse{}, err
	}

	filter := bson.M{"_id": objectID}

	projection := bson.M{
		"_id":                      1,
		"profile":                  1,
		"accountStatus.isVerified": 1,
		"accountStatus.isBanned":   1,
	}

	userData, err := FindByID[models.GetUserProfileResponse](context.Background(), objects.DB.Collection(string(objects.UserColl)), filter, projection)
	if err != nil {
		return models.GetUserProfileResponse{}, err
	}

	return *userData, nil
}
