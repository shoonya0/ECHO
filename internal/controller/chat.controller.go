package controller

import (
	"gin/internal/models"
	"gin/internal/services"
	"gin/internal/utils"
	"gin/objects"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ HTTP ENDPOINTS FOR CHAT MANAGEMENT ============
func CreateDirectChatHTTP(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	targetUserID := ctx.Param("userId")
	if targetUserID == "" {
		log.Debug("user id is required")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "User ID is required", nil)
		return
	}

	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid user ID format", nil)
		return
	}

	chat, err := services.CreateDirectChat(reqCtx, targetObjectID)
	if err != nil {
		log.Debug("failed to create direct chat")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to create chat", nil)
		return
	}

	utils.SuccessResponse(ctx, "Direct chat created successfully", chat)
}

func CreateGroupChatHTTP(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	var request struct {
		Name         string   `json:"name" binding:"required"`
		Description  string   `json:"description"`
		Participants []string `json:"participants" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		log.Debug("invalid request data ", err)
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid request data", err.Error())
		return
	}

	var participantIDs []bson.ObjectID
	for _, idStr := range request.Participants {
		if id, err := bson.ObjectIDFromHex(idStr); err == nil {
			participantIDs = append(participantIDs, id)
		}
	}

	if len(participantIDs) == 0 {
		log.Debug("at least one valid participant is required")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "At least one valid participant is required", nil)
		return
	}

	chat, err := services.CreateGroupChat(reqCtx, request.Name, request.Description, participantIDs)
	if err != nil {
		log.Debug("failed to create group chat ", err)
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to create group chat", nil)
		return
	}

	utils.SuccessResponse(ctx, "Group chat created successfully", chat)
}

func AddGroupMembersHTTP(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	var request struct {
		UserIDs []string `json:"userIDs" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		log.Debug("invalid request data ", err)
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid request data", err.Error())
		return
	}

	chatIDStr := ctx.Param("groupID")
	if chatIDStr == "" {
		log.Debug("chat ID is required")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Chat ID is required", nil)
		return
	}

	chatID, err := bson.ObjectIDFromHex(chatIDStr)
	if err != nil {
		log.Debug("invalid chat ID format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid chat ID format", nil)
		return
	}

	var userIDs []bson.ObjectID
	for _, userIDStr := range request.UserIDs {
		userID, err := bson.ObjectIDFromHex(userIDStr)
		if err != nil {
			log.Debug("invalid user ID format")
			utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid user ID format", nil)
			return
		}
		userIDs = append(userIDs, userID)
	}

	user, ok := reqCtx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if !ok {
		log.Debug("user not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	err = services.AddGroupMember(reqCtx, user.ID, chatID, userIDs)
	if err != nil {
		log.Debug("failed to add group member ", err)
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to add group member", nil)
		return
	}

	utils.SuccessResponse(ctx, "Group member added successfully", nil)
}

func GetChatMessagesHTTP(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user not authenticated")
		return
	}

	chatIDStr := ctx.Param("chatID")
	if chatIDStr == "" {
		log.Debug("chat ID is required")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Chat ID is required", nil)
		return
	}

	chatID, err := bson.ObjectIDFromHex(chatIDStr)
	if err != nil {
		log.Debug("invalid chat ID format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid chat ID format", nil)
		return
	}

	limit := 50 // Default limit
	offset := 0 // Default offset

	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsedLimit := utils.ParseInt(limitStr, 50); parsedLimit > 0 && parsedLimit <= 10 {
			limit = parsedLimit
		}
	}

	if offsetStr := ctx.Query("offset"); offsetStr != "" {
		if parsedOffset := utils.ParseInt(offsetStr, 0); parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	messages, err := services.GetChatMessages(reqCtx, chatID, limit, offset)
	if err != nil {
		log.Debug("failed to get chat messages ", err)
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get chat messages", nil)
		return
	}

	totalPages := (len(messages) + limit - 1) / limit

	type PaginationResponse struct {
		Limit      int `json:"limit"`
		Offset     int `json:"offset"`
		Count      int `json:"count"`
		TotalCount int `json:"totalCount"`
		Page       int `json:"page"`
		TotalPages int `json:"totalPages"`
	}

	type ChatMessagesResponse struct {
		Chat       bson.ObjectID      `json:"chat"`
		Messages   []models.Message   `json:"messages"`
		Pagination PaginationResponse `json:"pagination"`
	}

	response := ChatMessagesResponse{
		Chat:     chatID,
		Messages: messages,
		Pagination: PaginationResponse{
			Limit:      limit,
			Offset:     offset,
			Count:      len(messages),
			TotalCount: len(messages),
			Page:       1,
			TotalPages: totalPages,
		},
	}

	utils.SuccessResponse(ctx, "Chat messages retrieved successfully", response)
}

// GetHubStatsHTTP returns WebSocket hub statistics (for monitoring)
func GetHubStatsHTTP(ctx *gin.Context) {
	stats := services.GetHubInstance().GetStats()
	utils.SuccessResponse(ctx, "Hub statistics retrieved successfully", stats)
}

// ================ Invites ================
func CreateInvite(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	user, ok := reqCtx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if !ok {
		log.Debug("user not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	chatIDStr := ctx.Param("groupID")
	if chatIDStr == "" {
		log.Debug("chat ID is required")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Chat ID is required", nil)
		return
	}

	chatID, err := bson.ObjectIDFromHex(chatIDStr)
	if err != nil {
		log.Debug("invalid chat ID format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid chat ID format", err.Error())
		return
	}

	_, err = services.CreateInvite(reqCtx, user.ID, chatID)
	if err != nil {
		log.Debug("failed to create invite ", err.Error())
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to create invite", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "Invite created successfully", nil)
}

func GetGroupInvites(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	user, ok := reqCtx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if !ok {
		log.Debug("user not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	chatIDStr := ctx.Param("groupID")
	if chatIDStr == "" {
		log.Debug("chat ID is required")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Chat ID is required", nil)
		return
	}

	chatID, err := bson.ObjectIDFromHex(chatIDStr)
	if err != nil {
		log.Debug("invalid chat ID format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid chat ID format", err.Error())
		return
	}

	invites, err := services.GetGroupInvites(reqCtx, user.ID, chatID)
	if err != nil {
		log.Debug("failed to get group invites ", err.Error())
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get group invites", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "Invites fetched successfully", invites)
}

func DeleteInvite(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	user, ok := reqCtx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if !ok {
		log.Debug("user not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	inviteIDStr := ctx.Param("inviteID")
	if inviteIDStr == "" {
		log.Debug("invite ID is required")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invite ID is required", nil)
		return
	}

	err := services.DeleteInvite(reqCtx, user.ID, inviteIDStr)
	if err != nil {
		log.Debug("failed to delete invite ", err.Error())
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to delete invite", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "Invite deleted successfully", nil)
}

func UpdateInviteStatus(ctx *gin.Context) {
	// this is done internally
	utils.SuccessResponse(ctx, "Invite status updated successfully", nil)
}

func GetJoinedUsersByInvite(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	user, ok := reqCtx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if !ok {
		log.Debug("user not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	inviteIDStr := ctx.Param("inviteID")
	if inviteIDStr == "" {
		log.Debug("invite ID is required")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invite ID is required", nil)
		return
	}

	joinedUsers, err := services.GetJoinedUsersByInvite(reqCtx, user.ID, inviteIDStr)
	if err != nil {
		log.Debug("failed to get joined users by invite ", err.Error())
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get joined users by invite", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "Joined users fetched successfully", joinedUsers)
}

func SendInviteToUser(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	user, ok := reqCtx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if !ok {
		log.Debug("user not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	inviteID := ctx.Param("inviteID")
	userIDStr := ctx.Param("userID")

	if inviteID == "" || userIDStr == "" {
		log.Debug("invite ID and user ID are required")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invite ID and user ID are required", nil)
		return
	}

	targetUserID, err := bson.ObjectIDFromHex(userIDStr)
	if err != nil {
		log.Debug("invalid user ID format " + err.Error())
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid user ID format", err.Error())
		return
	}

	err = services.SendInviteToUser(reqCtx, user.ID, inviteID, targetUserID)
	if err != nil {
		log.Debug("failed to send invite to user " + err.Error())
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to send invite to user", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "Invite sent successfully", nil)
}

func JoinGroupByInvite(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	user, ok := reqCtx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if !ok {
		log.Debug("user not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	inviteCode := ctx.Param("inviteCode")

	if inviteCode == "" {
		log.Debug("invite code is required")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invite code is required", nil)
		return
	}

	err := services.JoinGroupByInvite(reqCtx, user.ID, inviteCode)
	if err != nil {
		log.Debug("failed to join group by invite " + err.Error())
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to join group by invite", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "Joined group by invite successfully", nil)
}

func GetAllInvitesOfUser(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	user, ok := reqCtx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if !ok {
		log.Debug("user not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	invites, err := services.GetAllInvitesOfUser(reqCtx, user.ID)
	if err != nil {
		log.Debug("failed to get all invites of user " + err.Error())
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get all invites of user", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "All invites fetched successfully", invites)
}
