package router

import (
	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/controller"
	"github.com/shoonya0/ECHO/internal/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(r *gin.Engine) {
	r.POST(constants.ApiBasePath+"login", controller.Login)
	r.POST(constants.ApiBasePath+"signup", controller.Signup)

	// Register WebSocket routes BEFORE auth middleware (they handle auth internally)
	RegisterWebSocketRoutes(r)

	r.Use(middleware.AuthMiddleware)
	r.Use(middleware.LoggerMiddleware())

	RegisterUserRoutes(r)
	RegisterChatRoutes(r)
}

// RegisterWebSocketRoutes registers WebSocket endpoints that handle authentication internally
func RegisterWebSocketRoutes(r *gin.Engine) {
	wsGroup := r.Group(constants.ApiBasePath + "ws")
	wsGroup.Use(middleware.AuthMiddleware) // Apply auth middleware to WebSocket routes
	wsGroup.Use(middleware.LoggerMiddleware())

	wsGroup.GET("/chat", controller.HandleWebSocketChat)
}
