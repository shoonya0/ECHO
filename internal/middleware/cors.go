package middleware

import (
	"gin/logger"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORSMiddleware() gin.HandlerFunc {
	corsHandler := cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           86400,
	})

	return func(c *gin.Context) {
		// Get logger from context
		log := logger.WithContext(c.Request.Context())

		// Log CORS request details
		log.WithFields(map[string]interface{}{
			"origin":  c.GetHeader("Origin"),
			"method":  c.Request.Method,
			"path":    c.Request.URL.Path,
			"headers": c.Request.Header,
		}).Debug("Processing CORS request")

		corsHandler(c)
	}
}
