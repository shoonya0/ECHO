package controller

import (
	"gin/internal/services"
	"gin/internal/utils"
	"gin/objects"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func getStatus(ctx *gin.Context) objects.ContactStatus {
	status := ctx.Query("status")
	switch status {
	case "pending":
		return objects.StatusPending
	case "accepted":
		return objects.StatusAccepted
	case "blocked":
		return objects.StatusBlocked
	case "favorite":
		return objects.StatusFavorite
	case "contact":
		return objects.StatusContact
	default:
		return objects.StatusContact
	}
}

// ============ CONTACTS & FRIENDS MANAGEMENT ============
// Contact List Management

func GetUsersContacts(ctx *gin.Context) {
	userID, ok := ctx.Get("userId")
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		limit = 10
	}

	userData, err := services.GetContacts(userID.(string), objects.StatusAccepted, limit)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get users contacts", err.Error())
		return
	}
	utils.PaginatedResponse(ctx, "Users contacts fetched successfully", userData, limit, len(userData))
}

func GetContactRequests(ctx *gin.Context) {
	userID, ok := ctx.Get("userId")
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}
	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		limit = 10
	}
	userData, err := services.GetContacts(userID.(string), objects.StatusPending, limit)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get contact requests", err.Error())
		return
	}
	utils.PaginatedResponse(ctx, "Contact requests fetched successfully", userData, limit, len(userData))
}

func GetSentContactRequests(ctx *gin.Context) {
	userID, ok := ctx.Get("userId")
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}
	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		limit = 10
	}
	userData, err := services.GetContacts(userID.(string), objects.StatusPending, limit)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get sent contact requests", err.Error())
		return
	}
	utils.PaginatedResponse(ctx, "Sent contact requests fetched successfully", userData, limit, len(userData))
}

func GetBlockedUsers(ctx *gin.Context) {
	userID, ok := ctx.Get("userId")
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}
	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		limit = 10
	}
	userData, err := services.GetContacts(userID.(string), objects.StatusBlocked, limit)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get blocked users", err.Error())
		return
	}
	utils.PaginatedResponse(ctx, "Blocked users fetched successfully", userData, limit, len(userData))
}

func GetFavoriteContacts(ctx *gin.Context) {
	userID, ok := ctx.Get("userId")
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}
	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		limit = 10
	}
	userData, err := services.GetContacts(userID.(string), objects.StatusFavorite, limit)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get favorite contacts", err.Error())
		return
	}
	utils.PaginatedResponse(ctx, "Favorite contacts fetched successfully", userData, limit, len(userData))
}

// Contact Actions
func SendContactRequest(ctx *gin.Context) {
	userID, ok := ctx.Get("userId")
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}
	targetUserID := ctx.Param("targetUserId")
	userData, err := services.SendContactRequest(userID.(string), targetUserID)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to send contact request", err.Error())
		return
	}
	utils.SuccessResponse(ctx, "Contact request sent successfully", userData)
}

// Accept or decline a request ({ action: "accept"|"decline" }).
func AcceptOrDeclineContactRequest(ctx *gin.Context) {
	userID, ok := ctx.Get("userId")
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}
	requestId := ctx.Param("requestId")
	action := ctx.Query("action")
	if action != "accept" && action != "decline" {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid action", nil)
		return
	}
	userData, err := services.AcceptOrDeclineContactRequest(userID.(string), requestId, action)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to accept or decline contact request", err.Error())
		return
	}
	utils.SuccessResponse(ctx, "Contact request accepted or declined successfully", userData)
}

// // // Remove a contact.
// // func RemoveContact(c *gin.Context) {
// // 	userData, err := services.RemoveContact(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }

// // // Block a user.
// // func BlockUser(c *gin.Context) {
// // 	userData, err := services.BlockUser(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }

// // // Unblock a user.
// // func UnblockUser(c *gin.Context) {
// // 	userData, err := services.UnblockUser(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }

// // // Add a user to your favorites.
// // func AddToFavorites(c *gin.Context) {
// // 	userData, err := services.AddToFavorites(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }

// // // Remove a user from your favorites.
// // func RemoveFromFavorites(c *gin.Context) {
// // 	userData, err := services.RemoveFromFavorites(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }

// // func GetFavoriteContacts(c *gin.Context) {
// // 	userData, err := services.GetFavoriteContacts(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }
