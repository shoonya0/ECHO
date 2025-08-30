package routes

import (
	"gin/internal/controller"
	"gin/internal/middleware"
	"gin/objects"

	"github.com/gin-gonic/gin"
)

// remaining :- when sending an message to an unknown user then we have to autojoin both the users insted of one.
func RegisterChatRoutes(r *gin.Engine) {
	// ============ WEBSOCKET ROUTES (NO AUTH MIDDLEWARE) ============
	wsApi := r.Group(objects.ApiBasePath + "ws")
	{
		wsApi.GET("/chat", controller.HandleWebSocketChat) // Main WebSocket endpoint for real-time chat
	}

	chatApi := r.Group(objects.ApiBasePath)
	chatApi.Use(middleware.AuthMiddleware)

	// ============ CHAT MANAGEMENT ROUTES ============
	chatRoutes := chatApi.Group("chats")
	{
		chatRoutes.POST("/direct", controller.CreateDirectChatHTTP) // Create direct chat
		chatRoutes.POST("/group", controller.CreateGroupChatHTTP)   // Create group chat
		// add target users to group after group creation (this rout take array of user ids)
		// chatRoutes.POST("/group/add-members", controller.AddGroupMembersHTTP)
		chatRoutes.GET("/messages", controller.GetChatMessagesHTTP) // Get chat messages with pagination

		// Monitoring (for admin/debugging)
		chatRoutes.GET("/hub/stats", controller.GetHubStatsHTTP) // Get WebSocket hub statistics
	}

	// chatUrl for group chat := to add people in group chat via invite code
	// chatRoutes.POST("/group/add-members/:InviteCode", controller.AddGroupMembersHTTP)

	// ============ MESSAGING ROUTES ============
	messageRoutes := chatApi.Group("messages")
	{
		// Group Messages (legacy endpoint for backward compatibility)
		messageRoutes.GET("/groups/chat", controller.GetGroupMessages) // Get group message history

		// 		// Message Operations (Future implementations)
		// 		messageRoutes.PUT("/:messageID", controller.EditMessage)                        // Edit message
		// 		messageRoutes.DELETE("/:messageID", controller.DeleteMessage)                   // Delete message
		// 		messageRoutes.POST("/:messageID/reactions", controller.AddReaction)             // Add reaction
		// 		messageRoutes.DELETE("/:messageID/reactions/:emoji", controller.RemoveReaction) // Remove reaction
		// 		messageRoutes.POST("/:messageID/reply", controller.ReplyToMessage)              // Reply to message
		// 		messageRoutes.POST("/:messageID/forward", controller.ForwardMessage)            // Forward message
		// 		messageRoutes.PUT("/:messageID/pin", controller.PinMessage)                     // Pin message
		// 		messageRoutes.DELETE("/:messageID/pin", controller.UnpinMessage)                // Unpin message
	}

	// 	// ============ GROUP MANAGEMENT ROUTES ============
	groupRoutes := chatApi.Group("groups")
	{
		// 		// Group CRUD
		// 		groupRoutes.POST("/", controller.CreateGroup)           // Create group
		// 		groupRoutes.GET("/:groupID", controller.GetGroup)       // Get group details
		// 		groupRoutes.PUT("/:groupID", controller.UpdateGroup)    // Update group
		// 		groupRoutes.DELETE("/:groupID", controller.DeleteGroup) // Delete group
		// 		groupRoutes.GET("/", controller.GetUserGroups)          // Get user's groups

		// 		// Member Management
		// 		groupRoutes.POST("/:groupID/members", controller.AddGroupMember)               // Add member
		// 		groupRoutes.DELETE("/:groupID/members/:userID", controller.RemoveGroupMember)  // Remove member
		// 		groupRoutes.PUT("/:groupID/members/:userID/role", controller.UpdateMemberRole) // Update role
		// 		groupRoutes.GET("/:groupID/members", controller.GetGroupMembers)               // Get members
		// 		groupRoutes.POST("/:groupID/leave", controller.LeaveGroup)                     // Leave group

		// 		// Group Settings
		// 		groupRoutes.PUT("/:groupID/settings", controller.UpdateGroupSettings) // Update settings
		// 		groupRoutes.GET("/:groupID/settings", controller.GetGroupSettings)    // Get settings

		// ============ INVITE CODE ROUTES ============

		// make an invite code for the group chat
		groupRoutes.POST("/:groupID/invites", controller.CreateInvite) // Create invite

		// list all invite code of any specific chatID
		groupRoutes.GET("/:groupID/invites", controller.GetGroupInvites) // Get group invites

		// delete an invite code
		groupRoutes.DELETE("/invites/:inviteID/", controller.DeleteInvite) // Delete invite

		// update the status of an invite code -> done internally
		// groupRoutes.PUT("/invites/:inviteID/status", controller.UpdateInviteStatus) // Update invite status

		// get the joined users by an invite code
		groupRoutes.GET("/invites/:inviteID/joined", controller.GetJoinedUsersByInvite) // Get joined users by invite

		// in user we make map of chatID and invite code
		// send an invite code to an user
		groupRoutes.POST("/invites/:inviteID/:userID/send", controller.SendInviteToUser) // Send invite to user

		// join a group via an invite code
		groupRoutes.POST("/join/:inviteCode", controller.JoinGroupByInvite) // Join via invite

		// get all invite chat of an user
		// groupRoutes.GET("/invites", controller.GetAllInvites) // Get all invites of an user
	}

	// 	// ============ CHANNEL ROUTES (for Discord-like functionality) ============
	// 	// channelRoutes := chatApi.Group("channels")
	// 	// {
	// 	// 	channelRoutes.POST("/", controller.CreateChannel)                 // Create channel
	// 	// 	channelRoutes.GET("/:channelID", controller.GetChannel)           // Get channel
	// 	// 	channelRoutes.PUT("/:channelID", controller.UpdateChannel)        // Update channel
	// 	// 	channelRoutes.DELETE("/:channelID", controller.DeleteChannel)     // Delete channel
	// 	// 	channelRoutes.GET("/group/:groupID", controller.GetGroupChannels) // Get group channels
	// 	// }

	// 	// ============ MEDIA/FILE ROUTES ============
	// 	mediaRoutes := chatApi.Group("media")
	// 	{
	// 		mediaRoutes.POST("/upload", controller.UploadFile)          // Upload file/media
	// 		mediaRoutes.GET("/:fileID", controller.GetFile)             // Get file
	// 		mediaRoutes.DELETE("/:fileID", controller.DeleteFile)       // Delete file
	// 		mediaRoutes.POST("/upload/avatar", controller.UploadAvatar) // Upload avatar
	// 	}

	// 	// ============ PRESENCE/STATUS ROUTES ============
	// 	presenceRoutes := chatApi.Group("presence")
	// 	{
	// 		presenceRoutes.PUT("/status", controller.UpdateStatus)                 // Update online status
	// 		presenceRoutes.GET("/status/:userID", controller.GetUserStatus)        // Get user status
	// 		presenceRoutes.POST("/typing/:chatID", controller.SendTypingIndicator) // Typing indicator
	// 	}

	// 	// ============ NOTIFICATION ROUTES ============
	// 	notificationRoutes := chatApi.Group("notifications")
	// 	{
	// 		notificationRoutes.GET("/", controller.GetNotifications)                     // Get notifications
	// 		notificationRoutes.PUT("/:notificationID/read", controller.MarkAsRead)       // Mark as read
	// 		notificationRoutes.PUT("/read-all", controller.MarkAllAsRead)                // Mark all as read
	// 		notificationRoutes.DELETE("/:notificationID", controller.DeleteNotification) // Delete notification
	// 	}

	// 	// ============ SEARCH ROUTES ============
	// 	searchRoutes := chatApi.Group("search")
	// 	{
	// 		searchRoutes.GET("/messages", controller.SearchMessages) // Global message search
	// 		searchRoutes.GET("/users", controller.SearchUsers)       // Search users (already exists)
	// 		searchRoutes.GET("/groups", controller.SearchGroups)     // Search groups
	// 	}

	// 	// ============ MODERATION ROUTES ============
	// 	moderationRoutes := chatApi.Group("moderation")
	// 	{
	// 		moderationRoutes.POST("/reports", controller.CreateReport)          // Report user/message
	// 		moderationRoutes.GET("/reports", controller.GetReports)             // Get reports (admin)
	// 		moderationRoutes.PUT("/reports/:reportID", controller.HandleReport) // Handle report (admin)
	// 		moderationRoutes.POST("/ban", controller.BanUser)                   // Ban user (admin)
	// 		moderationRoutes.DELETE("/ban/:userID", controller.UnbanUser)       // Unban user (admin)
	// 		moderationRoutes.POST("/mute", controller.MuteUser)                 // Mute user
	// 		moderationRoutes.DELETE("/mute/:userID", controller.UnmuteUser)     // Unmute user
	// 	}

	// 	// ============ ADMIN ROUTES ============
	// 	adminRoutes := chatApi.Group("admin")
	// 	{
	// 		adminRoutes.GET("/users", controller.GetAllUsers)                     // Get all users
	// 		adminRoutes.GET("/groups", controller.GetAllGroups)                   // Get all groups
	// 		adminRoutes.GET("/analytics", controller.GetAnalytics)                // Get analytics
	// 		adminRoutes.PUT("/users/:userID/status", controller.UpdateUserStatus) // Update user status
	// 	}
}

// // ============ WEBSOCKET ROUTES ============
// func RegisterWebSocketRoutes(r *gin.Engine) {
// 	wsApi := r.Group(objects.WebSocketBasePath)
// 	{
// 		// Real-time messaging
// 		wsApi.GET("/chat", controller.HandleWebSocketChat)                   // Main chat WebSocket
// 		wsApi.GET("/notifications", controller.HandleWebSocketNotifications) // Notifications WebSocket
// 		wsApi.GET("/presence", controller.HandleWebSocketPresence)           // Presence WebSocket

// 		// Voice/Video calling (future implementation)
// 		// 	wsApi.GET("/voice/:channelID", controller.HandleVoiceChannel) // Voice channel
// 		// 	wsApi.GET("/video/:channelID", controller.HandleVideoChannel) // Video channel
// }
// }
