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

// ============ SHARED HELPERS ============

// authUserID extracts the authenticated user's ObjectID from the Gin context.
// Returns (ObjectID, false) if the user is not authenticated or the ID is malformed.
func authUserID(ctx *gin.Context) (bson.ObjectID, bool) {
	raw, ok := ctx.Get("userId")
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "user id not found", nil)
		return bson.ObjectID{}, false
	}
	id, err := bson.ObjectIDFromHex(raw.(string))
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid user id format", err.Error())
		return bson.ObjectID{}, false
	}
	return id, true
}

// parseLimit reads the "limit" query parameter, defaulting to 10 if absent or unparseable.
func parseLimit(ctx *gin.Context) int {
	limit, err := strconv.Atoi(ctx.Query("limit"))
	if err != nil {
		return 10
	}
	return limit
}

// paramObjectID returns the ObjectID for a named path parameter.
func paramObjectID(ctx *gin.Context, param string) (bson.ObjectID, bool) {
	raw := ctx.Param(param)
	id, err := bson.ObjectIDFromHex(raw)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid "+param+" format", err.Error())
		return bson.ObjectID{}, false
	}
	return id, true
}

// contactsList fetches and paginates contacts of the given status for the current user.
func contactsList(ctx *gin.Context, status objects.ContactStatus, msgPrefix string) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		return
	}

	userID, ok := authUserID(ctx)
	if !ok {
		return
	}

	limit := parseLimit(ctx)

	results, err := services.GetContacts(reqCtx, userID, status, limit)
	if err != nil {
		log.Debug("failed to get " + msgPrefix)
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get "+msgPrefix, err.Error())
		return
	}

	utils.PaginatedResponse(ctx, msgPrefix+" fetched successfully", results, limit, len(results))
}

// ============ CONTACT LIST QUERIES ============

// GET /echo/v1/users/contacts/
func GetUsersContacts(ctx *gin.Context) {
	contactsList(ctx, objects.StatusAccepted, "users contacts")
}

// GET /echo/v1/users/contacts/requests
func GetContactRequests(ctx *gin.Context) {
	contactsList(ctx, objects.StatusPending, "contact requests")
}

// GET /echo/v1/users/contacts/sent-requests
func GetSentContactRequests(ctx *gin.Context) {
	contactsList(ctx, objects.StatusPending, "sent contact requests")
}

// GET /echo/v1/users/contacts/blocked
func GetBlockedUsers(ctx *gin.Context) {
	contactsList(ctx, objects.StatusBlocked, "blocked users")
}

// GET /echo/v1/users/contacts/favorites
func GetFavoriteContacts(ctx *gin.Context) {
	contactsList(ctx, objects.StatusFavorite, "favorite contacts")
}

// ============ CONTACT ACTIONS ============

// POST /echo/v1/users/contacts/:targetUserId
func SendContactRequest(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		return
	}

	userID, ok := authUserID(ctx)
	if !ok {
		return
	}

	targetID, ok := paramObjectID(ctx, "targetUserId")
	if !ok {
		return
	}

	if err := services.SendContactRequest(reqCtx, userID, targetID); err != nil {
		log.Debug("failed to send contact request")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to send contact request", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "contact request sent successfully", nil)
}

// PUT /echo/v1/users/contacts/:requestId?action=accepted|declined
// Accepts or declines a pending contact request.
func AcceptOrDeclineContactRequest(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		return
	}

	userID, ok := authUserID(ctx)
	if !ok {
		return
	}

	action := ctx.Query("action")
	if action != string(objects.StatusAccepted) && action != string(objects.StatusDeclined) {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "action must be 'accepted' or 'declined'", nil)
		return
	}

	requestID, ok := paramObjectID(ctx, "requestId")
	if !ok {
		return
	}

	if err := services.AcceptOrDeclineContactRequest(reqCtx, userID, requestID, action); err != nil {
		log.Debug("failed to " + action + " contact request")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to "+action+" contact request", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "contact request "+action+" successfully", nil)
}

// DELETE /echo/v1/users/contacts/:contactId
func RemoveContact(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		return
	}

	userID, ok := authUserID(ctx)
	if !ok {
		return
	}

	targetID, ok := paramObjectID(ctx, "contactId")
	if !ok {
		return
	}

	if err := services.RemoveContact(reqCtx, userID, targetID); err != nil {
		log.Debug("failed to remove contact")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to remove contact", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "contact removed successfully", nil)
}

// POST /echo/v1/users/contacts/blockUnblock/:userId?action=block|unblock
// Blocks or unblocks a user.
func BlockUnblockUser(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		return
	}

	userID, ok := authUserID(ctx)
	if !ok {
		return
	}

	targetID, ok := paramObjectID(ctx, "userId")
	if !ok {
		return
	}

	action := ctx.Query("action")
	if action == "" {
		action = string(objects.StatusBlocked) // default to block for backward compatibility
	}

	if err := services.BlockUnblockUser(reqCtx, userID, targetID, action); err != nil {
		log.Debug("failed to " + action + " user")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to "+action+" user", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "user "+action+" successfully", nil)
}

// POST /echo/v1/users/contacts/favorite/:userId
func AddToFavorites(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		return
	}

	userID, ok := authUserID(ctx)
	if !ok {
		return
	}

	targetID, ok := paramObjectID(ctx, "userId")
	if !ok {
		return
	}

	if err := services.AddToFavorites(reqCtx, userID, targetID); err != nil {
		log.Debug("failed to add user to favorites")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to add user to favorites", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "user added to favorites successfully", nil)
}

// DELETE /echo/v1/users/contacts/favorite/:userId
func RemoveFromFavorites(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		return
	}

	userID, ok := authUserID(ctx)
	if !ok {
		return
	}

	targetID, ok := paramObjectID(ctx, "userId")
	if !ok {
		return
	}

	if err := services.RemoveFromFavorites(reqCtx, userID, targetID); err != nil {
		log.Debug("failed to remove user from favorites")
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to remove user from favorites", err.Error())
		return
	}

	utils.SuccessResponse(ctx, "user removed from favorites successfully", nil)
}
