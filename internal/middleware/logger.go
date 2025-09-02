package middleware

import (
	"gin/logger"
	"time"

	"github.com/gin-gonic/gin"
)

// LoggerMiddleware is a Gin middleware that logs HTTP requests using our custom logger
func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()

		// Create request ID and add it to context
		ctx := logger.WithRequestID(c.Request.Context())
		// here it creates a new request with the new context
		c.Request = c.Request.WithContext(ctx)

		// Add user ID to context if available
		if userID, exists := c.Get("userId"); exists {
			ctx = logger.WithUserID(ctx, userID.(string))
			c.Request = c.Request.WithContext(ctx)
		}

		// Create logger with request context
		log := logger.WithContext(ctx)

		// Process request
		c.Next()

		// Stop timer
		duration := time.Since(start)

		// Log request details
		log.WithFields(map[string]interface{}{
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"status":     c.Writer.Status(),
			"duration":   duration.String(),
			"client_ip":  c.ClientIP(),
			"user_agent": c.Request.UserAgent(),
		}).Info("HTTP Request")

		// Log errors if any
		for _, err := range c.Errors {
			log.WithError(err.Err).Error("HTTP Request Error")
		}
	}
}
