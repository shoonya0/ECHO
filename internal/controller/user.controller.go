package controller

import (
	"fmt"
	"gin/internal/models"
	"gin/internal/services"
	"gin/internal/utils"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ PROFILE MANAGEMENT ============
func GetProfile(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		fmt.Println("error in reducing gin context to context or logger")
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	objectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	profile, err := services.GetUserBasicInfo(reqCtx, objectID)
	if err != nil {
		log.Debug("failed to get user profile: " + err.Error())
		status, msg := MapServiceErrorToHTTP(err)
		utils.ErrorResponse(ctx, status, msg, nil)
		return
	}

	utils.SuccessResponse(ctx, "User profile fetched successfully", profile)
}

func UpdateProfile(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		fmt.Println("error in reducing gin context to context or logger")
		return
	}

	var updateReq models.UpdateUserRequest
	if err := ctx.ShouldBindJSON(&updateReq); err != nil {
		log.Debug("invalid request format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid request format", err.Error())
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	objectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	updateDoc := BuildPartialDocument(updateReq)

	// Drop backend/service-owned fields (accountStatus, timestamps, etc.)
	// before they can reach MongoDB. Any client attempt to set them is logged.
	sanitizedDoc, droppedKeys := FilterProfileUpdateKeys(updateDoc)
	if len(droppedKeys) > 0 {
		log.WithField("droppedKeys", droppedKeys).Warn("Dropped immutable fields from profile update")
	}

	err = services.UpdateProfile(reqCtx, objectID, sanitizedDoc)
	if err != nil {
		log.Debug("failed to update profile: " + err.Error())
		status, msg := MapServiceErrorToHTTP(err)
		utils.ErrorResponse(ctx, status, msg, nil)
		return
	}

	profile, err := services.GetUserBasicInfo(reqCtx, objectID)
	if err != nil {
		log.Debug("failed to get updated profile: " + err.Error())
		status, msg := MapServiceErrorToHTTP(err)
		utils.ErrorResponse(ctx, status, msg, nil)
		return
	}

	utils.SuccessResponse(ctx, "Profile updated successfully", profile)
}

func DeleteProfile(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		fmt.Println("error in reducing gin context to context or logger")
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	err := services.DeleteProfile(reqCtx, userID.(string))
	if err != nil {
		log.Debug("failed to delete profile: " + err.Error())
		status, msg := MapServiceErrorToHTTP(err)
		utils.ErrorResponse(ctx, status, msg, nil)
		return
	}

	log.Debug("profile deleted successfully")
	utils.SuccessResponse(ctx, "Profile deleted successfully", nil)
}

// ============ USER DISCOVERY & SEARCH ============
func GetUserProfile(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		fmt.Println("error in reducing gin context to context or logger")
		return
	}

	userID := ctx.Param("id")
	if userID == "" {
		log.Debug("user id is required")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "user id is required", nil)
		return
	}

	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		log.Debug("invalid user id format: " + err.Error())
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", nil)
		return
	}

	UserDataKey, err := services.GetUserProfile(reqCtx, objectID)
	if err != nil {
		log.Debug("failed to get user profile: " + err.Error())
		status, msg := MapServiceErrorToHTTP(err)
		utils.ErrorResponse(ctx, status, msg, nil)
		return
	}

	log.Debug("user profile fetched successfully")
	utils.SuccessResponse(ctx, "User profile fetched successfully", UserDataKey)
}

func GetUserSuggestions(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		return
	}

	userID, ok := authUserID(ctx)
	if !ok {
		return
	}

	// Parse page (1-based, min 1).
	page, err := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	// Parse limit (default 20, max 50).
	limit, err := strconv.Atoi(ctx.DefaultQuery("limit", "20"))
	if err != nil || limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	// Parse optional comma-separated excludeIds.
	var excludeIDs []bson.ObjectID
	if raw := ctx.Query("excludeIds"); raw != "" {
		parts := utils.SplitAndTrim(raw, ",")
		for _, p := range parts {
			id, err := bson.ObjectIDFromHex(p)
			if err == nil {
				excludeIDs = append(excludeIDs, id)
			}
		}
	}

	results, total, err := services.GetUserSuggestions(reqCtx, userID, page, limit, excludeIDs)
	if err != nil {
		log.Debug("failed to get user suggestions: " + err.Error())
		status, msg := MapServiceErrorToHTTP(err)
		utils.ErrorResponse(ctx, status, msg, nil)
		return
	}

	utils.PaginatedResponse(ctx, "User suggestions fetched successfully", results, limit, int(total))
}

func GetNearbyUsers(ctx *gin.Context) {
	utils.SuccessResponse(ctx, "Nearby users", nil)
}

func GetPopularUsers(ctx *gin.Context) {
	utils.SuccessResponse(ctx, "Popular users", nil)
}
