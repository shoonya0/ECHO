package controller

import (
	"gin/internal/models"
	"gin/internal/services"
	"gin/internal/utils"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ HTTP ENDPOINTS FOR CHAT MANAGEMENT ============
func CreateDirectChatHTTP(ctx *gin.Context) {
	userInterface, exists := ctx.Get("user")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	user, ok := userInterface.(models.LoginUserResponse)
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "Invalid user data", nil)
		return
	}

	var request struct {
		UserID string `json:"userId" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid request data", err.Error())
		return
	}

	targetUserID, err := bson.ObjectIDFromHex(request.UserID)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid user ID format", nil)
		return
	}

	chat, err := services.CreateDirectChat(user, targetUserID)
	if err != nil {
		log.Printf("Failed to create direct chat: %v", err)
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to create chat", nil)
		return
	}

	utils.SuccessResponse(ctx, "Direct chat created successfully", chat)
}

// CreateGroupChatHTTP creates a group chat via HTTP endpoint
func CreateGroupChatHTTP(ctx *gin.Context) {
	// Get current user
	userInterface, exists := ctx.Get("user")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	user, ok := userInterface.(models.LoginUserResponse)
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "Invalid user data", nil)
		return
	}

	// Parse request
	var request struct {
		Name         string   `json:"name" binding:"required"`
		Description  string   `json:"description"`
		Participants []string `json:"participants" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid request data", err.Error())
		return
	}

	// Parse participant IDs
	var participantIDs []bson.ObjectID
	for _, idStr := range request.Participants {
		if id, err := bson.ObjectIDFromHex(idStr); err == nil {
			participantIDs = append(participantIDs, id)
		}
	}

	if len(participantIDs) == 0 {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "At least one valid participant is required", nil)
		return
	}

	// Create group chat
	chat, err := services.CreateGroupChat(user.ID, request.Name, request.Description, participantIDs)
	if err != nil {
		log.Printf("Failed to create group chat: %v", err)
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to create group chat", nil)
		return
	}

	utils.SuccessResponse(ctx, "Group chat created successfully", chat)
}

// GetChatMessagesHTTP retrieves chat messages via HTTP endpoint
func GetChatMessagesHTTP(ctx *gin.Context) {
	// Get current user
	_, exists := ctx.Get("user")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// Parse query parameters
	chatIDStr := ctx.Query("chatId")
	if chatIDStr == "" {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Chat ID is required", nil)
		return
	}

	chatID, err := bson.ObjectIDFromHex(chatIDStr)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid chat ID format", nil)
		return
	}

	// Parse pagination parameters
	limit := 50 // Default limit
	offset := 0 // Default offset

	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsedLimit := utils.ParseInt(limitStr, 50); parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if parsedOffset := utils.ParseInt(offsetStr, 0); parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Get chat with messages
	chat, messages, err := services.GetChatWithMessages(chatID, limit, offset)
	if err != nil {
		log.Printf("Failed to get chat messages: %v", err)
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get chat messages", nil)
		return
	}

	// Prepare response
	response := map[string]interface{}{
		"chat":     chat,
		"messages": messages,
		"pagination": map[string]interface{}{
			"limit":  limit,
			"offset": offset,
			"count":  len(messages),
		},
	}

	utils.SuccessResponse(ctx, "Chat messages retrieved successfully", response)
}

// GetHubStatsHTTP returns WebSocket hub statistics (for monitoring)
func GetHubStatsHTTP(ctx *gin.Context) {
	stats := services.GetHubStats()
	utils.SuccessResponse(ctx, "Hub statistics retrieved successfully", stats)
}

// GetGroupMessages retrieves group messages via HTTP endpoint
func GetGroupMessages(ctx *gin.Context) {
	chatInfo := models.ContactInfo{}
	if err := ctx.ShouldBindJSON(&chatInfo); err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid chat info", nil)
		return
	}

	chat, err := services.GetChat(chatInfo)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get chat", nil)
		return
	}
	utils.SuccessResponse(ctx, "Chat fetched successfully", chat)
}
