package routes

import (
	"gin/internal/controller"
	"gin/objects"

	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes registers all user-related routes
func RegisterUserRoutes(r *gin.Engine) {
	// Apply authentication middleware to all user routes
	userApi := r.Group(objects.ApiBasePath)

	// ============ PROFILE MANAGEMENT ============
	profileRoutes := userApi.Group("profile")
	{
		profileRoutes.GET("/", controller.GetProfile)             // Get current user profile
		profileRoutes.PUT("/", controller.UpdateProfile)          // Update current user profile
		profileRoutes.DELETE("/delete", controller.DeleteProfile) // delete profile
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
		contactRoutes.POST("/blockUnblock/:userId", controller.BlockUnblockUser)   // Block/Unblock user
		contactRoutes.POST("/favorite/:userId", controller.AddToFavorites)         // Add to favorites
		contactRoutes.DELETE("/favorite/:userId", controller.RemoveFromFavorites)  // Remove from favorites
	}
}
