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
func GetUserByID(ctx context.Context, userID bson.ObjectID) (*models.User, error) {
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
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

func UpdateUserPresence(ctx context.Context, userID bson.ObjectID, status string) error {
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
		return fmt.Errorf("failed to update user presence: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

func UpdateProfile(ctx context.Context, userID bson.ObjectID, profileUpdate map[string]interface{}) error {
	filter := bson.M{"_id": userID}
	update := bson.M{"$set": profileUpdate}

	result, err := objects.DB.Collection(string(objects.UserColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("no user found to update profile")
	}
	return nil
}

func GetUserProfile(ctx context.Context, userID bson.ObjectID) (*models.GetUserProfileResponse, error) {
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
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}

	return &user, nil
}

func DeleteProfile(ctx context.Context, userID string) error {
	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user id format: %w", err)
	}

	filter := bson.M{"_id": objectID}

	result, err := objects.DB.Collection(string(objects.UserColl)).DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}

	if result.DeletedCount == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

func GetUserBasicInfo(ctx context.Context, userID bson.ObjectID) (*models.GetProfileResponse, error) {
	filter := bson.M{"_id": userID}

	projection := bson.M{
		"_id":           1,
		"username":      1,
		"email":         1,
		"phone":         1,
		"presence":      1,
		"profile":       1,
		"accountStatus": 1,
	}

	var user models.GetProfileResponse

	err := objects.DB.Collection(string(objects.UserColl)).FindOne(ctx, filter, options.FindOne().SetProjection(projection)).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user basic info: %w", err)
	}

	return &user, nil
}

func GetUsersByIDs(ctx context.Context, userIDs []bson.ObjectID) ([]models.LoginUserResponse, error) {
	if len(userIDs) == 0 {
		return []models.LoginUserResponse{}, nil
	}
	filter := bson.M{"_id": bson.M{"$in": userIDs}}
	projection := bson.M{
		"_id":           1,
		"username":      1,
		"profile":       1,
		"accountStatus": 1,
	}

	var users []models.LoginUserResponse
	cursor, err := objects.DB.Collection(string(objects.UserColl)).Find(ctx, filter, options.Find().SetProjection(projection))
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}
	defer cursor.Close(ctx)

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	for cursor.Next(ctx) {
		var user models.LoginUserResponse
		if err := cursor.Decode(&user); err != nil {
			return nil, fmt.Errorf("failed to decode user: %w", err)
		}
		users = append(users, user)
	}

	return users, nil
}

// func SearchUsers(ctx context.Context, query string, limit int) ([]models.User, error) {
// 	filter := bson.M{
// 		"$or": []bson.M{
// 			{"username": bson.M{"$regex": query, "$options": "i"}},
// 			{"profile.displayName": bson.M{"$regex": query, "$options": "i"}},
// 		},
// 		"accountStatus.isActive": true,
// 	}
// 	projection := bson.M{
// 		"_id":                   1,
// 		"username":              1,
// 		"profile.displayName":   1,
// 		"profile.avatar":        1,
// 		"profile.statusMessage": 1,
// 		"presence.status":       1,
// 	}
// 	users, err := FindAll[models.User](
// 		ctx,
// 		objects.DB.Collection(string(objects.UserColl)),
// 		filter,
// 		projection,
// 		bson.M{"username": 1},
// 		int64(limit),
// 		0,
// 	)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to search users: %w", err)
// 	}
// 	return users, nil
// }

// func CheckUserExists(ctx context.Context, userID bson.ObjectID) (bool, error) {
// 	filter := bson.M{
// 		"_id":                    userID,
// 		"accountStatus.isActive": true,
// 	}
// 	count, err := objects.DB.Collection(string(objects.UserColl)).CountDocuments(ctx, filter)
// 	if err != nil {
// 		return false, fmt.Errorf("failed to check user existence: %w", err)
// 	}
// 	exists := count > 0
// 	return exists, nil
// }

// func UpdateUserLastActivity(ctx context.Context, userID bson.ObjectID) error {
// 	log := logger.WithContext(ctx)
// 	log.WithField("user_id", userID.Hex()).Debug("Updating user last activity")
// 	filter := bson.M{"_id": userID}
// 	update := bson.M{
// 		"$set": bson.M{
// 			"presence.lastSeen": bson.M{"$currentDate": true},
// 		},
// 	}
// 	result, err := objects.DB.Collection(string(objects.UserColl)).UpdateOne(ctx, filter, update)
// 	if err != nil {
// 		log.WithError(err).Error("Failed to update user last activity")
// 		return fmt.Errorf("failed to update user last activity: %w", err)
// 	}
// 	if result.MatchedCount == 0 {
// 		log.Warn("No user found to update last activity")
// 		return fmt.Errorf("user not found")
// 	}
// 	log.WithField("updated_at", time.Now()).Debug("User last activity updated successfully")
// 	return nil
// }
