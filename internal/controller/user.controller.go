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

// Get the authenticated user’s profile.
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

// Update profile fields (display name, avatar URL, status message ,etc...).
func UpdateProfile(ctx *gin.Context) {
	// Parse the partial update request
	var updateReq models.UpdateUserRequest
	if err := ctx.ShouldBindJSON(&updateReq); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	// Get user ID from context (set by auth middleware)
	userID, ok := ctx.Get("userId")
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user id not found"})
		return
	}

	// Convert string ID to ObjectID
	objectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id format"})
		return
	}

	// Build update document from request
	updateDoc := services.BuildPartialUpdateDocument(updateReq)

	// Update user profile with only provided fields
	err = services.UpdateProfile(objectID, updateDoc)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get updated user data
	updatedUser, err := services.GetUserProfile(objectID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch updated profile"})
		return
	}

	// Return updated user data
	ctx.JSON(http.StatusOK, gin.H{
		"message": "Profile updated successfully",
		"user":    updatedUser,
	})
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
	// Convert string ID to ObjectID
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
	// userData, err := services.GetUserSuggestions(ctx)
	// if err != nil {
	// 	ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	// 	return
	// }
	ctx.JSON(http.StatusOK, gin.H{"message": "User suggestions"})
}

func GetNearbyUsers(ctx *gin.Context) {
	// userData, err := services.GetNearbyUsers(ctx)
	// if err != nil {
	// 	ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	// 	return
	// }
	ctx.JSON(http.StatusOK, gin.H{"message": "Nearby users"})
}

func GetPopularUsers(ctx *gin.Context) {
	// userData, err := services.GetPopularUsers(ctx)
	// if err != nil {
	// 	ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	// 	return
	// }
	ctx.JSON(http.StatusOK, gin.H{"message": "Popular users"})
}
