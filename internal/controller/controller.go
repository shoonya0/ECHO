package controller

import (
	"context"
	"encoding/json"
	"errors"
	"gin/internal/services"
	"gin/internal/utils"
	"gin/logger"
	"gin/objects"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// function to reduce gincontext to context.Context and logger to logrus.Entry
func ReduceGinContextToContext(ctx *gin.Context) (context.Context, logrus.Entry, bool) {
	userInterface, exists := ctx.Get("user")
	if !exists {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "User not authenticated", nil)
		return nil, logrus.Entry{}, false
	}

	reqCtx := context.WithValue(ctx.Request.Context(), objects.UserDataKey, userInterface)
	log := logger.WithContext(reqCtx)

	return reqCtx, *log, true
}

// BuildPartialDocument creates a MongoDB update document only with non-nil fields
// This prevents updating fields that weren't provided in the request
func BuildPartialDocument(updateReq interface{}) bson.M {
	updateDoc := bson.M{}

	jsonData, err := json.Marshal(updateReq)
	if err != nil {
		return updateDoc
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(jsonData, &dataMap); err != nil {
		return updateDoc
	}

	buildNestedUpdate("", dataMap, updateDoc)

	return updateDoc
}

// MapServiceErrorToHTTP converts a service-layer error to an HTTP status code
// and a static human-readable message (never the raw error).
func MapServiceErrorToHTTP(err error) (int, string) {
	switch {
	case errors.Is(err, services.ErrUserNotFound):
		return http.StatusNotFound, "User not found"
	case errors.Is(err, services.ErrChatNotFound):
		return http.StatusNotFound, "Chat not found"
	case errors.Is(err, services.ErrContactNotFound):
		return http.StatusNotFound, "Contact not found"
	case errors.Is(err, services.ErrContactRequestNotFound):
		return http.StatusNotFound, "Contact request not found"
	case errors.Is(err, services.ErrInvalidInvite):
		return http.StatusBadRequest, "Invalid invite"
	case errors.Is(err, services.ErrNotInContacts):
		return http.StatusBadRequest, "User is not in your contacts"
	case errors.Is(err, services.ErrNotInFavorites):
		return http.StatusBadRequest, "User is not in your favorites"
	case errors.Is(err, services.ErrAlreadyInContacts):
		return http.StatusConflict, "User is already in your contacts"
	case errors.Is(err, services.ErrAlreadyInFavorites):
		return http.StatusConflict, "User is already in your favorites"
	case errors.Is(err, services.ErrBlocked):
		return http.StatusForbidden, "User is blocked"
	case errors.Is(err, services.ErrAlreadyBlocked):
		return http.StatusConflict, "User is already blocked"
	case errors.Is(err, services.ErrAlreadyUnblocked):
		return http.StatusConflict, "User is already unblocked"
	case errors.Is(err, services.ErrSelfBlock):
		return http.StatusBadRequest, "Cannot block yourself"
	case errors.Is(err, services.ErrRequestAlreadySent):
		return http.StatusConflict, "Contact request already sent"
	case errors.Is(err, services.ErrNoValidParticipants):
		return http.StatusBadRequest, "No valid participants to remove"
	case errors.Is(err, services.ErrNotParticipant):
		return http.StatusForbidden, "You are not a participant of this chat"
	case errors.Is(err, services.ErrPermissionDenied):
		return http.StatusForbidden, "Permission denied"
	default:
		return http.StatusInternalServerError, "Internal server error"
	}
}

func buildNestedUpdate(prefix string, data map[string]interface{}, updateDoc bson.M) {
	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch metaData := value.(type) {
		case map[string]interface{}:
			buildNestedUpdate(fullKey, metaData, updateDoc)
		case nil:
			continue
		default:
			updateDoc[fullKey] = value
		}
	}
}
