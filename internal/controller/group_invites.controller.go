package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/utils"
)

// GetPendingGroupInvitesHTTP lists the groups the caller is invited to and hasn't
// answered (#5).
// GET /echo/v1/groups/pending
func GetPendingGroupInvitesHTTP(ctx *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		return
	}
	user, ok := reqCtx.Value(constants.UserDataKey).(models.LoginUserResponse)
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	invites, err := services.PendingGroupInvites(reqCtx, user.ID)
	if err != nil {
		log.WithError(err).Debug("failed to list pending group invites")
		status, msg := MapServiceErrorToHTTP(err)
		utils.ErrorResponse(ctx, status, msg, nil)
		return
	}
	utils.SuccessResponse(ctx, "Pending group invites fetched successfully", invites)
}

// AcceptGroupInviteHTTP accepts the caller's pending invite to a group (#5).
// POST /echo/v1/groups/:groupID/accept
func AcceptGroupInviteHTTP(ctx *gin.Context) {
	answerGroupInvite(ctx, services.AcceptGroupInvite, "Group invite accepted")
}

// DeclineGroupInviteHTTP declines the caller's pending invite to a group (#5).
// POST /echo/v1/groups/:groupID/decline
func DeclineGroupInviteHTTP(ctx *gin.Context) {
	answerGroupInvite(ctx, services.DeclineGroupInvite, "Group invite declined")
}

// answerGroupInvite runs answer for the caller and the :groupID parameter. No
// pending invite gives 404 whether or not the group exists (#59); an accepted
// member gets 409.
func answerGroupInvite(ctx *gin.Context, answer func(context.Context, bson.ObjectID, bson.ObjectID) error, done string) {
	reqCtx, log, ok := ReduceGinContextToContext(ctx)
	if !ok {
		return
	}
	user, ok := reqCtx.Value(constants.UserDataKey).(models.LoginUserResponse)
	if !ok {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}
	groupID, err := bson.ObjectIDFromHex(ctx.Param("groupID"))
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "Invalid group ID", nil)
		return
	}

	if err := answer(reqCtx, user.ID, groupID); err != nil {
		log.WithError(err).Debug("failed to answer group invite")
		status, msg := MapServiceErrorToHTTP(err)
		utils.ErrorResponse(ctx, status, msg, nil)
		return
	}
	utils.SuccessResponse(ctx, done, gin.H{"chatId": groupID.Hex()})
}
