package controller

import (
	"gin/internal/models"
	"gin/internal/services"
	"gin/internal/utils"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// GetPresence handles GET /presence - Get current user presence
func GetPresence(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	presence, err := services.PresenceServiceInstance.GetPresence(c)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "Presence retrieved successfully", presence)
}

// UpdatePresence handles PUT /presence - Update presence status
func UpdatePresence(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	var req models.UpdatePresenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request body: "+err.Error(), nil)
		return
	}

	err := services.PresenceServiceInstance.UpdatePresence(c, &req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "Presence updated successfully", nil)
}

// SetCustomStatus handles POST /custom - Set custom status message
func SetCustomStatus(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	var req models.SetCustomStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request body: "+err.Error(), nil)
		return
	}

	err := services.PresenceServiceInstance.SetCustomStatus(c, &req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "Custom status set successfully", nil)
}

// ClearCustomStatus handles DELETE /custom - Clear custom status message
func ClearCustomStatus(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	err := services.PresenceServiceInstance.ClearCustomStatus(c)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "Custom status cleared successfully", nil)
}

// GetStatusHistory handles GET /history - Get status history
func GetStatusHistory(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	history, err := services.PresenceServiceInstance.GetStatusHistory(c)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "Status history retrieved successfully", history)
}

// UpdateAvailability handles PUT /availability - Update availability
func UpdateAvailability(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	var req models.UpdateAvailabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request body: "+err.Error(), nil)
		return
	}

	err := services.PresenceServiceInstance.UpdateAvailability(c, &req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "Availability updated successfully", nil)
}

// GetUserPresence handles GET /users/:id/presence - Get specific user's presence
func GetUserPresence(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	userID := c.Param("id")
	if userID == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "User ID is required", nil)
		return
	}

	presence, err := services.PresenceServiceInstance.GetUserPresence(userID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "User presence retrieved successfully", presence)
}

// GetOnlineUsers handles GET /online - Get all online users (admin endpoint)
func GetOnlineUsers(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	users, err := services.PresenceServiceInstance.GetOnlineUsers()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	response := map[string]interface{}{
		"online_users": users,
		"count":        len(users),
	}

	utils.SuccessResponse(c, "Online users retrieved successfully", response)
}

// GetAllPresences handles GET /all - Get all user presences (admin endpoint)
func GetAllPresences(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	presences, err := services.PresenceServiceInstance.GetAllPresences()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	response := map[string]interface{}{
		"presences": presences,
		"count":     len(presences),
	}

	utils.SuccessResponse(c, "All presences retrieved successfully", response)
}

// UpdateUserActivity handles POST /activity - Update user activity (middleware endpoint)
func UpdateUserActivity(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	userID, ok := c.Get("user_id")
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User ID not found in context", nil)
		return
	}

	err := services.PresenceServiceInstance.UpdateActivity(userID.(string))
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "Activity updated successfully", nil)
}

// HandleUserLogin handles user login event (called from auth service)
func HandleUserLogin(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	userID, ok := c.Get("user_id")
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User ID not found in context", nil)
		return
	}

	// Get device info from headers or request
	deviceInfo := c.GetHeader("User-Agent")

	err := services.PresenceServiceInstance.UserLogin(userID.(string), &deviceInfo)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "User login handled successfully", nil)
}

// HandleUserLogout handles user logout event (called from auth service)
func HandleUserLogout(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	userID, ok := c.Get("user_id")
	if !ok {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User ID not found in context", nil)
		return
	}

	err := services.PresenceServiceInstance.UserLogout(userID.(string))
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "User logout handled successfully", nil)
}

// GetPresenceStats handles GET /stats - Get presence statistics (admin endpoint)
func GetPresenceStats(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	// Get all presences
	presences, err := services.PresenceServiceInstance.GetAllPresences()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	// Calculate statistics
	stats := map[string]int{
		"total":     len(presences),
		"online":    0,
		"away":      0,
		"dnd":       0,
		"invisible": 0,
		"offline":   0,
	}

	for _, presence := range presences {
		switch presence.Status {
		case models.StatusOnline:
			stats["online"]++
		case models.StatusAway:
			stats["away"]++
		case models.StatusDND:
			stats["dnd"]++
		case models.StatusInvisible:
			stats["invisible"]++
		case models.StatusOffline:
			stats["offline"]++
		}
	}

	utils.SuccessResponse(c, "Presence statistics retrieved successfully", stats)
}

// CleanupInactiveUsers handles POST /cleanup - Cleanup inactive users (admin endpoint)
func CleanupInactiveUsers(c *gin.Context) {
	// Ensure presence service is initialized
	if services.PresenceServiceInstance == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Presence service not initialized", nil)
		return
	}

	// Get threshold from query parameter (default 15 minutes)
	thresholdStr := c.DefaultQuery("threshold", "15")
	threshold, err := strconv.Atoi(thresholdStr)
	if err != nil || threshold <= 0 {
		threshold = 15
	}

	inactiveThreshold := time.Duration(threshold) * time.Minute

	err = services.PresenceServiceInstance.CleanupInactiveUsers(inactiveThreshold)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	response := map[string]interface{}{
		"message":           "Inactive users cleanup completed",
		"threshold_minutes": threshold,
		"cleanup_completed": true,
	}

	utils.SuccessResponse(c, "Cleanup completed successfully", response)
}
