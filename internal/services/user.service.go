package services

import (
	"context"
	"fmt"
	"gin/internal/models"
	"gin/logger"
	"gin/objects"
	"strings"
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
			return nil, fmt.Errorf("%w: %w", ErrUserNotFound, err)
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
		return fmt.Errorf("%w", ErrUserNotFound)
	}

	return nil
}

func UpdateProfile(ctx context.Context, userID bson.ObjectID, profileUpdate map[string]interface{}) error {
	filter := bson.M{"_id": userID}

	// Before applying the update, fetch the existing document so we can
	// prevent required fields from being overwritten with empty values.
	// This protects identity/display fields (username, email, displayName)
	// from being wiped by a partial request that omits them or sends "".
	var existingDoc bson.M
	err := objects.DB.Collection(string(objects.UserColl)).FindOne(ctx, filter).Decode(&existingDoc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("%w: %w", ErrUserNotFound, err)
		}
		return fmt.Errorf("failed to fetch existing user: %w", err)
	}

	// Defense-in-depth: drop any backend/service-owned field that escaped
	// the controller whitelist. Account state must never be changed through
	// the profile-update path.
	dropImmutableUpdateKeys(profileUpdate)

	// Sanitize the update map: replace empty values for required fields
	// with their previously stored values.
	sanitizeUpdateFields(profileUpdate, existingDoc)

	// updatedAt is server-controlled, never client-controlled.
	profileUpdate["updatedAt"] = time.Now()

	update := bson.M{"$set": profileUpdate}

	result, err := objects.DB.Collection(string(objects.UserColl)).UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("%w", ErrUserNotFound)
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
			return nil, fmt.Errorf("%w: %w", ErrUserNotFound, err)
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
		return fmt.Errorf("%w", ErrUserNotFound)
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
			return nil, fmt.Errorf("%w: %w", ErrUserNotFound, err)
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

