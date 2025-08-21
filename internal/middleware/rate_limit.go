package middleware

import (
	"gin/logger"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type RateLimiter struct {
	sync.Mutex
	requests map[string][]time.Time
	window   time.Duration
	limit    int
}

func NewRateLimiter(window time.Duration, limit int) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		window:   window,
		limit:    limit,
	}
}

func (rl *RateLimiter) cleanup(key string) {
	rl.Lock()
	defer rl.Unlock()

	if reqs, exists := rl.requests[key]; exists {
		now := time.Now()
		validReqs := make([]time.Time, 0)
		for _, req := range reqs {
			if now.Sub(req) <= rl.window {
				validReqs = append(validReqs, req)
			}
		}
		if len(validReqs) > 0 {
			rl.requests[key] = validReqs
		} else {
			delete(rl.requests, key)
		}
	}
}

func (rl *RateLimiter) isAllowed(key string) bool {
	rl.Lock()
	defer rl.Unlock()

	now := time.Now()
	reqs := rl.requests[key]

	// Remove expired requests
	validReqs := make([]time.Time, 0)
	for _, req := range reqs {
		if now.Sub(req) <= rl.window {
			validReqs = append(validReqs, req)
		}
	}

	if len(validReqs) >= rl.limit {
		rl.requests[key] = validReqs
		return false
	}

	rl.requests[key] = append(validReqs, now)
	return true
}

func RateLimitMiddleware(window time.Duration, limit int) gin.HandlerFunc {
	limiter := NewRateLimiter(window, limit)

	return func(c *gin.Context) {
		// Get logger from context
		log := logger.WithContext(c.Request.Context())

		// Use IP address as key for rate limiting
		clientIP := c.ClientIP()

		// Cleanup old requests periodically
		go limiter.cleanup(clientIP)

		// Check if request is allowed
		if !limiter.isAllowed(clientIP) {
			log.WithFields(map[string]interface{}{
				"client_ip": clientIP,
				"limit":     limit,
				"window":    window.String(),
			}).Warn("Rate limit exceeded")

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
				"code":  "RATE_LIMIT_EXCEEDED",
			})
			c.Abort()
			return
		}

		log.WithFields(map[string]interface{}{
			"client_ip": clientIP,
			"path":      c.Request.URL.Path,
			"method":    c.Request.Method,
		}).Debug("Request allowed by rate limiter")

		c.Next()
	}
}
