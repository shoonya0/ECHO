package services

import (
	"fmt"
	"gin/objects"

	"github.com/gin-gonic/gin"
)

func GetPresence(c *gin.Context) (string, error) {
	userID, ok := c.Get("user_id")
	if !ok {
		return "", fmt.Errorf("user id not found")
	}
	// we will get the presence of user from fast storage
	presence, err := objects.RedisClient.Get(c.Request.Context(), userID.(string)).Result()
	if err != nil {
		return "", fmt.Errorf("failed to get presence: %w", err)
	}
	return presence, nil
}

func UpdatePresence(c *gin.Context) (string, error) {
	userID, ok := c.Get("user_id")
	if !ok {
		return "", fmt.Errorf("user id not found")
	}
}
