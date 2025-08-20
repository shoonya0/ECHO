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

// ============ USER SERVICE FUNCTIONS ============

// GetUserByID retrieves a user by their ObjectID
func GetUserByID(userID bson.ObjectID) (*models.User, error) {
	filter := bson.M{"_id": userID}
	projection := bson.M{
		"_id":                    1,
		"username":               1,
		"email":                  1,
		"profile.displayName":    1,
		"profile.avatar":         1,
		"profile.statusMessage":  1,
		"presence.status":        1,
		"presence.lastSeen":      1,
		"accountStatus.isActive": 1,
		"createdAt":              1,
		"updatedAt":              1,
	}

	user, err := FindByID[models.User](context.Background(), objects.DB.Collection(string(objects.UserColl)), filter, projection)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// GetUsersByIDs retrieves multiple users by their ObjectIDs
func GetUsersByIDs(userIDs []bson.ObjectID) ([]models.User, error) {
	if len(userIDs) == 0 {
		return []models.User{}, nil
	}

	filter := bson.M{"_id": bson.M{"$in": userIDs}}
	projection := bson.M{
		"_id":                    1,
		"username":               1,
		"email":                  1,
		"profile.displayName":    1,
		"profile.avatar":         1,
		"profile.statusMessage":  1,
		"presence.status":        1,
		"presence.lastSeen":      1,
		"accountStatus.isActive": 1,
	}

	users, err := FindAll[models.User](
		context.Background(),
		objects.DB.Collection(string(objects.UserColl)),
		filter,
		projection,
		bson.M{},
		0,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	return users, nil
}

// UpdateUserPresence updates a user's presence status
func UpdateUserPresence(userID bson.ObjectID, status string) error {
	filter := bson.M{"_id": userID}
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"presence.status":   status,
			"presence.lastSeen": now,
		},
	}

	_, err := objects.DB.Collection(string(objects.UserColl)).UpdateOne(context.Background(), filter, update)
	if err != nil {
		return fmt.Errorf("failed to update user presence: %w", err)
	}

	return nil
}

// SearchUsers searches for users by username or display name
func SearchUsers(query string, limit int) ([]models.User, error) {
	filter := bson.M{
		"$or": []bson.M{
			{"username": bson.M{"$regex": query, "$options": "i"}},
			{"profile.displayName": bson.M{"$regex": query, "$options": "i"}},
		},
		"accountStatus.isActive": true,
	}

	projection := bson.M{
		"_id":                   1,
		"username":              1,
		"profile.displayName":   1,
		"profile.avatar":        1,
		"profile.statusMessage": 1,
		"presence.status":       1,
	}

	users, err := FindAll[models.User](
		context.Background(),
		objects.DB.Collection(string(objects.UserColl)),
		filter,
		projection,
		bson.M{"username": 1},
		int64(limit),
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to search users: %w", err)
	}

	return users, nil
}

// GetUserBasicInfo retrieves basic user information for display purposes
func GetUserBasicInfo(userID bson.ObjectID) (*models.UserProfileEmbed, error) {
	filter := bson.M{"_id": userID}
	projection := bson.M{
		"username":            1,
		"profile.displayName": 1,
		"profile.avatar":      1,
	}

	user, err := FindByID[models.User](context.Background(), objects.DB.Collection(string(objects.UserColl)), filter, projection)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user basic info: %w", err)
	}

	// Return a simplified profile structure
	profile := &models.UserProfileEmbed{
		DisplayName: user.Profile.DisplayName,
		Avatar:      user.Profile.Avatar,
	}

	return profile, nil
}

// CheckUserExists checks if a user exists and is active
func CheckUserExists(userID bson.ObjectID) (bool, error) {
	filter := bson.M{
		"_id":                    userID,
		"accountStatus.isActive": true,
	}

	count, err := objects.DB.Collection(string(objects.UserColl)).CountDocuments(context.Background(), filter)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return count > 0, nil
}

// UpdateUserLastActivity updates the user's last activity timestamp
func UpdateUserLastActivity(userID bson.ObjectID) error {
	filter := bson.M{"_id": userID}
	update := bson.M{
		"$set": bson.M{
			"presence.lastSeen": bson.M{"$currentDate": true},
		},
	}

	_, err := objects.DB.Collection(string(objects.UserColl)).UpdateOne(context.Background(), filter, update)
	return err
}

func UpdateProfile(userID bson.ObjectID, profileUpdate map[string]interface{}) error {
	filter := bson.M{"_id": userID}
	update := bson.M{"$set": profileUpdate}

	_, err := objects.DB.Collection(string(objects.UserColl)).UpdateOne(context.Background(), filter, update)
	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}
	return nil
}

func GetUserProfile(userID bson.ObjectID) (*models.GetUserProfileResponse, error) {
	filter := bson.M{"_id": userID}
	projection := bson.M{
		"_id":           1,
		"profile":       1,
		"accountStatus": 1,
	}

	var user models.GetUserProfileResponse
	err := objects.DB.Collection(string(objects.UserColl)).FindOne(context.Background(), filter, options.FindOne().SetProjection(projection)).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}

	return &user, nil
}

func DeleteProfile(userID string) error {
	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user id format: %w", err)
	}

	filter := bson.M{"_id": objectID}

	_, err = objects.DB.Collection(string(objects.UserColl)).DeleteOne(context.Background(), filter)
	if err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}
	return nil
}
