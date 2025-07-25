package routes

import (
	"github.com/gin-gonic/gin"
)

// Register all routes here
func RegisterAPIRoutes(r *gin.Engine) {
	// Register authentication routes
	RegisterAuthRoutes(r)

	// // Register chat routes
	// RegisterChatRoutes(r)

	// // Register WebSocket routes
	// RegisterWebSocketRoutes(r)

	// // Register user management routes
	RegisterUserRoutes(r)

	// // Register admin routes
	// RegisterAdminRoutes(r)
}
