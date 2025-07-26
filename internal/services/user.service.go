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

	if err := objects.DB.Collection(objects.UserColl).FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&user); err != nil {
		if err == mongo.ErrNoDocuments {
			return models.User{}, false, nil
		}
		return models.User{}, false, err
	}
	fmt.Println(user)
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
		if _, err := objects.DB.Collection(objects.UserColl).InsertOne(context.Background(), userData); err != nil {
			return models.User{}, err
		}
		return userData, nil
	}

	// update the user in the database
	if _, err := objects.DB.Collection(objects.UserColl).UpdateOne(context.Background(), bson.M{"_id": user.ID}, bson.M{"$set": userData}); err != nil {
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

	if err := objects.DB.Collection(objects.UserColl).FindOne(context.Background(), bson.M{"_id": objectID}, options.FindOne().SetProjection(projection)).Decode(&userData); err != nil {
		if err == mongo.ErrNoDocuments {
			return models.SearchUser{}, fmt.Errorf("user not found")
		}
		return models.SearchUser{}, err
	}

	return userData, nil
}

// this return array of users id with their name and avatar
func GetContacts(userID string, contactType objects.ContactType) ([]models.User, error) {
	users := []models.User{}

	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return []models.User{}, err
	}

	// first we have to get the information from the contact collection
	contact := models.Contact{}
	if err := objects.DB.Collection(objects.ContactColl).FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&contact); err != nil {
		if err == mongo.ErrNoDocuments {
			return []models.User{}, fmt.Errorf("you don't have any contact")
		}
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}

	// now we will get all the chat id from the contact collection
	chatIds := []string{}
	for _, contact := range contact.Contacts {
		chatIds = append(chatIds, contact.ChatID)
	}

	// now we find all the chat id(this is mongo id) and fetch the projection of name ,avater and sort it by updated at
	projection := bson.M{
		"_id":       1,
		"name":      1,
		"avatar":    1,
		"updatedAt": 1,
	}

	var cursor *mongo.Cursor

	if contactType == objects.RecentContacts {
		cursor, err = objects.DB.Collection(objects.ChatColl).Find(context.Background(), bson.M{"_id": bson.M{"$in": chatIds}}, options.Find().SetSort(bson.M{"updatedAt": -1}).SetProjection(projection))
		if err != nil {
			return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
		}
	} else if contactType == objects.AllContacts {
		cursor, err = objects.DB.Collection(objects.ChatColl).Find(context.Background(), bson.M{"_id": bson.M{"$in": chatIds}}, options.Find().SetProjection(projection))
		if err != nil {
			return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
		}
	}

	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var chat models.Chat
		if err := cursor.Decode(&chat); err != nil {
			return []models.User{}, fmt.Errorf("failed to decode chat: %w", err)
		}
		users = append(users, models.User{
			ID:       bson.ObjectID(chat.ChatID),
			Username: *chat.Name,
			Avatar:   *chat.Avatar,
		})
	}

	return users, nil
}

func GetContactRequests(userID string) ([]models.User, error) {
	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return []models.User{}, err
	}

	// first we have to get the information from the contact collection
	contact := models.Contact{}
	if err := objects.DB.Collection(objects.ContactColl).FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&contact); err != nil {
		if err == mongo.ErrNoDocuments {
			return []models.User{}, fmt.Errorf("you don't have any contact request")
		}
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}

	chatIds := []bson.ObjectID{}
	for _, contact := range contact.Contacts {
		if contact.Status == objects.StatusPending {
			objectID, err := bson.ObjectIDFromHex(contact.ChatID)
			if err != nil {
				return []models.User{}, err
			}
			chatIds = append(chatIds, objectID)
		}
	}

	// now we will get all the chat id from the contact collection and fetch the projection of name ,avater
	projection := bson.M{
		"_id":    1,
		"name":   1,
		"avatar": 1,
	}

	cursor, err := objects.DB.Collection(objects.ChatColl).Find(context.Background(), bson.M{"_id": bson.M{"$in": chatIds}}, options.Find().SetProjection(projection))
	if err != nil {
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}
	defer cursor.Close(context.Background())

	users := []models.User{}
	for cursor.Next(context.Background()) {
		var user models.User
		if err := cursor.Decode(&user); err != nil {
			return []models.User{}, fmt.Errorf("failed to decode user: %w", err)
		}
		users = append(users, user)
	}

	return users, nil
}

func GetSentContactRequests(userID string) ([]models.User, error) {
	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return []models.User{}, err
	}

	// first we have to get the information from the contact collection
	contact := models.Contact{}
	if err := objects.DB.Collection(objects.ContactColl).FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&contact); err != nil {
		if err == mongo.ErrNoDocuments {
			return []models.User{}, fmt.Errorf("no contact found")
		}
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}

	// now we will get all the chat id from the contact collection
	chatIds := []bson.ObjectID{}
	for _, contactID := range contact.SentRequests {
		if contactID != userID {
			objectID, err := bson.ObjectIDFromHex(contactID)
			if err != nil {
				return []models.User{}, err
			}
			chatIds = append(chatIds, objectID)
		}
	}

	// find all the name and avatar from chat collection
	projection := bson.M{
		"_id":    1,
		"name":   1,
		"avatar": 1,
	}

	// now we will get all the users from the users collection
	users := []models.User{}

	cursor, err := objects.DB.Collection(objects.ChatColl).Find(context.Background(), bson.M{"_id": bson.M{"$in": chatIds}}, options.Find().SetProjection(projection))
	if err != nil {
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}
	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var user models.User
		if err := cursor.Decode(&user); err != nil {
			return []models.User{}, fmt.Errorf("failed to decode user: %w", err)
		}
		users = append(users, models.User{
			ID:       bson.ObjectID(user.ID),
			Username: user.Username,
			Avatar:   user.Avatar,
		})
	}

	return users, nil
}

func GetBlockedUsers(userID string) ([]models.User, error) {
	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return []models.User{}, err
	}

	// first we have to get the information from the contact collection
	contact := models.Contact{}
	if err := objects.DB.Collection(objects.ContactColl).FindOne(context.Background(), bson.M{"_id": objectID}).Decode(&contact); err != nil {
		if err == mongo.ErrNoDocuments {
			return []models.User{}, fmt.Errorf("no contact found")
		}
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}

	// now we will get all the chat id from the contact collection
	chatIds := []bson.ObjectID{}
	for _, contact := range contact.Contacts {
		if contact.Status == objects.StatusBlocked {
			objectID, err := bson.ObjectIDFromHex(contact.ChatID)
			if err != nil {
				return []models.User{}, err
			}
			chatIds = append(chatIds, objectID)
		}
	}

	// find all the name and avatar from chat collection
	projection := bson.M{
		"_id":    1,
		"name":   1,
		"avatar": 1,
	}

	users := []models.User{}
	cursor, err := objects.DB.Collection(objects.ChatColl).Find(context.Background(), bson.M{"_id": bson.M{"$in": chatIds}}, options.Find().SetProjection(projection))
	if err != nil {
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}
	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var user models.User
		if err := cursor.Decode(&user); err != nil {
			return []models.User{}, fmt.Errorf("failed to decode user: %w", err)
		}
		users = append(users, models.User{
			ID:       bson.ObjectID(user.ID),
			Username: user.Username,
			Avatar:   user.Avatar,
		})
	}

	return users, nil
}
