package services

import (
	"context"
	"fmt"
	"gin/internal/models"
	"gin/logger"
	"gin/objects"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ============ USER SERVICE FUNCTIONS ============

// GetUserByID retrieves a user by their ObjectID
func GetUserByID(ctx context.Context, userID bson.ObjectID) (*models.User, error) {
	log := logger.WithContext(ctx)
	log.WithField("user_id", userID.Hex()).Debug("Retrieving user by ID")

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

	user, err := FindByID[models.User](ctx, objects.DB.Collection(string(objects.UserColl)), filter, projection)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.WithError(err).Warn("User not found")
			return nil, fmt.Errorf("user not found")
		}
		log.WithError(err).Error("Failed to get user")
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"username":    user.Username,
		"email":       user.Email,
		"is_active":   user.AccountStatus.IsActive,
		"last_active": user.Presence.LastSeen,
	}).Debug("User retrieved successfully")

	return user, nil
}

// GetUsersByIDs retrieves multiple users by their ObjectIDs
func GetUsersByIDs(ctx context.Context, userIDs []bson.ObjectID) ([]models.User, error) {
	log := logger.WithContext(ctx)
	log.WithField("user_count", len(userIDs)).Debug("Retrieving multiple users by IDs")

	if len(userIDs) == 0 {
		log.Debug("No user IDs provided, returning empty list")
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
		ctx,
		objects.DB.Collection(string(objects.UserColl)),
		filter,
		projection,
		bson.M{},
		0,
		0,
	)
	if err != nil {
		log.WithError(err).Error("Failed to get users")
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"requested_count": len(userIDs),
		"found_count":     len(users),
		"active_count":    countActiveUsers(users),
	}).Debug("Users retrieved successfully")

	return users, nil
}

// countActiveUsers is a helper function to count active users
func countActiveUsers(users []models.User) int {
	count := 0
	for _, user := range users {
		if user.AccountStatus.IsActive {
			count++
		}
	}
	return count
}

// UpdateUserPresence updates a user's presence status
func UpdateUserPresence(ctx context.Context, userID bson.ObjectID, status string) error {
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"user_id": userID.Hex(),
		"status":  status,
	}).Debug("Updating user presence")

	filter := bson.M{"_id": userID}
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"presence.status":   status,
			"presence.lastSeen": now,
		},
	}

	result, err := objects.DB.Collection(string(objects.UserColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		log.WithError(err).Error("Failed to update user presence")
		return fmt.Errorf("failed to update user presence: %w", err)
	}

	if result.MatchedCount == 0 {
		log.Warn("No user found to update presence")
		return fmt.Errorf("user not found")
	}

	log.WithFields(map[string]interface{}{
		"status":     status,
		"last_seen":  now,
		"updated_at": time.Now(),
	}).Debug("User presence updated successfully")

	return nil
}

// SearchUsers searches for users by username or display name
func SearchUsers(ctx context.Context, query string, limit int) ([]models.User, error) {
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"query": query,
		"limit": limit,
	}).Debug("Searching users")

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
		ctx,
		objects.DB.Collection(string(objects.UserColl)),
		filter,
		projection,
		bson.M{"username": 1},
		int64(limit),
		0,
	)
	if err != nil {
		log.WithError(err).Error("Failed to search users")
		return nil, fmt.Errorf("failed to search users: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"found_count":  len(users),
		"active_count": countActiveUsers(users),
	}).Debug("Users search completed successfully")

	return users, nil
}

// GetUserBasicInfo retrieves basic user information for display purposes
func GetUserBasicInfo(ctx context.Context, userID bson.ObjectID) (*models.UserProfileEmbed, error) {
	log := logger.WithContext(ctx)
	log.WithField("user_id", userID.Hex()).Debug("Retrieving user basic info")

	filter := bson.M{"_id": userID}
	projection := bson.M{
		"username":            1,
		"profile.displayName": 1,
		"profile.avatar":      1,
	}

	user, err := FindByID[models.User](ctx, objects.DB.Collection(string(objects.UserColl)), filter, projection)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.WithError(err).Warn("User not found")
			return nil, fmt.Errorf("user not found")
		}
		log.WithError(err).Error("Failed to get user basic info")
		return nil, fmt.Errorf("failed to get user basic info: %w", err)
	}

	// Return a simplified profile structure
	profile := &models.UserProfileEmbed{
		DisplayName: user.Profile.DisplayName,
		Avatar:      user.Profile.Avatar,
	}

	log.WithFields(map[string]interface{}{
		"display_name": profile.DisplayName,
		"has_avatar":   profile.Avatar != "",
	}).Debug("User basic info retrieved successfully")

	return profile, nil
}