// GetUserSuggestions returns paginated user suggestions for lazy loading.
// It excludes the requesting user, their existing contacts/favorites/pending/blocked,
// and any IDs provided via excludeIDs. All users (active or inactive) are included.
// Results are sorted by createdAt descending for stable paging.
func GetUserSuggestions(ctx context.Context, userID bson.ObjectID, page, limit int, excludeIDs []bson.ObjectID) ([]models.UserSuggestion, int64, error) {
	coll := objects.DB.Collection(string(objects.UserColl))

	skip := int64((page - 1) * limit)
	limit64 := int64(limit)

	// Build the aggregation pipeline: single round-trip with two self-$lookups:
	// one for paginated suggestions, one for the true unfiltered total count.
	// All exclusion filtering happens inside MongoDB — no data fetched into Go beforehand.

	// $match inside the $lookup pipeline: exclude self, all known relationships,
	// and client-provided excludeIDs.
	// NOTE: Dotted field paths must NOT appear inside $expr keys — MongoDB 5.0+
	// rejects them with error 16412.
	matchStages := []bson.D{
		{bson.E{Key: "$match", Value: bson.D{bson.E{Key: "$expr", Value: bson.D{
			bson.E{Key: "$and", Value: []bson.M{
				{"$ne": []interface{}{"$_id", "$$selfId"}},
				{"$not": bson.D{bson.E{Key: "$in", Value: []interface{}{"$_id", "$$blocked"}}}},
				{"$not": bson.D{bson.E{Key: "$in", Value: []interface{}{"$_id", "$$pendingOut"}}}},
				{"$not": bson.D{bson.E{Key: "$in", Value: []interface{}{"$_id", "$$pendingIn"}}}},
				{"$not": bson.D{bson.E{Key: "$in", Value: []interface{}{"$_id", "$$favorites"}}}},
				{"$not": bson.D{bson.E{Key: "$in", Value: []interface{}{"$_id", "$$contacts"}}}},
			}},
		}}}}},
	}

	// Append client-provided excludeIDs if non-empty.
	if len(excludeIDs) > 0 {
		matchStages = append(matchStages, bson.D{bson.E{Key: "$match", Value: bson.M{"_id": bson.M{"$nin": excludeIDs}}}})
	}

	// $skip must be int64 for the aggregation stage.
	skipStage := bson.D{bson.E{Key: "$skip", Value: skip}}
	limitStage := bson.D{bson.E{Key: "$limit", Value: limit64}}
	sortStage := bson.D{bson.E{Key: "$sort", Value: bson.D{bson.E{Key: "createdAt", Value: -1}}}}
	projectStage := bson.D{bson.E{Key: "$project", Value: bson.D{
		bson.E{Key: "_id", Value: 1},
		bson.E{Key: "username", Value: 1},
		bson.E{Key: "profile", Value: 1},
	}}}

	lookupPipeline := make(mongo.Pipeline, 0, len(matchStages)+4)
	for _, s := range matchStages {
		lookupPipeline = append(lookupPipeline, s)
	}
	lookupPipeline = append(lookupPipeline, sortStage, skipStage, limitStage, projectStage)

	// reusable "let" block for both lookups (same variables referenced by $$)
	letDoc := bson.D{
		bson.E{Key: "selfId", Value: "$_id"},
		bson.E{Key: "blocked", Value: "$contactInfo.blockedChats"},
		bson.E{Key: "pendingOut", Value: "$contactInfo.pendingOut"},
		bson.E{Key: "pendingIn", Value: "$contactInfo.pendingIn"},
		bson.E{Key: "favorites", Value: "$contactInfo.favorites"},
		bson.E{Key: "contacts", Value: "$contactInfo.contacts"},
	}

	// countPipeline: same match filters, but only $count (no sort/skip/limit/project)
	countPipeline := make(mongo.Pipeline, 0, len(matchStages)+1)
	for _, s := range matchStages {
		countPipeline = append(countPipeline, s)
	}
	countPipeline = append(countPipeline, bson.D{bson.E{Key: "$count", Value: "count"}})

	pipeline := mongo.Pipeline{
		// Pick the requesting user.
		{bson.E{Key: "$match", Value: bson.D{bson.E{Key: "_id", Value: userID}}}},
		{bson.E{Key: "$limit", Value: 1}},

		// Lookup 1: paginated suggestions with sort/skip/limit/project.
		{bson.E{Key: "$lookup", Value: bson.D{
			bson.E{Key: "from", Value: string(objects.UserColl)},
			bson.E{Key: "let", Value: letDoc},
			bson.E{Key: "pipeline", Value: lookupPipeline},
			bson.E{Key: "as", Value: "suggestions"},
		}}},

		// Lookup 2: true unfiltered total count (same match, just $count).
		{bson.E{Key: "$lookup", Value: bson.D{
			bson.E{Key: "from", Value: string(objects.UserColl)},
			bson.E{Key: "let", Value: letDoc},
			bson.E{Key: "pipeline", Value: countPipeline},
			bson.E{Key: "as", Value: "totalAgg"},
		}}},

		// Project: surface suggestions array and extract total count into a scalar.
		{bson.E{Key: "$project", Value: bson.D{
			bson.E{Key: "suggestions", Value: 1},
			bson.E{Key: "total", Value: bson.M{
				"$ifNull": []interface{}{
					bson.M{"$arrayElemAt": []interface{}{"$totalAgg.count", 0}},
					0,
				},
			}},
		}}},
	}

	var result []struct {
		Suggestions []models.UserSuggestion `bson:"suggestions"`
		Total       int64                   `bson:"total"`
	}

	cursor, err := coll.Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to aggregate user suggestions: %w", err)
	}
	defer cursor.Close(ctx)

	if err := cursor.All(ctx, &result); err != nil {
		return nil, 0, fmt.Errorf("failed to decode user suggestions: %w", err)
	}

	if len(result) == 0 {
		return []models.UserSuggestion{}, 0, nil
	}

	r := result[0]

	if r.Suggestions == nil {
		r.Suggestions = []models.UserSuggestion{}
	}

	return r.Suggestions, r.Total, nil
}

// GetUserDisplayInfoFromDB fetches user display info from database
func GetUserDisplayInfoFromDB(userID bson.ObjectID) (*models.UserDisplayInfo, error) {
	filter := bson.M{"_id": userID}
	projection := bson.M{
		"username":            1,
		"profile.displayName": 1,
		"profile.avatar":      1,
		"presence.status":     1,
		"presence.isOnline":   1,
		"presence.lastSeen":   1,
	}

	var user struct {
		ID       bson.ObjectID           `bson:"_id"`
		Username string                  `bson:"username"`
		Profile  models.UserProfileEmbed `bson:"profile"`
		Presence models.PresenceEmbed    `bson:"presence"`
	}

	err := objects.DB.Collection(string(objects.UserColl)).FindOne(
		context.Background(),
		filter,
		options.FindOne().SetProjection(projection),
	).Decode(&user)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("%w: %s", ErrUserNotFound, userID.Hex())
		}
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}

	return &models.UserDisplayInfo{
		Username:    user.Username,
		DisplayName: user.Profile.DisplayName,
	}, nil
}

