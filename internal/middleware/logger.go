package middleware

import (
	"time"

	"github.com/shoonya0/ECHO/internal/logger"

	"github.com/gin-gonic/gin"
)

// LoggerMiddleware writes one "HTTP Request" line per request, with its final
// status. Register it once, before AuthMiddleware and the rate limiters, so a
// request they refuse is logged too (#172). It logs the method, path, status,
// duration, client IP and user agent, never the body or headers. The user_id
// comes from the request context after the handlers ran, since AuthMiddleware
// adds it there; a WebSocket's line is written when the socket closes.
func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Later middleware and handlers derive their context from this one, so
		// their logs carry the same request_id.
		c.Request = c.Request.WithContext(logger.WithRequestID(c.Request.Context()))

		c.Next()

		log := logger.WithContext(c.Request.Context())
		log.WithFields(map[string]interface{}{
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"status":     c.Writer.Status(),
			"duration":   time.Since(start).String(),
			"client_ip":  c.ClientIP(),
			"user_agent": c.Request.UserAgent(),
		}).Info("HTTP Request")

		for _, err := range c.Errors {
			log.WithError(err.Err).Error("HTTP Request Error")
		}
	}
}