// CheckUserExists checks if a user exists and is active
func CheckUserExists(ctx context.Context, userID bson.ObjectID) (bool, error) {
	log := logger.WithContext(ctx)
	log.WithField("user_id", userID.Hex()).Debug("Checking user existence")

	filter := bson.M{
		"_id":                    userID,
		"accountStatus.isActive": true,
	}

	count, err := objects.DB.Collection(string(objects.UserColl)).CountDocuments(ctx, filter)
	if err != nil {
		log.WithError(err).Error("Failed to check user existence")
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	exists := count > 0
	log.WithFields(map[string]interface{}{
		"exists":     exists,
		"is_active":  exists,
		"checked_at": time.Now(),
	}).Debug("User existence check completed")

	return exists, nil
}

// UpdateUserLastActivity updates the user's last activity timestamp
func UpdateUserLastActivity(ctx context.Context, userID bson.ObjectID) error {
	log := logger.WithContext(ctx)
	log.WithField("user_id", userID.Hex()).Debug("Updating user last activity")

	filter := bson.M{"_id": userID}
	update := bson.M{
		"$set": bson.M{
			"presence.lastSeen": bson.M{"$currentDate": true},
		},
	}

	result, err := objects.DB.Collection(string(objects.UserColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		log.WithError(err).Error("Failed to update user last activity")
		return fmt.Errorf("failed to update user last activity: %w", err)
	}

	if result.MatchedCount == 0 {
		log.Warn("No user found to update last activity")
		return fmt.Errorf("user not found")
	}

	log.WithField("updated_at", time.Now()).Debug("User last activity updated successfully")
	return nil
}

func UpdateProfile(ctx context.Context, userID bson.ObjectID, profileUpdate map[string]interface{}) error {
	log := logger.WithContext(ctx)
	log.WithFields(map[string]interface{}{
		"user_id":          userID.Hex(),
		"fields_to_update": len(profileUpdate),
	}).Debug("Updating user profile")

	filter := bson.M{"_id": userID}
	update := bson.M{"$set": profileUpdate}

	result, err := objects.DB.Collection(string(objects.UserColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		log.WithError(err).Error("Failed to update profile")
		return fmt.Errorf("failed to update profile: %w", err)
	}

	if result.MatchedCount == 0 {
		log.Warn("No user found to update profile")
		return fmt.Errorf("user not found")
	}

	log.WithFields(map[string]interface{}{
		"updated_fields": len(profileUpdate),
		"updated_at":     time.Now(),
	}).Debug("User profile updated successfully")
	return nil
}

func GetUserProfile(ctx context.Context, userID bson.ObjectID) (*models.GetUserProfileResponse, error) {
	log := logger.WithContext(ctx)
	log.WithField("user_id", userID.Hex()).Debug("Retrieving user profile")

	filter := bson.M{"_id": userID}
	projection := bson.M{
		"_id":           1,
		"profile":       1,
		"accountStatus": 1,
	}

	var user models.GetUserProfileResponse
	err := objects.DB.Collection(string(objects.UserColl)).FindOne(ctx, filter, options.FindOne().SetProjection(projection)).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.WithError(err).Warn("User not found")
			return nil, fmt.Errorf("user not found")
		}
		log.WithError(err).Error("Failed to get user profile")
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}

	log.WithFields(map[string]interface{}{
		"display_name": user.Profile.DisplayName,
		"is_active":    user.AccountStatus.IsActive,
	}).Debug("User profile retrieved successfully")

	return &user, nil
}

func DeleteProfile(ctx context.Context, userID string) error {
	log := logger.WithContext(ctx)
	log.WithField("user_id", userID).Debug("Deleting user profile")

	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		log.WithError(err).Error("Invalid user ID format")
		return fmt.Errorf("invalid user id format: %w", err)
	}

	filter := bson.M{"_id": objectID}

	result, err := objects.DB.Collection(string(objects.UserColl)).DeleteOne(ctx, filter)
	if err != nil {
		log.WithError(err).Error("Failed to delete profile")
		return fmt.Errorf("failed to delete profile: %w", err)
	}

	if result.DeletedCount == 0 {
		log.Warn("No user found to delete")
		return fmt.Errorf("user not found")
	}

	log.WithFields(map[string]interface{}{
		"deleted_at": time.Now(),
	}).Info("User profile deleted successfully")
	return nil
}
