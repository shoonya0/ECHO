package controller

import (
	"context"
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
	userID, ok := ctx.Get("userId")
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	projection := bson.M{
		"_id":           1,
		"username":      1,
		"email":         1,
		"phone":         1,
		"presence":      1,
		"profile":       1,
		"accountStatus": 1,
	}

	objectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	userData, err := services.FindByID[models.GetProfileResponse](context.Background(), objects.DB.Collection(string(objects.UserColl)), bson.M{"_id": objectID}, projection)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get user profile", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "User profile fetched successfully", userData)
}

func UpdateProfile(ctx *gin.Context) {
	var updateReq models.UpdateUserRequest
	if err := ctx.ShouldBindJSON(&updateReq); err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid request format", err.Error())
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	objectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	updateDoc := services.BuildPartialDocument(updateReq)

	err = services.UpdateProfile(objectID, updateDoc)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to update profile", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "Profile updated successfully", nil)
}

func DeleteProfile(ctx *gin.Context) {
	userID, ok := ctx.Get("userId")
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	err := services.DeleteProfile(userID.(string))
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to delete profile", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "Profile deleted successfully", nil)
}

// ============ USER DISCOVERY & SEARCH ============
func GetUserProfile(ctx *gin.Context) {
	userID := ctx.Param("id")
	if userID == "" {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "user id is required", nil)
		return
	}

	objectID, err := bson.ObjectIDFromHex(userID)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", nil)
		return
	}

	userData, err := services.GetUserProfile(objectID)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get user profile", err.Error())
		return
	}
	utils.SuccessResponse(ctx, "User profile fetched successfully", userData)
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
