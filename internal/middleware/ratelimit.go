package middleware

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/utils"
)

// ConfigureClientIP sets where c.ClientIP() reads the client's address. With a
// header (CLIENT_IP_HEADER, e.g. True-Client-IP on Render) the address comes from
// that header, which the platform's edge sets. With no header, or when it's
// missing, it is the socket address. X-Forwarded-For is never trusted: on Render
// the proxy appends to what the client sent, so its first entry is forgeable (#40).
// A header value that isn't an IP is dropped, so it can't name a bucket. Call it
// before registering routes: it adds the middleware that does that.
func ConfigureClientIP(r *gin.Engine, header string) error {
	r.TrustedPlatform = header
	if header != "" {
		r.Use(func(c *gin.Context) {
			if v := c.Request.Header.Get(header); v != "" && net.ParseIP(strings.TrimSpace(v)) == nil {
				c.Request.Header.Del(header)
			}
			c.Next()
		})
	}
	return r.SetTrustedProxies(nil)
}

// RateLimitByIP lets each client IP make l.Max requests per l.Window to the
// route, counted under name. A request over that gets 429 with Retry-After and
// error RATE_LIMITED. When Redis is down, requests go through.
func RateLimitByIP(name string, l services.Limit) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		allowed, retryAfter := services.Hit(c.Request.Context(), services.RateLimitKey(name, ip), l)
		if allowed {
			c.Next()
			return
		}
		logger.WithContext(c.Request.Context()).
			WithField("limit", name).
			Info("rate limit reached")
		utils.RateLimitedResponse(c, retryAfter)
		c.Abort()
	}
}