// dropImmutableUpdateKeys removes backend/service-owned fields from an update
// map. This is a defense-in-depth guard layered on top of the controller
// whitelist; it ensures account state, credentials, contact graph, chat state,
// and timestamps can never be mutated via UpdateProfile.
func dropImmutableUpdateKeys(updateMap map[string]interface{}) {
	for key := range updateMap {
		if isImmutableUpdateKey(key) {
			delete(updateMap, key)
		}
	}
}

// isImmutableUpdateKey reports whether a dotted key targets a field the user
// must not change directly.
func isImmutableUpdateKey(key string) bool {
	immutablePrefixes := []string{
		"accountStatus.",
		"createdAt",
		"updatedAt",
		"passwordHash",
		"contactInfo.",
		"chats",
		"chatInvitations",
	}
	for _, prefix := range immutablePrefixes {
		if key == prefix || strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// sanitizeUpdateFields walks a dotted-path update map and replaces any empty-
// string/nil value for a required field with the previously stored value.
// Required fields (never allowed to be empty): username, email, profile.displayName.
func sanitizeUpdateFields(updateMap, existingDoc map[string]interface{}) {
	required := map[string]bool{
		"username":            true,
		"email":               true,
		"profile.displayName": true,
	}

	for key, newVal := range updateMap {
		if !required[key] {
			continue
		}

		if isEmptyValue(newVal) {
			prev := getNestedPath(existingDoc, key)
			if prev != nil && !isEmptyValue(prev) {
				updateMap[key] = prev
				continue
			}

			// Both incoming and stored values are empty — derive a default.
			if defaultVal := defaultForRequired(key, existingDoc, updateMap); defaultVal != "" {
				updateMap[key] = defaultVal
			}
			// username and email have no meaningful derivation; we leave
			// the empty string so the caller/controller sees the error
			// when it re-fetches the profile.
		}
	}
}

// isEmptyValue reports whether a value represents "empty" for our purposes.
// nil and zero-length trimmed strings are considered empty.
func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}

// getNestedPath walks a dotted key (e.g. "profile.displayName") through a
// nested map and returns the leaf value, or nil if any segment is missing.
func getNestedPath(doc map[string]interface{}, dottedKey string) interface{} {
	parts := strings.Split(dottedKey, ".")
	if len(parts) == 0 {
		return nil
	}

	current := doc
	for i, part := range parts {
		raw, ok := current[part]
		if !ok {
			return nil
		}

		// Last segment — return the value regardless of type.
		if i == len(parts)-1 {
			return raw
		}

		// Not the last segment — must be a map to continue walking.
		sub, ok := raw.(map[string]interface{})
		if !ok {
			return nil
		}
		current = sub
	}
	return nil
}

// defaultForRequired provides a reasonable default when the existing document
// also has no value for a required field.
func defaultForRequired(key string, existingDoc, updateMap map[string]interface{}) string {
	switch key {
	case "profile.displayName":
		// Fall back to username if available in the existing doc or update map.
		if uname := getNestedPath(existingDoc, "username"); uname != nil {
			if s := toString(uname); strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		if raw, ok := updateMap["username"]; ok {
			if s := toString(raw); strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		return "User"
	}
	return ""
}

// toString converts an interface{} to string safely.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// LogoutUser adds a token's JTI to the Redis blacklist so it cannot be reused.
// The TTL is set to the token's remaining lifetime; already-expired tokens are a no-op.
func LogoutUser(ctx context.Context, jti string, expiresAt time.Time) error {
	log := logger.WithContext(ctx)

	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		// Token already expired — nothing to blacklist.
		log.WithField("jti", jti).Info("Logout for already-expired token — skipping blacklist")
		return nil
	}

	blacklistKey := "auth:blacklist:" + jti

	if objects.RedisClient == nil {
		log.Warn("Redis client is nil — cannot blacklist token")
		return fmt.Errorf("redis client is not initialized")
	}

	if err := objects.RedisClient.Set(ctx, blacklistKey, "1", ttl).Err(); err != nil {
		log.WithError(err).WithField("jti", jti).Error("Failed to blacklist token")
		return fmt.Errorf("failed to blacklist token: %w", err)
	}

	log.WithField("jti", jti).Info("Token blacklisted successfully")
	return nil
}
