package controller

import (
	"context"
	"gin/internal/models"
	"gin/internal/services"
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
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user id not found"})
		return
	}

	projection := bson.M{
		"_id":                      1,
		"username":                 1,
		"email":                    1,
		"phone":                    1,
		"presence":                 1,
		"profile":                  1,
		"accountStatus.isVerified": 1,
	}

	objectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userData, err := services.FindByID[models.GetUserProfile](context.Background(), objects.DB.Collection(string(objects.UserColl)), bson.M{"_id": objectID}, projection)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, userData)
}

// Update profile fields (display name, avatar URL, status message ,etc...).
func UpdateProfile(ctx *gin.Context) {
	var err error
	user := models.User{}
	if err := ctx.ShouldBindJSON(&user); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID, ok := ctx.Get("userId")
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user id not found"})
		return
	}
	user.ID, err = bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userData, err := services.UpdateProfile(user)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, userData)
}

// ============ USER DISCOVERY & SEARCH ============
func GetUserProfile(ctx *gin.Context) {
	userID := ctx.Param("id")
	if userID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "user id is required"})
		return
	}
	userData, err := services.GetUserProfile(userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, userData)
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
