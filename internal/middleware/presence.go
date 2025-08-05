package middleware

// import (
// 	"gin/internal/services"
// 	"time"

// 	"github.com/gin-gonic/gin"
// )

// // PresenceMiddleware updates user activity on every authenticated request
// func PresenceMiddleware() gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		// Skip if presence service not initialized
// 		if services.PresenceServiceInstance == nil {
// 			c.Next()
// 			return
// 		}

// 		// Skip if user is not authenticated
// 		userID, exists := c.Get("user_id")
// 		if !exists {
// 			c.Next()
// 			return
// 		}

// 		// Update user activity asynchronously to avoid blocking the request
// 		go func() {
// 			err := services.PresenceServiceInstance.UpdateActivity(userID.(string))
// 			if err != nil {
// 				// Log error but don't fail the request
// 				// Could be enhanced with proper logging
// 			}
// 		}()

// 		c.Next()
// 	}
// }

// // PresenceCleanupMiddleware periodically cleans up inactive users
// func PresenceCleanupMiddleware(inactiveThreshold time.Duration, cleanupInterval time.Duration) gin.HandlerFunc {
// 	// Start background cleanup goroutine
// 	go func() {
// 		ticker := time.NewTicker(cleanupInterval)
// 		defer ticker.Stop()

// 		for range ticker.C {
// 			if services.PresenceServiceInstance != nil {
// 				err := services.PresenceServiceInstance.CleanupInactiveUsers(inactiveThreshold)
// 				if err != nil {
// 					// Log error - could be enhanced with proper logging
// 				}
// 			}
// 		}
// 	}()

// 	// Return a middleware that does nothing (cleanup runs in background)
// 	return func(c *gin.Context) {
// 		c.Next()
// 	}
// }

// // UserLoginMiddleware handles user login events
// func UserLoginMiddleware() gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		// Skip if presence service not initialized
// 		if services.PresenceServiceInstance == nil {
// 			c.Next()
// 			return
// 		}

// 		// Check if this is a login request
// 		if c.Request.Method == "POST" && (c.Request.URL.Path == "/auth/login" || c.Request.URL.Path == "/echo/v1/auth/login") {
// 			// Process the request first
// 			c.Next()

// 			// If login was successful (check response status)
// 			if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
// 				userID, exists := c.Get("user_id")
// 				if exists {
// 					// Handle user login asynchronously
// 					go func() {
// 						deviceInfo := c.GetHeader("User-Agent")
// 						err := services.PresenceServiceInstance.UserLogin(userID.(string), &deviceInfo)
// 						if err != nil {
// 							// Log error - could be enhanced with proper logging
// 						}
// 					}()
// 				}
// 			}
// 		} else {
// 			c.Next()
// 		}
// 	}
// }

// // UserLogoutMiddleware handles user logout events
// func UserLogoutMiddleware() gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		// Skip if presence service not initialized
// 		if services.PresenceServiceInstance == nil {
// 			c.Next()
// 			return
// 		}

// 		// Check if this is a logout request
// 		if c.Request.Method == "POST" && (c.Request.URL.Path == "/auth/logout" || c.Request.URL.Path == "/echo/v1/auth/logout") {
// 			userID, exists := c.Get("user_id")
// 			if exists {
// 				// Handle user logout before processing the request
// 				go func() {
// 					err := services.PresenceServiceInstance.UserLogout(userID.(string))
// 					if err != nil {
// 						// Log error - could be enhanced with proper logging
// 					}
// 				}()
// 			}
// 		}

// 		c.Next()
// 	}
// }
