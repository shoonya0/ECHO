package controller

import (
	"gin/internal/models"
	"gin/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ============ PROFILE MANAGEMENT ============

// Get the authenticated user’s profile.
func GetProfile(ctx *gin.Context) {
	userData, err := services.GetProfile(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, userData)
}

// Update profile fields (display name, avatar URL, status message).
func UpdateProfile(ctx *gin.Context) {
	user := models.User{}
	if err := ctx.ShouldBindJSON(&user); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userData, err := services.UpdateProfile(ctx, user)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, userData)
}

// ============ USER DISCOVERY & SEARCH ============
func GetUserProfile(ctx *gin.Context) {
	userData, err := services.GetUserProfile(ctx)
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

func GetRecentUsers(ctx *gin.Context) {
	userData, err := services.GetRecentUsers(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, userData)
}

// ============ CONTACTS & FRIENDS MANAGEMENT ============
func GetContacts(ctx *gin.Context) {
	userData, err := services.GetContacts(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, userData)
}
