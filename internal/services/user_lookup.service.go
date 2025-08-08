package services

import (
	"context"
	"fmt"
	"gin/internal/models"
	"gin/objects"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ============ USER DISPLAY INFO LOOKUP SERVICE ============
// Provides efficient user data lookup with caching for UI display

var (
	userCache      = make(map[string]*models.UserDisplayInfo)
	cacheExpiry    = make(map[string]time.Time)
	cacheMutex     sync.RWMutex
	cacheTTL       = 5 * time.Minute
	cleanupTicker  *time.Ticker
	cleanupStarted bool
)

// InitUserLookupService initializes the user lookup service with cache cleanup
func InitUserLookupService() {
	if !cleanupStarted {
		cleanupTicker = time.NewTicker(1 * time.Minute)
		go cleanupExpiredCache()
		cleanupStarted = true
	}
}

// StopUserLookupService stops the cache cleanup ticker
func StopUserLookupService() {
	if cleanupTicker != nil {
		cleanupTicker.Stop()
		cleanupStarted = false
	}
}

// GetUserDisplayInfo retrieves user display information with caching
func GetUserDisplayInfo(userID bson.ObjectID) (*models.UserDisplayInfo, error) {
	userIDStr := userID.Hex()

	// Check cache first
	cacheMutex.RLock()
	if cachedInfo, exists := userCache[userIDStr]; exists {
		if expiry, hasExpiry := cacheExpiry[userIDStr]; hasExpiry && time.Now().Before(expiry) {
			cacheMutex.RUnlock()
			return cachedInfo, nil
		}
	}
	cacheMutex.RUnlock()

	// Fetch from database
	userInfo, err := fetchUserDisplayInfoFromDB(userID)
	if err != nil {
		return nil, err
	}

	// Cache the result
	cacheMutex.Lock()
	userCache[userIDStr] = userInfo
	cacheExpiry[userIDStr] = time.Now().Add(cacheTTL)
	cacheMutex.Unlock()

	return userInfo, nil
}

// GetMultipleUserDisplayInfo retrieves multiple users' display information efficiently
func GetMultipleUserDisplayInfo(userIDs []bson.ObjectID) (map[string]*models.UserDisplayInfo, error) {
	result := make(map[string]*models.UserDisplayInfo)
	var missingIDs []bson.ObjectID

	// Check cache for existing entries
	cacheMutex.RLock()
	for _, userID := range userIDs {
		userIDStr := userID.Hex()
		if cachedInfo, exists := userCache[userIDStr]; exists {
			if expiry, hasExpiry := cacheExpiry[userIDStr]; hasExpiry && time.Now().Before(expiry) {
				result[userIDStr] = cachedInfo
				continue
			}
		}
		missingIDs = append(missingIDs, userID)
	}
	cacheMutex.RUnlock()

	// Fetch missing entries from database
	if len(missingIDs) > 0 {
		fetchedUsers, err := fetchMultipleUserDisplayInfoFromDB(missingIDs)
		if err != nil {
			return nil, err
		}

		// Cache fetched results
		cacheMutex.Lock()
		for userIDStr, userInfo := range fetchedUsers {
			result[userIDStr] = userInfo
			userCache[userIDStr] = userInfo
			cacheExpiry[userIDStr] = time.Now().Add(cacheTTL)
		}
		cacheMutex.Unlock()
	}

	return result, nil
}

// InvalidateUserCache removes a user from cache (call when user data changes)
func InvalidateUserCache(userID bson.ObjectID) {
	userIDStr := userID.Hex()
	cacheMutex.Lock()
	delete(userCache, userIDStr)
	delete(cacheExpiry, userIDStr)
	cacheMutex.Unlock()
}

// BuildParticipantEmbedFromRef builds a ParticipantEmbed from ParticipantRef with user lookup
func BuildParticipantEmbedFromRef(participantRef models.ParticipantRef) (*models.ParticipantEmbed, error) {
	userInfo, err := GetUserDisplayInfo(participantRef.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user display info: %w", err)
	}

	return &models.ParticipantEmbed{
		UserID:      participantRef.UserID,
		Username:    userInfo.Username,
		DisplayName: userInfo.DisplayName,
		Avatar:      userInfo.Avatar,
		Role:        participantRef.Role,
		IsBlocked:   participantRef.IsBlocked,
		JoinedAt:    participantRef.JoinedAt,
		LastActive:  participantRef.LastActive,
		IsMuted:     participantRef.IsMuted,
		Permissions: participantRef.Permissions,
	}, nil
}

// BuildMultipleParticipantEmbeds builds multiple ParticipantEmbeds efficiently
func BuildMultipleParticipantEmbeds(participantRefs map[string]models.ParticipantRef) (map[string]models.ParticipantEmbed, error) {
	// Extract user IDs
	userIDs := make([]bson.ObjectID, 0, len(participantRefs))
	for _, ref := range participantRefs {
		userIDs = append(userIDs, ref.UserID)
	}

	// Get user display info for all participants
	userInfoMap, err := GetMultipleUserDisplayInfo(userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get multiple user display info: %w", err)
	}

	// Build participant embeds
	result := make(map[string]models.ParticipantEmbed)
	for userIDStr, ref := range participantRefs {
		userInfo, exists := userInfoMap[userIDStr]
		if !exists {
			return nil, fmt.Errorf("user info not found for user ID: %s", userIDStr)
		}

		result[userIDStr] = models.ParticipantEmbed{
			UserID:      ref.UserID,
			Username:    userInfo.Username,
			DisplayName: userInfo.DisplayName,
			Avatar:      userInfo.Avatar,
			Role:        ref.Role,
			IsBlocked:   ref.IsBlocked,
			JoinedAt:    ref.JoinedAt,
			LastActive:  ref.LastActive,
			IsMuted:     ref.IsMuted,
			Permissions: ref.Permissions,
		}
	}

	return result, nil
}

// ============ PRIVATE HELPER FUNCTIONS ============

// fetchUserDisplayInfoFromDB fetches user display info from database
func fetchUserDisplayInfoFromDB(userID bson.ObjectID) (*models.UserDisplayInfo, error) {
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
			return nil, fmt.Errorf("user not found: %s", userID.Hex())
		}
		return nil, fmt.Errorf("failed to fetch user: %w", err)
	}

	return &models.UserDisplayInfo{
		UserID:      user.ID,
		Username:    user.Username,
		DisplayName: user.Profile.DisplayName,
		Avatar:      user.Profile.Avatar,
		Status:      user.Presence.Status,
		IsOnline:    user.Presence.IsOnline,
		LastSeen:    user.Presence.LastSeen,
	}, nil
}

