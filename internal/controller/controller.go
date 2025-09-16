package controller

import (
	"context"
	"encoding/json"
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
