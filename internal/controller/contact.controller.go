package controller

import (
	"gin/internal/services"
	"gin/internal/utils"
	"gin/objects"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ CONTACTS & FRIENDS MANAGEMENT ============
func GetUsersContacts(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		limit = 10
	}

	objectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	UserDataKey, err := services.GetContacts(reqCtx, objectID, objects.StatusAccepted, limit)
	if err != nil {
		log.Debug("failed to get users contacts")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get users contacts", err.Error())
		return
	}
	utils.PaginatedResponse(ctx, "Users contacts fetched successfully", UserDataKey, limit, len(UserDataKey))
}

func GetContactRequests(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		limit = 10
	}

	objectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	UserDataKey, err := services.GetContacts(reqCtx, objectID, objects.StatusPending, limit)
	if err != nil {
		log.Debug("failed to get contact requests")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get contact requests", err.Error())
		return
	}
	utils.PaginatedResponse(ctx, "Contact requests fetched successfully", UserDataKey, limit, len(UserDataKey))
}

func GetSentContactRequests(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		limit = 10
	}

	objectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	UserDataKey, err := services.GetContacts(reqCtx, objectID, objects.StatusPending, limit)
	if err != nil {
		log.Debug("failed to get sent contact requests")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get sent contact requests", err.Error())
		return
	}

	utils.PaginatedResponse(ctx, "Sent contact requests fetched successfully", UserDataKey, limit, len(UserDataKey))
}

func GetBlockedUsers(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		limit = 10
	}

	objectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	UserDataKey, err := services.GetContacts(reqCtx, objectID, objects.StatusBlocked, limit)
	if err != nil {
		log.Debug("failed to get blocked users")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get blocked users", err.Error())
		return
	}

	utils.PaginatedResponse(ctx, "Blocked users fetched successfully", UserDataKey, limit, len(UserDataKey))
}

func GetFavoriteContacts(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		limit = 10
	}

	objectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	UserDataKey, err := services.GetContacts(reqCtx, objectID, objects.StatusFavorite, limit)
	if err != nil {
		log.Debug("failed to get favorite contacts")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get favorite contacts", err.Error())
		return
	}

	utils.PaginatedResponse(ctx, "Favorite contacts fetched successfully", UserDataKey, limit, len(UserDataKey))
}

// ============= Contact Actions =============
func SendContactRequest(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
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

	targetUserID := ctx.Param("targetUserId")

	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		log.Debug("invalid target user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid target user id format", err.Error())
		return
	}

	err = services.SendContactRequest(reqCtx, objectID, targetObjectID)
	if err != nil {
		log.Debug("failed to send contact request")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to send contact request", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "Contact request sent successfully", nil)
}

// Accept or decline a request ({ action: "accept"|"decline" }).
func AcceptOrDeclineContactRequest(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	userObjectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	targetRequestID := ctx.Param("requestId")

	action := ctx.Query("action")

	if action != string(objects.StatusAccepted) && action != string(objects.StatusDeclined) {
		log.Debug("invalid action")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid action please provide your action as (accepted or declined)", nil)
		return
	}

	targetRequestObjectID, err := bson.ObjectIDFromHex(targetRequestID)
	if err != nil {
		log.Debug("invalid target user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid target user id format", err.Error())
		return
	}

	err = services.AcceptOrDeclineContactRequest(reqCtx, userObjectID, targetRequestObjectID, action)
	if err != nil {
		log.Debug("failed to " + action + " contact request")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to "+action+" contact request", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "Contact request "+action+" successfully", nil)
}

func RemoveContact(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	userObjectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	targetUserID := ctx.Param("contactId")
	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		log.Debug("invalid target user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid target user id format", err.Error())
		return
	}

	err = services.RemoveContact(reqCtx, userObjectID, targetObjectID)
	if err != nil {
		log.Debug("failed to remove contact")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to remove contact", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "Contact removed successfully", nil)
}

func BlockUnblockUser(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}
	targetUserID := ctx.Param("userId")

	userObjectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		log.Debug("invalid target user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid target user id format", err.Error())
		return
	}

	err = services.BlockUnblockUser(reqCtx, userObjectID, targetObjectID, "block")
	if err != nil {
		log.Debug("failed to block user")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to block user", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "User blocked successfully", nil)
}

func AddToFavorites(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	userObjectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	targetUserID := ctx.Param("userId")
	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		log.Debug("invalid target user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid target user id format", err.Error())
		return
	}

	err = services.AddToFavorites(reqCtx, userObjectID, targetObjectID)
	if err != nil {
		log.Debug("failed to add user to favorites")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to add user to favorites", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "User added to favorites successfully", nil)
}

func RemoveFromFavorites(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		log.Debug("user id not found")
		return
	}

	userID, ok := ctx.Get("userId")
	if !ok {
		log.Debug("user id not found")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return
	}

	userObjectID, err := bson.ObjectIDFromHex(userID.(string))
	if err != nil {
		log.Debug("invalid user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return
	}

	targetUserID := ctx.Param("userId")
	targetObjectID, err := bson.ObjectIDFromHex(targetUserID)
	if err != nil {
		log.Debug("invalid target user id format")
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid target user id format", err.Error())
		return
	}

	err = services.RemoveFromFavorites(reqCtx, userObjectID, targetObjectID)
	if err != nil {
		log.Debug("failed to remove user from favorites")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to remove user from favorites", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "User removed from favorites successfully", nil)
}
