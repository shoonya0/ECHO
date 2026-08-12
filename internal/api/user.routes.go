package routes

import (
	"gin/internal/controller"
	"gin/objects"

	"github.com/gin-gonic/gin"
)

// RegisterUserRoutes registers all user-related routes under /echo/v1/
func RegisterUserRoutes(r *gin.Engine) {
	userApi := r.Group(objects.ApiBasePath)

	// ============ AUTHENTICATION ============
	authRoutes := userApi.Group("auth")
	{
		authRoutes.POST("/logout", controller.Logout)
	}

	// ============ PROFILE MANAGEMENT ============
	profileRoutes := userApi.Group("profile")
	{
		profileRoutes.GET("/", controller.GetProfile)             // Current user's profile
		profileRoutes.PUT("/", controller.UpdateProfile)          // Update current user's profile
		profileRoutes.DELETE("/delete", controller.DeleteProfile) // Delete current user's profile
	}

	// ============ USER DISCOVERY & SEARCH ============
	discoveryRoutes := userApi.Group("users")
	{
		// Static routes MUST be registered before /:id to prevent Gin from matching
		// literal path segments as the :id parameter.
		discoveryRoutes.GET("/suggestions", controller.GetUserSuggestions) // Friend suggestions (stub)
		discoveryRoutes.GET("/nearby", controller.GetNearbyUsers)          // Nearby users (stub — requires location)
		discoveryRoutes.GET("/popular", controller.GetPopularUsers)        // Popular users (stub)
		discoveryRoutes.GET("/:id", controller.GetUserProfile)             // Specific user by ID
	}

	// ============ CONTACTS & FRIENDS MANAGEMENT ============
	contactRoutes := userApi.Group("users/contacts")
	{
		// Contact list queries
		contactRoutes.GET("/", controller.GetUsersContacts)                    // User's contact list
		contactRoutes.GET("/requests", controller.GetContactRequests)          // Pending incoming requests
		contactRoutes.GET("/sent-requests", controller.GetSentContactRequests) // Sent contact requests
		contactRoutes.GET("/blocked", controller.GetBlockedUsers)              // Blocked users
		contactRoutes.GET("/favorites", controller.GetFavoriteContacts)        // Favorite contacts

		// Contact actions
		contactRoutes.POST("/:targetUserId", controller.SendContactRequest)        // Send contact request
		contactRoutes.PUT("/:requestId", controller.AcceptOrDeclineContactRequest) // Accept/decline (?action=accepted|declined)
		contactRoutes.DELETE("/:contactId", controller.RemoveContact)              // Remove contact
		contactRoutes.POST("/blockUnblock/:userId", controller.BlockUnblockUser)   // Block/unblock user
		contactRoutes.POST("/favorite/:userId", controller.AddToFavorites)         // Add to favorites
		contactRoutes.DELETE("/favorite/:userId", controller.RemoveFromFavorites)  // Remove from favorites
	}
}
