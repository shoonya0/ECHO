package middleware

import (
	"github.com/shoonya0/ECHO/internal/logger"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORSMiddleware() gin.HandlerFunc {
	corsHandler := cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders: []string{"Content-Length", "Content-Type"},
		// Auth uses Bearer tokens in the Authorization header, not cookies, so
		// credentials mode is off. This keeps the wildcard origin CORS-valid
		// (Access-Control-Allow-Origin: * with credentials is rejected by browsers).
		AllowCredentials: false,
		MaxAge:           86400,
	})

	return func(c *gin.Context) {
		// Get logger from context
		log := logger.WithContext(c.Request.Context())

		// Log CORS request details (exclude Authorization header for security)
		safeHeaders := make(map[string][]string)
		for k, v := range c.Request.Header {
			if k != "Authorization" {
				safeHeaders[k] = v
			}
		}
		log.WithFields(map[string]interface{}{
			"origin":  c.GetHeader("Origin"),
			"method":  c.Request.Method,
			"path":    c.Request.URL.Path,
			"headers": safeHeaders,
		}).Debug("Processing CORS request")

		corsHandler(c)
	}
}
