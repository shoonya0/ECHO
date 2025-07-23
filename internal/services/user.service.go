package services

import (
	"context"
	"fmt"
	"gin/internal/models"
	"gin/objects"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func isUserExists(userID string) (models.User, bool, error) {
	user := models.User{}
	if err := objects.DBClient.Database("ECHO").Collection("users").FindOne(context.Background(), bson.M{"user_id": userID}).Decode(&user); err != nil {
		if err == mongo.ErrNoDocuments {
			return models.User{}, false, nil
		}
		return models.User{}, false, err
	}
	return user, true, nil
}

func UpdateProfile(ctx *gin.Context, user models.User) (models.User, error) {
	// get the user id from the context
	userId, ok := ctx.Get("user_id")
	if !ok {
		return models.User{}, fmt.Errorf("user id not found")
	}
	// if their is no user than at that case we have to create new one insted of update
	userData, isUserExists, err := isUserExists(userId.(string))
	if err != nil {
		return models.User{}, err
	}

	if !isUserExists {
		// generate a new user id
		userData.ID = bson.NewObjectID()
		userData.UserID = userId.(string)
		userData.Username = user.Username
		userData.Email = user.Email
		userData.Phone = user.Phone
		userData.PasswordHash = user.PasswordHash
		userData.DisplayName = user.DisplayName
		userData.Avatar = user.Avatar
		userData.Bio = user.Bio
		userData.IsVerified = user.IsVerified
		userData.CreatedAt = time.Now()
		userData.UpdatedAt = time.Now()

		// create the user in the database
		if _, err := objects.DBClient.Database("ECHO").Collection("users").InsertOne(context.Background(), userData); err != nil {
			return models.User{}, err
		}
		return userData, nil
	} else {
		userData.Username = user.Username
		userData.Email = user.Email
		userData.Phone = user.Phone
		userData.PasswordHash = user.PasswordHash
		userData.DisplayName = user.DisplayName
		userData.Avatar = user.Avatar
		userData.Bio = user.Bio
		userData.IsVerified = user.IsVerified
		userData.UpdatedAt = time.Now()
	}

	// update the user in the database
	if _, err := objects.DBClient.Database("ECHO").Collection("users").UpdateOne(context.Background(), bson.M{"user_id": userData.UserID}, bson.M{"$set": userData}); err != nil {
		return models.User{}, err
	}

	return userData, nil
}

func GetProfile(ctx *gin.Context) (models.User, error) {
	userID, ok := ctx.Get("user_id")
	if !ok {
		return models.User{}, fmt.Errorf("user id not found")
	}

	user, isUserExists, err := isUserExists(userID.(string))
	if err != nil {
		return models.User{}, err
	}

	if !isUserExists {
		return models.User{}, fmt.Errorf("user not found")
	}

	return user, nil
}

func GetUserProfile(ctx *gin.Context) (models.User, error) {
	userID := ctx.Param("id")

	// check if the user id is valid
	if userID == "" {
		return models.User{}, fmt.Errorf("user id is required")
	}

	user, isUserExists, err := isUserExists(userID)
	if err != nil {
		return models.User{}, err
	}

	if !isUserExists {
		return models.User{}, fmt.Errorf("user not found")
	}

	searchUser := models.User{
		ID:       user.ID,
		Username: user.Username,
		Avatar:   user.Avatar,
	}

	return searchUser, nil
}

// this return array of users id with their name and avatar
func GetRecentUsers(ctx *gin.Context) ([]models.User, error) {
	users := []models.User{}
	userID, ok := ctx.Get("user_id")
	if !ok {
		return []models.User{}, fmt.Errorf("user id not found")
	}

	// first we have to get the information from the contact collection
	contact := models.Contact{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(ctx.Request.Context(), bson.M{"user_id": userID.(string)}).Decode(&contact); err != nil {
		if err == mongo.ErrNoDocuments {
			return []models.User{}, fmt.Errorf("no contact found")
		}
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}

	// now we will get all the chat id from the contact collection
	chatIds := []string{}
	for _, contact := range contact.Contacts {
		chatIds = append(chatIds, contact.ChatId)
	}

	// now we find all the chat id(this is mongo id) and fetch the projection of name ,avater and sort it by updated at
	projection := bson.M{
		"Name":      1,
		"Avatar":    1,
		"UpdatedAt": 1,
	}

	cursor, err := objects.DBClient.Database("ECHO").Collection("Chat").Find(ctx.Request.Context(), bson.M{"_id": bson.M{"$in": chatIds}}, options.Find().SetSort(bson.M{"updated_at": -1}).SetProjection(projection))
	if err != nil {
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}

	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var chat models.Chat
		if err := cursor.Decode(&chat); err != nil {
			return []models.User{}, fmt.Errorf("failed to decode chat: %w", err)
		}
		users = append(users, models.User{
			ID:       chat.ID,
			Username: *chat.Name,
			Avatar:   chat.Avatar,
		})
	}

	return users, nil
}

func GetContacts(ctx *gin.Context) ([]models.User, error) {
	userID, ok := ctx.Get("user_id")
	if !ok {
		return []models.User{}, fmt.Errorf("user id not found")
	}

	// first we have to get the information from the contact collection
	contact := models.Contact{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(ctx.Request.Context(), bson.M{"user_id": userID.(string)}).Decode(&contact); err != nil {
		if err == mongo.ErrNoDocuments {
			return []models.User{}, fmt.Errorf("no contact found")
		}
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}

	// now we will get all the chat id from the contact collection
	chatIds := []string{}
	for _, contact := range contact.Contacts {
		chatIds = append(chatIds, contact.ChatId)
	}

	// find all the name and avatar from chat collection
	projection := bson.M{
		"Name":   1,
		"Avatar": 1,
	}

	cursor, err := objects.DBClient.Database("ECHO").Collection("Chat").Find(ctx.Request.Context(), bson.M{"_id": bson.M{"$in": chatIds}}, options.Find().SetProjection(projection))
	if err != nil {
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}

	defer cursor.Close(context.Background())

	users := []models.User{}
	for cursor.Next(context.Background()) {
		var chat models.Chat
		if err := cursor.Decode(&chat); err != nil {
			return []models.User{}, fmt.Errorf("failed to decode chat: %w", err)
		}
		users = append(users, models.User{
			ID:       chat.ID,
			Username: *chat.Name,
			Avatar:   chat.Avatar,
		})
	}

	return users, nil
}

func GetContactRequests(ctx *gin.Context) ([]models.User, error) {
	userID, ok := ctx.Get("user_id")
	if !ok {
		return []models.User{}, fmt.Errorf("user id not found")
	}

	// first we have to get the information from the contact collection
	contact := models.Contact{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(ctx.Request.Context(), bson.M{"user_id": userID.(string)}).Decode(&contact); err != nil {
		if err == mongo.ErrNoDocuments {
			return []models.User{}, fmt.Errorf("no contact found")
		}
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}

	// now we will get all the chat id from the contact collection and fetch the projection of name ,avater
	projection := bson.M{
		"Name":   1,
		"Avatar": 1,
	}

	users := []models.User{}
	for _, contact := range contact.Contacts {
		if contact.Status == "pending" {
			chat := models.Chat{}
			if err := objects.DBClient.Database("ECHO").Collection("Chat").FindOne(ctx.Request.Context(), bson.M{"_id": contact.ChatId}, options.FindOne().SetProjection(projection)).Decode(&chat); err != nil {
				return []models.User{}, fmt.Errorf("failed to fetch chat: %w", err)
			}
			users = append(users, models.User{
				ID:       chat.ID,
				Username: *chat.Name,
				Avatar:   chat.Avatar,
			})
		}
	}

	return users, nil
}

func GetSentContactRequests(ctx *gin.Context) ([]models.User, error) {
	userID, ok := ctx.Get("user_id")
	if !ok {
		return []models.User{}, fmt.Errorf("user id not found")
	}

	// first we have to get the information from the contact collection
	contact := models.Contact{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(ctx.Request.Context(), bson.M{"user_id": userID.(string)}).Decode(&contact); err != nil {
		if err == mongo.ErrNoDocuments {
			return []models.User{}, fmt.Errorf("no contact found")
		}
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}

	// now we will get all the chat id from the contact collection
	chatIds := []string{}
	for _, contact := range contact.Contacts {
		if contact.RequestedBy != userID.(string) {
			chatIds = append(chatIds, contact.ChatId)
		}
	}

	// find all the name and avatar from chat collection
	projection := bson.M{
		"_id":    1,
		"Name":   1,
		"Avatar": 1,
	}

	// now we will get all the users from the users collection
	users := []models.User{}
	for _, chatId := range chatIds {
		chat := models.Chat{}
		if err := objects.DBClient.Database("ECHO").Collection("Chat").FindOne(ctx.Request.Context(), bson.M{"_id": chatId}, options.FindOne().SetProjection(projection)).Decode(&chat); err != nil {
			return []models.User{}, fmt.Errorf("failed to fetch chat: %w", err)
		}
		users = append(users, models.User{
			ID:       chat.ID,
			Username: *chat.Name,
			Avatar:   chat.Avatar,
		})
	}

	return users, nil
}

func GetBlockedUsers(ctx *gin.Context) ([]models.User, error) {
	userID, ok := ctx.Get("user_id")
	if !ok {
		return []models.User{}, fmt.Errorf("user id not found")
	}

	// first we have to get the information from the contact collection
	contact := models.Contact{}
	if err := objects.DBClient.Database("ECHO").Collection("contacts").FindOne(ctx.Request.Context(), bson.M{"user_id": userID.(string)}).Decode(&contact); err != nil {
		if err == mongo.ErrNoDocuments {
			return []models.User{}, fmt.Errorf("no contact found")
		}
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}

	// now we will get all the chat id from the contact collection
	chatIds := []string{}
	for _, contact := range contact.Contacts {
		if contact.Status == "blocked" && contact.RequestedBy == userID.(string) {
			chatIds = append(chatIds, contact.ChatId)
		}
	}

	// find all the name and avatar from chat collection
	projection := bson.M{
		"_id":    1,
		"Name":   1,
		"Avatar": 1,
	}

	users := []models.User{}
	for _, chatId := range chatIds {
		chat := models.Chat{}
		if err := objects.DBClient.Database("ECHO").Collection("Chat").FindOne(ctx.Request.Context(), bson.M{"_id": chatId}, options.FindOne().SetProjection(projection)).Decode(&chat); err != nil {
			return []models.User{}, fmt.Errorf("failed to fetch chat: %w", err)
		}
		users = append(users, models.User{
			ID:       chat.ID,
			Username: *chat.Name,
			Avatar:   chat.Avatar,
		})
	}

	return users, nil
}
