package services

import (
	"context"
	"fmt"
	"gin/internal/models"
	"gin/objects"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func UpdateProfile(user models.User) (models.User, error) {
	// if their is no user than at that case we have to create new one insted of update
	userData, err := FindByID[models.User](context.Background(), objects.DB.Collection(string(objects.UserColl)), bson.M{"_id": user.ID}, bson.M{})
	if err != nil && err != mongo.ErrNoDocuments {
		return models.User{}, err
	}

	if err == mongo.ErrNoDocuments {
		// generate a new user id - create new user
		*userData = user
		userData.CreatedAt = time.Now()
		userData.UpdatedAt = time.Now()

		// create the user in the database
		if _, err := objects.DB.Collection(string(objects.UserColl)).InsertOne(context.Background(), *userData); err != nil {
			return models.User{}, err
		}
		return *userData, nil
	}

	unmarshalStructureIntoStructure(&user, &userData)

	userData.ID = user.ID
	userData.UpdatedAt = time.Now()

	// update the user in the database
	if _, err := objects.DB.Collection(string(objects.UserColl)).UpdateOne(context.Background(), bson.M{"_id": user.ID}, bson.M{"$set": *userData}); err != nil {
		return models.User{}, err
	}

	return *userData, nil
}

func GetUserProfile(userID string) (models.SearchUser, error) {
	if userID == "" {
		return models.SearchUser{}, fmt.Errorf("user id is required")
	}

	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return models.SearchUser{}, err
	}

	projection := bson.M{
		"_id":           1,
		"username":      1,
		"avatar":        1,
		"displayName":   1,
		"statusMessage": 1,
	}

	searchUser := models.SearchUser{}

	if err := objects.DB.Collection(string(objects.UserColl)).FindOne(context.Background(), bson.M{"_id": objectID}, options.FindOne().SetProjection(projection)).Decode(&searchUser); err != nil {
		if err == mongo.ErrNoDocuments {
			return models.SearchUser{}, fmt.Errorf("user not found")
		}
		return models.SearchUser{}, err
	}

	return searchUser, nil
}
