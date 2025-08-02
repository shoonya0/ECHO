package services

import (
	"context"
	"fmt"
	"gin/internal/cache"
	"gin/internal/models"
	"gin/objects"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// PresenceService handles all presence-related operations
type PresenceService struct {
	pubsub *cache.PresencePubSubManager
}

// NewPresenceService creates a new presence service
func NewPresenceService() *PresenceService {
	return &PresenceService{
		pubsub: cache.PresenceManager,
	}
}

// GetPresence gets current user presence
func (ps *PresenceService) GetPresence(c *gin.Context) (*models.PresenceResponse, error) {
	userID, ok := c.Get("user_id")
	if !ok {
		return nil, fmt.Errorf("user id not found in context")
	}

	presence, err := ps.pubsub.GetPresence(userID.(string))
	if err != nil {
		if err == redis.Nil {
			// User presence not found, create default offline presence
			return &models.PresenceResponse{
				UserID:       userID.(string),
				IsOnline:     false,
				Status:       models.StatusOffline,
				LastSeen:     time.Now(),
				LastActivity: time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("failed to get presence: %w", err)
	}

	// Convert to response model
	response := &models.PresenceResponse{
		UserID:   presence.UserID,
		IsOnline: presence.IsOnline,
		Status:   presence.Status,
		// CustomStatus: presence.CustomStatus,
		LastSeen: presence.LastSeen,
		// LastActivity: presence.LastActivity,
	}

	// Apply visibility rules for invisible users
	if presence.Status == models.StatusInvisible {
		response.IsOnline = false
		response.Status = models.StatusOffline
	}

	return response, nil
}

// UpdatePresence updates user presence status
func (ps *PresenceService) UpdatePresence(c *gin.Context, req *models.UpdatePresenceRequest) error {
	userID, ok := c.Get("user_id")
	if !ok {
		return fmt.Errorf("user id not found in context")
	}

	// Validate status
	if !isValidStatus(*req.Status) {
		return fmt.Errorf("invalid status: %s", *req.Status)
	}

	// Get custom status and device info
	customStatus := ""
	if req.CustomStatus != nil {
		customStatus = *req.CustomStatus
	}

	// Update presence using pub/sub manager
	err := ps.pubsub.UpdatePresence(
		userID.(string),
		*req.Status,
		customStatus,
		req.DeviceInfo,
		req.Location,
	)
	if err != nil {
		return fmt.Errorf("failed to update presence: %w", err)
	}

	// Also update user activity
	ps.pubsub.UpdateActivity(userID.(string))

	// Optionally sync to database for persistence
	go ps.syncPresenceToDatabase(userID.(string))

	return nil
}

// SetCustomStatus sets or updates custom status
func (ps *PresenceService) SetCustomStatus(c *gin.Context, req *models.SetCustomStatusRequest) error {
	userID, ok := c.Get("user_id")
	if !ok {
		return fmt.Errorf("user id not found in context")
	}

	err := ps.pubsub.SetCustomStatus(userID.(string), req.CustomStatus)
	if err != nil {
		return fmt.Errorf("failed to set custom status: %w", err)
	}

	// Update activity
	ps.pubsub.UpdateActivity(userID.(string))

	return nil
}

// ClearCustomStatus clears user's custom status
func (ps *PresenceService) ClearCustomStatus(c *gin.Context) error {
	userID, ok := c.Get("user_id")
	if !ok {
		return fmt.Errorf("user id not found in context")
	}

	err := ps.pubsub.ClearCustomStatus(userID.(string))
	if err != nil {
		return fmt.Errorf("failed to clear custom status: %w", err)
	}

	// Update activity
	ps.pubsub.UpdateActivity(userID.(string))

	return nil
}

// GetStatusHistory gets user's presence history
func (ps *PresenceService) GetStatusHistory(c *gin.Context) (*models.PresenceHistoryResponse, error) {
	userID, ok := c.Get("user_id")
	if !ok {
		return nil, fmt.Errorf("user id not found in context")
	}

	// Get pagination parameters
	page := 1
	limit := 50

	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Get history from Redis (recent entries)
	history, err := ps.pubsub.GetStatusHistory(userID.(string), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get status history: %w", err)
	}

	response := &models.PresenceHistoryResponse{
		History:    history,
		TotalCount: int64(len(history)),
		Page:       page,
		Limit:      limit,
	}

	return response, nil
}

// UpdateAvailability updates user availability status
func (ps *PresenceService) UpdateAvailability(c *gin.Context, req *models.UpdateAvailabilityRequest) error {
	userID, ok := c.Get("user_id")
	if !ok {
		return fmt.Errorf("user id not found in context")
	}

	// Validate status
	if !isValidStatus(req.Status) {
		return fmt.Errorf("invalid status: %s", req.Status)
	}

	// Get current presence to preserve custom status
	currentPresence, err := ps.pubsub.GetPresence(userID.(string))
	customStatus := ""
	if err == nil {
		customStatus = currentPresence.Status
	}

	// Update presence
	err = ps.pubsub.UpdatePresence(
		userID.(string),
		req.Status,
		customStatus,
		nil, // Don't update device info
		nil, // Don't update location
	)
	if err != nil {
		return fmt.Errorf("failed to update availability: %w", err)
	}

	// Update activity
	ps.pubsub.UpdateActivity(userID.(string))

	return nil
}

// UserLogin handles user login event
func (ps *PresenceService) UserLogin(userID string, deviceInfo *string) error {
	err := ps.pubsub.UserLogin(userID, deviceInfo)
	if err != nil {
		return fmt.Errorf("failed to handle user login: %w", err)
	}

	// Sync to database
	go ps.syncPresenceToDatabase(userID)

	return nil
}

// UserLogout handles user logout event
func (ps *PresenceService) UserLogout(userID string) error {
	err := ps.pubsub.UserLogout(userID)
	if err != nil {
		return fmt.Errorf("failed to handle user logout: %w", err)
	}

	// Sync to database
	go ps.syncPresenceToDatabase(userID)

	return nil
}

// UpdateActivity updates user's last activity
func (ps *PresenceService) UpdateActivity(userID string) error {
	return ps.pubsub.UpdateActivity(userID)
}

// GetOnlineUsers gets list of currently online users
func (ps *PresenceService) GetOnlineUsers() ([]string, error) {
	return ps.pubsub.GetOnlineUsers()
}

// GetAllPresences gets all user presences (admin function)
func (ps *PresenceService) GetAllPresences() (map[string]models.PresenceStatus, error) {
	return ps.pubsub.GetAllPresences()
}

// GetUserPresence gets specific user's presence (for other users to see)
func (ps *PresenceService) GetUserPresence(userID string) (*models.PresenceResponse, error) {
	presence, err := ps.pubsub.GetPresence(userID)
	if err != nil {
		if err == redis.Nil {
			return &models.PresenceResponse{
				UserID:   userID,
				IsOnline: false,
				Status:   models.StatusOffline,
				LastSeen: time.Now(),
			}, nil
		}
		return nil, fmt.Errorf("failed to get user presence: %w", err)
	}

	response := &models.PresenceResponse{
		UserID:   presence.UserID,
		IsOnline: presence.IsOnline,
		Status:   presence.Status,
		// CustomStatus: presence.Status,
		LastSeen: presence.LastSeen,
		// LastActivity: presence.LastActivity,
	}

	// Apply visibility rules for invisible users
	if presence.Status == models.StatusInvisible {
		response.IsOnline = false
		response.Status = models.StatusOffline
		response.CustomStatus = "" // Hide custom status for invisible users
	}

	return response, nil
}

// SubscribeToPresenceEvents subscribes to presence events for real-time updates
func (ps *PresenceService) SubscribeToPresenceEvents(subscriberID string) <-chan models.PresenceEvent {
	return ps.pubsub.Subscribe(subscriberID)
}

// UnsubscribeFromPresenceEvents unsubscribes from presence events
func (ps *PresenceService) UnsubscribeFromPresenceEvents(subscriberID string) {
	ps.pubsub.Unsubscribe(subscriberID)
}

// syncPresenceToDatabase syncs presence data to MongoDB for persistence
func (ps *PresenceService) syncPresenceToDatabase(userID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get current presence from Redis
	presence, err := ps.pubsub.GetPresence(userID)
	if err != nil {
		return // Skip if can't get presence
	}

	// Update user document in MongoDB
	filter := bson.M{"user_id": userID}
	update := bson.M{
		"$set": bson.M{
			"status": presence.Status,
			// "status_message": presence.CustomStatus,
			"is_online":  presence.IsOnline,
			"last_seen":  presence.LastSeen,
			"updated_at": time.Now(),
		},
	}

	collection := objects.DBClient.Database("ECHO").Collection("users")
	_, err = collection.UpdateOne(ctx, filter, update)
	if err != nil {
		// Log error but don't fail the operation
		fmt.Printf("Failed to sync presence to database for user %s: %v\n", userID, err)
	}
}

// CleanupInactiveUsers marks inactive users as away
func (ps *PresenceService) CleanupInactiveUsers(inactiveThreshold time.Duration) error {
	return ps.pubsub.CleanupInactiveUsers(inactiveThreshold)
}

// isValidStatus validates presence status
func isValidStatus(status string) bool {
	validStatuses := []string{
		models.StatusOnline,
		models.StatusAway,
		models.StatusDND,
		models.StatusInvisible,
		models.StatusOffline,
	}

	for _, validStatus := range validStatuses {
		if status == validStatus {
			return true
		}
	}
	return false
}

// Global presence service instance
var PresenceServiceInstance *PresenceService

// InitializePresenceService initializes the global presence service
func InitializePresenceService() {
	PresenceServiceInstance = NewPresenceService()
}

// Legacy functions for backward compatibility (deprecated)
func GetPresence(c *gin.Context) (string, error) {
	if PresenceServiceInstance == nil {
		return "", fmt.Errorf("presence service not initialized")
	}

	response, err := PresenceServiceInstance.GetPresence(c)
	if err != nil {
		return "", err
	}

	return response.Status, nil
}

func UpdatePresence(c *gin.Context) (string, error) {
	if PresenceServiceInstance == nil {
		return "", fmt.Errorf("presence service not initialized")
	}

	status := c.Query("status")
	if status == "" {
		return "", fmt.Errorf("status parameter is required")
	}

	req := &models.UpdatePresenceRequest{
		Status: &status,
	}

	err := PresenceServiceInstance.UpdatePresence(c, req)
	if err != nil {
		return "", err
	}

	return "presence updated", nil
}
