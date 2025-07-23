package controller

import (
	"gin/internal/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

func GetPresence(c *gin.Context) {
	presence, err := services.GetPresence(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, presence)
}

func UpdatePresence(c *gin.Context) {
	presence, err := services.UpdatePresence(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, presence)
}
