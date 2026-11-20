package router

import (
	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/controller"
	"github.com/shoonya0/ECHO/internal/middleware"
	"github.com/shoonya0/ECHO/internal/services"

	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes registers every API route. The access logger comes first, so
// login, signup and requests refused by auth or a rate limiter are logged too,
// each once (#172). Gin copies the middleware list into each route and group when
// it is registered, so a r.Use only applies to routes registered after it.
func RegisterAPIRoutes(r *gin.Engine) {
	r.Use(middleware.LoggerMiddleware())

	r.POST(constants.ApiBasePath+"login", middleware.RateLimitByIP("login", services.LoginIPLimit), controller.Login)
	r.POST(constants.ApiBasePath+"signup", middleware.RateLimitByIP("signup", services.SignupIPLimit), controller.Signup)

	RegisterWebSocketRoutes(r)

	r.Use(middleware.AuthMiddleware)

	RegisterUserRoutes(r)
	RegisterChatRoutes(r)
}

// RegisterWebSocketRoutes registers the WebSocket endpoint behind its own
// AuthMiddleware. It is called before the global AuthMiddleware is added, so the
// group doesn't get it twice; the access logger comes from RegisterAPIRoutes.
func RegisterWebSocketRoutes(r *gin.Engine) {
	wsGroup := r.Group(constants.ApiBasePath + "ws")
	wsGroup.Use(middleware.AuthMiddleware)

	wsGroup.GET("/chat", controller.HandleWebSocketChat)
}
