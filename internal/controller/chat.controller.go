package controller

import (
	"gin/internal/models"
	"gin/internal/services"
	"gin/internal/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetGroupMessages(ctx *gin.Context) {
	chatInfo := models.ContactInfo{}
	if err := ctx.ShouldBindJSON(&chatInfo); err != nil {
		utils.ErrorResponse(ctx, http.StatusBadRequest, "invalid chat info", nil)
		return
	}

	chat, err := services.GetChat(chatInfo)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusInternalServerError, "failed to get chat", nil)
		return
	}
	utils.SuccessResponse(ctx, "Chat fetched successfully", chat)
}
