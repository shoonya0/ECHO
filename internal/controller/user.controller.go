package controller

import (
	"fmt"
	"gin/internal/models"
	"gin/internal/services"
	"gin/internal/utils"
	"gin/objects"
	"net/http"

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

	UserDataKey := reqCtx.Value(objects.UserDataKey).(models.LoginUserResponse)
	if UserDataKey == (models.LoginUserResponse{}) {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	utils.SuccessResponse(ctx, "User profile fetched successfully", UserDataKey)
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

	err = services.UpdateProfile(reqCtx, objectID, updateDoc)
	if err != nil {
		log.Debug("failed to update profile" + err.Error())
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to update profile", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "Profile updated successfully", nil)
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
		log.Debug("failed to delete profile" + err.Error())
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to delete profile", err.Error())
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
		log.Debug("invalid user id format" + err.Error())
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", nil)
		return
	}

	UserDataKey, err := services.GetUserProfile(reqCtx, objectID)
	if err != nil {
		log.Debug("failed to get user profile" + err.Error())
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get user profile", err.Error())
		return
	}

	log.Debug("user profile fetched successfully")
	utils.SuccessResponse(ctx, "User profile fetched successfully", UserDataKey)
}

func GetUserSuggestions(ctx *gin.Context) {
	utils.SuccessResponse(ctx, "User suggestions", nil)
}

func GetNearbyUsers(ctx *gin.Context) {
	utils.SuccessResponse(ctx, "Nearby users", nil)
}

func GetPopularUsers(ctx *gin.Context) {
	utils.SuccessResponse(ctx, "Popular users", nil)
}
