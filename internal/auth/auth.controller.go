package auth

import (
	"github.com/gin-gonic/gin"
)

func Login(c *gin.Context) {
	// Handle login logic here
	c.JSON(200, gin.H{
		"message": "Login successful",
	})
}
func Register(c *gin.Context) {
	// Handle registration logic here
	c.JSON(200, gin.H{
		"message": "Registration successful",
	})
}
