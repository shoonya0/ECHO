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
	if err := objects.DBClient.Database("user_db").Collection("users").FindOne(context.Background(), bson.M{"user_id": userID}).Decode(&user); err != nil {
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
		if _, err := objects.DBClient.Database("user_db").Collection("users").InsertOne(context.Background(), userData); err != nil {
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
	if _, err := objects.DBClient.Database("user_db").Collection("users").UpdateOne(context.Background(), bson.M{"user_id": userData.UserID}, bson.M{"$set": userData}); err != nil {
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

	// Use the request context for cancellation and tracing
	ctxMongo := ctx.Request.Context()

	// Build projection to only fetch ID, Username, and Avatar fields
	projection := bson.M{
		"id_1":       1,
		"id_2":       1,
		"username_1": 1,
		"username_2": 1,
		"avatar_1":   1,
		"avatar_2":   1,
		"created_at": 1,
	}

	// we have to find all the users which are recently have talked to the users for this we have to find all the
	// all the user Id from contact collection
	cursor, err := objects.DBClient.Database("user_db").Collection("contacts").Find(ctxMongo, bson.M{}, options.Find().SetSort(bson.M{"created_at": -1}).SetLimit(10).SetProjection(projection))
	if err != nil {
		return []models.User{}, fmt.Errorf("failed to fetch recent users: %w", err)
	}
	// findOptions := options.Find().
	// 	SetSort(bson.M{"created_at": -1}).
	// 	SetLimit(10).
	// 	SetProjection(projection)

	// cursor, err := objects.DBClient.
	// 	Database("user_db").
	// 	Collection("users").
	// 	Find(ctxMongo, bson.M{}, findOptions)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to fetch recent users: %w", err)
	// }

	// defer cursor.Close(context.Background())

	// for cursor.Next(context.Background()) {
	// 	var user models.User
	// 	if err := cursor.Decode(&user); err != nil {
	// 		return []models.User{}, err
	// 	}
	// 	searchUser := models.User{
	// 		ID:       user.ID,
	// 		Username: user.Username,
	// 		Avatar:   user.Avatar,
	// 	}
	// 	users = append(users, searchUser)
	// }

	return users, nil
}

func GetContacts(ctx *gin.Context) ([]models.User, error) {
	userID, ok := ctx.Get("user_id")
	if !ok {
		return []models.User{}, fmt.Errorf("user id not found")
	}

	_, isUserExists, err := isUserExists(userID.(string))
	if err != nil {
		return []models.User{}, err
	}

	if !isUserExists {
		return []models.User{}, fmt.Errorf("user not found")
	}

	// TODO: Implement contacts logic based on your requirements
	// For now, returning empty slice to make function complete
	return []models.User{}, nil
}
