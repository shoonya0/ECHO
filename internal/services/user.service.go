package services

import (
	"context"
	"encoding/json"
	"fmt"
	"gin/internal/models"
	"gin/objects"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func GetUserIfExists(userID string) (models.User, bool, error) {
	user := models.User{}

	// first we convert the user id into primitive.NewObjectID().Hex()
	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return models.User{}, false, err
	}

	if err := objects.DB.Collection(string(objects.UserColl)).FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&user); err != nil {
		if err == mongo.ErrNoDocuments {
			return models.User{}, false, nil
		}
		return models.User{}, false, err
	}

	return user, true, nil
}

func unmarshalStructureIntoStructure(source interface{}, destination interface{}) error {
	jsonData, err := json.Marshal(source)
	if err != nil {
		return err
	}
	err = json.Unmarshal(jsonData, destination)
	if err != nil {
		return err
	}
	return nil
}

func UpdateProfile(user models.User) (models.User, error) {
	// if their is no user than at that case we have to create new one insted of update
	userData, isUserExists, err := GetUserIfExists(user.ID.Hex())
	if err != nil {
		return models.User{}, err
	}

	unmarshalStructureIntoStructure(&user, &userData)

	userData.ID = user.ID
	userData.UpdatedAt = time.Now()

	if !isUserExists {
		// generate a new user id
		userData.CreatedAt = time.Now()
		// create the user in the database
		if _, err := objects.DB.Collection(string(objects.UserColl)).InsertOne(context.Background(), userData); err != nil {
			return models.User{}, err
		}
		return userData, nil
	}

	// update the user in the database
	if _, err := objects.DB.Collection(string(objects.UserColl)).UpdateOne(context.Background(), bson.M{"_id": user.ID}, bson.M{"$set": userData}); err != nil {
		return models.User{}, err
	}

	return userData, nil
}

func GetUserProfile(userID string) (models.SearchUser, error) {
	if userID == "" {
		return models.SearchUser{}, fmt.Errorf("user id is required")
	}

	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return models.SearchUser{}, err
	}

	userData := models.SearchUser{}

	projection := bson.M{
		"_id":           1,
		"name":          1,
		"avatar":        1,
		"displayName":   1,
		"statusMessage": 1,
		"isVerified":    1,
	}

	if err := objects.DB.Collection(string(objects.UserColl)).FindOne(context.Background(), bson.M{"_id": objectID}, options.FindOne().SetProjection(projection)).Decode(&userData); err != nil {
		if err == mongo.ErrNoDocuments {
			return models.SearchUser{}, fmt.Errorf("user not found")
		}
		return models.SearchUser{}, err
	}

	return userData, nil
}
