package routes

import (
	"gin/internal/controller"
	"gin/objects"

	"github.com/gin-gonic/gin"
)

// RegisterChatRoutes registers all chat-related routes under /echo/v1/
func RegisterChatRoutes(r *gin.Engine) {
	chatApi := r.Group(objects.ApiBasePath)

	// ============ CHAT MANAGEMENT ============
	chatRoutes := chatApi.Group("chats")
	{
		chatRoutes.POST("/direct/:userId", controller.CreateDirectChatHTTP) // Create direct chat
		chatRoutes.POST("/group", controller.CreateGroupChatHTTP)           // Create group chat
		chatRoutes.GET("/hub/stats", controller.GetHubStatsHTTP)            // WebSocket hub stats (monitoring)
	}

	// ============ MESSAGING ============
	messageRoutes := chatApi.Group("messages")
	{
		messageRoutes.GET("/:chatID/messages", controller.GetChatMessagesHTTP) // Chat messages with pagination
	}

	// ============ GROUP MANAGEMENT ============
	groupRoutes := chatApi.Group("groups")
	{
		// Member management
		groupRoutes.POST("/:groupID/add-members", controller.AddGroupMembersHTTP) // Add members (admin)

		// Static invite routes MUST be registered before /:groupID to prevent
		// Gin from matching literal path segments as the :groupID parameter.
		groupRoutes.GET("/invites", controller.GetAllInvitesOfUser)                      // User's invites
		groupRoutes.POST("/join/:inviteCode", controller.JoinGroupByInvite)              // Join group via invite
		groupRoutes.DELETE("/invites/:inviteID", controller.DeleteInvite)                // Delete invite
		groupRoutes.GET("/invites/:inviteID/joined", controller.GetJoinedUsersByInvite)  // Joined users by invite
		groupRoutes.POST("/invites/:inviteID/:userID/send", controller.SendInviteToUser) // Send invite to user

		groupRoutes.POST("/:groupID/invites", controller.CreateInvite)   // Create invite for group
		groupRoutes.GET("/:groupID/invites", controller.GetGroupInvites) // Group's invites
	}
}
