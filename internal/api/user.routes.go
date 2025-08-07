package routes

import (
	"gin/internal/controller"
	"gin/internal/middleware"
	"gin/objects"

	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes registers all user-related routes
func RegisterUserRoutes(r *gin.Engine) {
	// Apply authentication middleware to all user routes
	userApi := r.Group(objects.ApiBasePath)
	userApi.Use(middleware.AuthMiddleware)

	// ============ PROFILE MANAGEMENT ============
	profileRoutes := userApi.Group("profile")
	{
		profileRoutes.GET("/", controller.GetProfile)    // Get current user profile
		profileRoutes.PUT("/", controller.UpdateProfile) // Update current user profile

		// 	profileRoutes.POST("/avatar", controller.UpdateAvatar)              // Update profile avatar
		// 	profileRoutes.DELETE("/avatar", controller.DeleteAvatar)            // Delete profile avatar
		// 	profileRoutes.PUT("/privacy", controller.UpdatePrivacySettings)     // Update privacy settings
		// 	profileRoutes.GET("/privacy", controller.GetPrivacySettings)        // Get privacy settings
		// 	profileRoutes.PUT("/preferences", controller.UpdateUserPreferences) // Update user preferences like theme, language, etc.
		// 	profileRoutes.GET("/preferences", controller.GetUserPreferences)    // Get user preferences
		// 	profileRoutes.GET("/activity", controller.GetUserActivity)          // Get user activity log
		// 	profileRoutes.GET("/statistics", controller.GetUserStatistics)      // Get user statistics
	}

	// ============ USER DISCOVERY & SEARCH ============
	userRoutes := userApi.Group("users")
	{
		userRoutes.GET("/:id", controller.GetUserProfile)             // Get specific user profile
		userRoutes.GET("/suggestions", controller.GetUserSuggestions) // Get friend suggestions
		userRoutes.GET("/nearby", controller.GetNearbyUsers)          // Get nearby users (if location enabled)
		userRoutes.GET("/popular", controller.GetPopularUsers)        // Get popular users
	}

	// ============ CONTACTS & FRIENDS MANAGEMENT ============
	contactRoutes := userApi.Group("users/contacts")
	{
		// Contact List Management
		contactRoutes.GET("/", controller.GetUsersContacts)                    // Get users contacts for this we retrieve the chatIds if user click on specific chat
		contactRoutes.GET("/requests", controller.GetContactRequests)          // Get pending contact requests
		contactRoutes.GET("/sent-requests", controller.GetSentContactRequests) // Get sent contact requests show all the users who you have sent the contact request
		contactRoutes.GET("/blocked", controller.GetBlockedUsers)              // Get blocked users list
		contactRoutes.GET("/favorites", controller.GetFavoriteContacts)        // Get favorite contacts

		// Contact Actions
		contactRoutes.POST("/:targetUserId", controller.SendContactRequest)        // Send contact request
		contactRoutes.PUT("/:requestId", controller.AcceptOrDeclineContactRequest) // Accept/decline contact request
		contactRoutes.DELETE("/:contactId", controller.RemoveContact)              // Remove contact/friend
		// need to test this
		contactRoutes.POST("/blockUnblock/:userId", controller.BlockUser)  // Block/Unblock user
		contactRoutes.POST("/favorite/:userId", controller.AddToFavorites) // Add to favorites
	}

	// // ============ PRESENCE & STATUS ============
	// presenceRoutes := userApi.Group("users/me/status")
	// {
	// 	presenceRoutes.GET("/", controller.GetPresence)                    // Get current user presence
	// 	presenceRoutes.PUT("/", controller.UpdatePresence)                 // Update presence status
	// 	presenceRoutes.POST("/custom", controller.SetCustomStatus)         // Set custom status message
	// 	presenceRoutes.DELETE("/custom", controller.ClearCustomStatus)     // Clear custom status message
	// 	presenceRoutes.GET("/history", controller.GetStatusHistory)        // Get status history
	// 	presenceRoutes.PUT("/availability", controller.UpdateAvailability) // Update availability (online/away/busy/invisible)
	// 	presenceRoutes.POST("/activity", controller.UpdateUserActivity)    // Update user activity (for middleware)
	// }

	// // ============ PRESENCE MANAGEMENT (Admin/Internal) ============
	// presenceAdminRoutes := userApi.Group("presence")
	// {
	// 	// Admin endpoints
	// 	presenceAdminRoutes.GET("/online", controller.GetOnlineUsers)         // Get all online users
	// 	presenceAdminRoutes.GET("/all", controller.GetAllPresences)           // Get all user presences
	// 	presenceAdminRoutes.GET("/stats", controller.GetPresenceStats)        // Get presence statistics
	// 	presenceAdminRoutes.POST("/cleanup", controller.CleanupInactiveUsers) // Cleanup inactive users

	// 	// Internal auth events (called by auth service)
	// 	presenceAdminRoutes.POST("/login", controller.HandleUserLogin)   // Handle user login event
	// 	presenceAdminRoutes.POST("/logout", controller.HandleUserLogout) // Handle user logout event
	// }

	// // ============ OTHER USERS PRESENCE ============
	// // Add presence endpoint to existing userRoutes group
	// userRoutes.GET("/:id/presence", controller.GetUserPresence) // Get specific user's presence

	// // ============ USER SETTINGS ============
	// settingsRoutes := userApi.Group("settings")
	// {
	// 	// General Settings
	// 	settingsRoutes.GET("/", controller.GetAllSettings) // Get all user settings
	// 	settingsRoutes.PUT("/", controller.UpdateSettings) // Update multiple settings

	// 	// Notification Settings
	// 	settingsRoutes.GET("/notifications", controller.GetNotificationSettings)    // Get notification preferences
	// 	settingsRoutes.PUT("/notifications", controller.UpdateNotificationSettings) // Update notification preferences
	// 	settingsRoutes.POST("/notifications/test", controller.TestNotifications)    // Test notification settings

	// 	// Privacy Settings
	// 	settingsRoutes.GET("/privacy", controller.GetPrivacySettings)    // Get privacy settings
	// 	settingsRoutes.PUT("/privacy", controller.UpdatePrivacySettings) // Update privacy settings

	// 	// Language & Localization
	// 	settingsRoutes.GET("/language", controller.GetLanguageSettings)    // Get language preferences
	// 	settingsRoutes.PUT("/language", controller.UpdateLanguageSettings) // Update language preferences
	// 	settingsRoutes.GET("/timezone", controller.GetTimezoneSettings)    // Get timezone settings
	// 	settingsRoutes.PUT("/timezone", controller.UpdateTimezoneSettings) // Update timezone settings

	// 	// Theme & Appearance
	// 	settingsRoutes.GET("/theme", controller.GetThemeSettings)        // Get theme preferences
	// 	settingsRoutes.PUT("/theme", controller.UpdateThemeSettings)     // Update theme preferences
	// 	settingsRoutes.GET("/display", controller.GetDisplaySettings)    // Get display settings
	// 	settingsRoutes.PUT("/display", controller.UpdateDisplaySettings) // Update display settings
	// }

	// // ============ BLOCKING & REPORTING ============
	// moderationRoutes := userApi.Group("moderation")
	// {
	// 	moderationRoutes.POST("/report/:userId", controller.ReportUser)   // Report a user
	// 	moderationRoutes.GET("/reports", controller.GetMyReports)         // Get user's submitted reports
	// 	moderationRoutes.POST("/block/:userId", controller.BlockUser)     // Block a user
	// 	moderationRoutes.DELETE("/block/:userId", controller.UnblockUser) // Unblock a user
	// 	moderationRoutes.GET("/blocked", controller.GetBlockedUsers)      // Get blocked users list
	// }

	// // ============ USER DATA & EXPORT ============
	// dataRoutes := userApi.Group("data")
	// {
	// 	dataRoutes.GET("/export", controller.ExportUserData)             // Export user data (GDPR)
	// 	dataRoutes.POST("/import", controller.ImportUserData)            // Import user data
	// 	dataRoutes.GET("/download/:exportId", controller.DownloadExport) // Download data export
	// 	dataRoutes.DELETE("/purge", controller.PurgeUserData)            // Purge user data (GDPR)
	// 	dataRoutes.GET("/usage", controller.GetDataUsage)                // Get data usage statistics
	// }

	// // ============ LOCATION & GEOGRAPHY ============
	// locationRoutes := userApi.Group("location")
	// {
	// 	locationRoutes.POST("/update", controller.UpdateLocation)        // Update user location
	// 	locationRoutes.GET("/", controller.GetLocation)                  // Get current location
	// 	locationRoutes.DELETE("/", controller.ClearLocation)             // Clear location data
	// 	locationRoutes.GET("/nearby", controller.GetNearbyUsers)         // Get nearby users
	// 	locationRoutes.PUT("/privacy", controller.UpdateLocationPrivacy) // Update location privacy settings
	// }

	// // ============ ACHIEVEMENTS & GAMIFICATION ============
	// achievementRoutes := userApi.Group("achievements")
	// {
	// 	achievementRoutes.GET("/", controller.GetUserAchievements)                   // Get user achievements
	// 	achievementRoutes.GET("/available", controller.GetAvailableAchievements)     // Get available achievements
	// 	achievementRoutes.POST("/:achievementId/claim", controller.ClaimAchievement) // Claim achievement reward
	// 	achievementRoutes.GET("/leaderboard", controller.GetLeaderboard)             // Get achievement leaderboard
	// 	achievementRoutes.GET("/progress", controller.GetAchievementProgress)        // Get achievement progress
	// }

	// // ============ USER VERIFICATION ============
	// verificationRoutes := userApi.Group("verification")
	// {
	// 	verificationRoutes.POST("/request", controller.RequestVerification)             // Request user verification
	// 	verificationRoutes.GET("/status", controller.GetVerificationStatus)             // Get verification status
	// 	verificationRoutes.POST("/documents", controller.UploadVerificationDocs)        // Upload verification documents
	// 	verificationRoutes.GET("/requirements", controller.GetVerificationRequirements) // Get verification requirements
	// }
}
