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