// fetchMultipleUserDisplayInfoFromDB fetches multiple users' display info from database
func fetchMultipleUserDisplayInfoFromDB(userIDs []bson.ObjectID) (map[string]*models.UserDisplayInfo, error) {
	filter := bson.M{"_id": bson.M{"$in": userIDs}}
	projection := bson.M{
		"username":            1,
		"profile.displayName": 1,
		"profile.avatar":      1,
		"presence.status":     1,
		"presence.isOnline":   1,
		"presence.lastSeen":   1,
	}

	cursor, err := objects.DB.Collection(string(objects.UserColl)).Find(
		context.Background(),
		filter,
		options.Find().SetProjection(projection),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch users: %w", err)
	}
	defer cursor.Close(context.Background())

	result := make(map[string]*models.UserDisplayInfo)
	for cursor.Next(context.Background()) {
		var user struct {
			ID       bson.ObjectID           `bson:"_id"`
			Username string                  `bson:"username"`
			Profile  models.UserProfileEmbed `bson:"profile"`
			Presence models.PresenceEmbed    `bson:"presence"`
		}

		if err := cursor.Decode(&user); err != nil {
			return nil, fmt.Errorf("failed to decode user: %w", err)
		}

		userInfo := &models.UserDisplayInfo{
			UserID:      user.ID,
			Username:    user.Username,
			DisplayName: user.Profile.DisplayName,
			Avatar:      user.Profile.Avatar,
			Status:      user.Presence.Status,
			IsOnline:    user.Presence.IsOnline,
			LastSeen:    user.Presence.LastSeen,
		}

		result[user.ID.Hex()] = userInfo
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return result, nil
}

// cleanupExpiredCache removes expired entries from cache
func cleanupExpiredCache() {
	for range cleanupTicker.C {
		now := time.Now()
		cacheMutex.Lock()
		for userID, expiry := range cacheExpiry {
			if now.After(expiry) {
				delete(userCache, userID)
				delete(cacheExpiry, userID)
			}
		}
		cacheMutex.Unlock()
	}
}

// GetCacheStats returns cache statistics for monitoring
func GetCacheStats() map[string]interface{} {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()

	return map[string]interface{}{
		"totalCached": len(userCache),
		"cacheSize":   len(userCache),
		"timestamp":   time.Now(),
	}
}
