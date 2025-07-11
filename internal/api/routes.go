package routes

import (
	"gin/internal/auth"
	"gin/objects"

	"github.com/gin-gonic/gin"
)

// Register all routes here
func RegisterAPIRoutes(r *gin.Engine) {
	// API base path
	authApi := r.Group(objects.AuthBasePath)
	{
		authApi.POST("login", auth.Login)
		// authApi.POST("register", auth.Register)
		// authApi.GET("user/:id", GetUserDetails)
	}

	// admin routes
	// adminApi := r.Group(objects.ApiBasePath + "admin/")
	// {
	// 	adminApi.GET("users", GetUsers)
	// 	adminApi.GET("users/:id", GetUser)
	// 	adminApi.POST("users", CreateUser)
	// 	adminApi.PUT("users/:id", UpdateUser)
	// 	adminApi.DELETE("users/:id", DeleteUser)
	// }

	// WebSocket routes
	// websocketApi := r.Group(objects.WebSocketBasePath)
	// {
	// websocketApi.GET("message", MessageHandler)
	// websocketApi.GET("group", GroupHandler)
	// websocketApi.GET("media", MediaHandler)
	// websocketApi.GET("notification", NotificationHandler)
	// }
}
