package routes

import (
	"gin/internal/controller"
	"gin/objects"

	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes registers all admin-related routes
func RegisterAdminRoutes(r *gin.Engine) {
	// Apply admin authentication middleware to all admin routes
	adminApi := r.Group(objects.ApiBasePath + "admin/")
	// adminApi.Use(middleware.AdminAuthMiddleware()) // Add when implemented

	// ============ USER MANAGEMENT ============
	userManagement := adminApi.Group("users")
	{
		// User CRUD Operations
		userManagement.GET("/", controller.GetUsers)                // Get all users with pagination
		userManagement.GET("/:id", controller.GetUser)              // Get specific user details
		userManagement.POST("/", controller.CreateUser)             // Create new user (admin)
		userManagement.PUT("/:id", controller.UpdateUser)           // Update user details
		userManagement.DELETE("/:id", controller.DeleteUser)        // Delete user account
		userManagement.POST("/:id/restore", controller.RestoreUser) // Restore deleted user

		// User Status Management
		userManagement.PUT("/:id/status", controller.UpdateUserStatus)  // Update user status (active/inactive/banned)
		userManagement.POST("/:id/ban", controller.BanUser)             // Ban user
		userManagement.POST("/:id/unban", controller.UnbanUser)         // Unban user
		userManagement.POST("/:id/suspend", controller.SuspendUser)     // Suspend user
		userManagement.POST("/:id/unsuspend", controller.UnsuspendUser) // Unsuspend user
		userManagement.PUT("/:id/role", controller.UpdateUserRole)      // Update user role/permissions

		// User Data & Privacy
		userManagement.GET("/:id/activity", controller.GetUserActivityAdmin)    // Get user activity (admin view)
		userManagement.GET("/:id/sessions", controller.GetUserSessions)         // Get user active sessions
		userManagement.POST("/:id/logout-all", controller.ForceLogoutUser)      // Force logout user from all devices
		userManagement.GET("/:id/data", controller.GetUserDataAdmin)            // Get user data (admin view)
		userManagement.POST("/:id/data-export", controller.ExportUserDataAdmin) // Export user data (admin)
	}

	// ============ CONTENT MODERATION ============
	moderationApi := adminApi.Group("moderation")
	{
		// Reports Management
		moderationApi.GET("/reports", controller.GetReports)                 // Get all reports
		moderationApi.GET("/reports/:id", controller.GetReport)              // Get specific report
		moderationApi.PUT("/reports/:id", controller.HandleReport)           // Handle/process report
		moderationApi.POST("/reports/:id/dismiss", controller.DismissReport) // Dismiss report
		moderationApi.GET("/reports/stats", controller.GetReportStats)       // Get report statistics

		// Content Moderation
		moderationApi.GET("/content/flagged", controller.GetFlaggedContent)   // Get flagged content
		moderationApi.POST("/content/:id/approve", controller.ApproveContent) // Approve flagged content
		moderationApi.POST("/content/:id/remove", controller.RemoveContent)   // Remove inappropriate content
		moderationApi.GET("/content/queue", controller.GetModerationQueue)    // Get moderation queue

		// Automated Moderation
		moderationApi.GET("/filters", controller.GetContentFilters)          // Get content filters
		moderationApi.POST("/filters", controller.CreateContentFilter)       // Create content filter
		moderationApi.PUT("/filters/:id", controller.UpdateContentFilter)    // Update content filter
		moderationApi.DELETE("/filters/:id", controller.DeleteContentFilter) // Delete content filter
	}

	// ============ SYSTEM MANAGEMENT ============
	systemApi := adminApi.Group("system")
	{
		// System Information
		systemApi.GET("/info", controller.GetSystemInfo)                // Get system information
		systemApi.GET("/health", controller.GetSystemHealth)            // Get system health status
		systemApi.GET("/metrics", controller.GetSystemMetrics)          // Get system metrics
		systemApi.GET("/logs", controller.GetSystemLogs)                // Get system logs
		systemApi.GET("/performance", controller.GetPerformanceMetrics) // Get performance metrics

		// Database Management
		systemApi.GET("/database/stats", controller.GetDatabaseStats)       // Get database statistics
		systemApi.POST("/database/backup", controller.CreateDatabaseBackup) // Create database backup
		systemApi.GET("/database/backups", controller.GetDatabaseBackups)   // Get backup list
		systemApi.POST("/database/restore", controller.RestoreDatabase)     // Restore from backup
		systemApi.POST("/database/cleanup", controller.CleanupDatabase)     // Cleanup old data

		// Cache Management
		systemApi.GET("/cache/stats", controller.GetCacheStats) // Get cache statistics
		systemApi.POST("/cache/clear", controller.ClearCache)   // Clear application cache
		systemApi.POST("/cache/warm", controller.WarmCache)     // Warm up cache
	}

	// ============ ANALYTICS & REPORTING ============
	analyticsApi := adminApi.Group("analytics")
	{
		// User Analytics
		analyticsApi.GET("/users", controller.GetUserAnalytics)                // Get user analytics
		analyticsApi.GET("/users/growth", controller.GetUserGrowthStats)       // Get user growth statistics
		analyticsApi.GET("/users/activity", controller.GetUserActivityStats)   // Get user activity statistics
		analyticsApi.GET("/users/retention", controller.GetUserRetentionStats) // Get user retention statistics

		// Platform Analytics
		analyticsApi.GET("/messages", controller.GetMessageAnalytics)    // Get message analytics
		analyticsApi.GET("/groups", controller.GetGroupAnalytics)        // Get group analytics
		analyticsApi.GET("/engagement", controller.GetEngagementMetrics) // Get engagement metrics
		analyticsApi.GET("/revenue", controller.GetRevenueAnalytics)     // Get revenue analytics (if applicable)

		// Custom Reports
		analyticsApi.GET("/reports", controller.GetCustomReports)         // Get available custom reports
		analyticsApi.POST("/reports", controller.CreateCustomReport)      // Create custom report
		analyticsApi.GET("/reports/:id", controller.GetCustomReport)      // Get specific custom report
		analyticsApi.POST("/reports/:id/export", controller.ExportReport) // Export report
	}

	// ============ CONFIGURATION MANAGEMENT ============
	configApi := adminApi.Group("config")
	{
		// Application Settings
		configApi.GET("/settings", controller.GetAppSettings)     // Get application settings
		configApi.PUT("/settings", controller.UpdateAppSettings)  // Update application settings
		configApi.GET("/features", controller.GetFeatureFlags)    // Get feature flags
		configApi.PUT("/features", controller.UpdateFeatureFlags) // Update feature flags

		// Email & Notifications
		configApi.GET("/email", controller.GetEmailSettings)                 // Get email configuration
		configApi.PUT("/email", controller.UpdateEmailSettings)              // Update email configuration
		configApi.POST("/email/test", controller.TestEmailSettings)          // Test email configuration
		configApi.GET("/notifications", controller.GetNotificationConfig)    // Get notification configuration
		configApi.PUT("/notifications", controller.UpdateNotificationConfig) // Update notification configuration

		// Security Settings
		configApi.GET("/security", controller.GetSecuritySettings)        // Get security settings
		configApi.PUT("/security", controller.UpdateSecuritySettings)     // Update security settings
		configApi.GET("/rate-limits", controller.GetRateLimitSettings)    // Get rate limit settings
		configApi.PUT("/rate-limits", controller.UpdateRateLimitSettings) // Update rate limit settings
	}

	// ============ ROLE & PERMISSION MANAGEMENT ============
	rbacApi := adminApi.Group("rbac")
	{
		// Roles Management
		rbacApi.GET("/roles", controller.GetRoles)          // Get all roles
		rbacApi.POST("/roles", controller.CreateRole)       // Create new role
		rbacApi.GET("/roles/:id", controller.GetRole)       // Get specific role
		rbacApi.PUT("/roles/:id", controller.UpdateRole)    // Update role
		rbacApi.DELETE("/roles/:id", controller.DeleteRole) // Delete role

		// Permissions Management
		rbacApi.GET("/permissions", controller.GetPermissions)          // Get all permissions
		rbacApi.POST("/permissions", controller.CreatePermission)       // Create new permission
		rbacApi.PUT("/permissions/:id", controller.UpdatePermission)    // Update permission
		rbacApi.DELETE("/permissions/:id", controller.DeletePermission) // Delete permission

		// Role-Permission Assignment
		rbacApi.POST("/roles/:roleId/permissions/:permissionId", controller.AssignPermissionToRole)     // Assign permission to role
		rbacApi.DELETE("/roles/:roleId/permissions/:permissionId", controller.RemovePermissionFromRole) // Remove permission from role
		rbacApi.GET("/roles/:roleId/permissions", controller.GetRolePermissions)                        // Get role permissions
	}

	// ============ AUDIT & LOGGING ============
	auditApi := adminApi.Group("audit")
	{
		auditApi.GET("/logs", controller.GetAuditLogs)            // Get audit logs
		auditApi.GET("/logs/:id", controller.GetAuditLog)         // Get specific audit log
		auditApi.GET("/actions", controller.GetAdminActions)      // Get admin actions log
		auditApi.GET("/changes", controller.GetDataChanges)       // Get data change history
		auditApi.POST("/logs/export", controller.ExportAuditLogs) // Export audit logs
	}

	// ============ MAINTENANCE & OPERATIONS ============
	maintenanceApi := adminApi.Group("maintenance")
	{
		maintenanceApi.POST("/mode/enable", controller.EnableMaintenanceMode)   // Enable maintenance mode
		maintenanceApi.POST("/mode/disable", controller.DisableMaintenanceMode) // Disable maintenance mode
		maintenanceApi.GET("/mode/status", controller.GetMaintenanceStatus)     // Get maintenance status
		maintenanceApi.POST("/tasks/run", controller.RunMaintenanceTask)        // Run maintenance task
		maintenanceApi.GET("/tasks", controller.GetMaintenanceTasks)            // Get available maintenance tasks
		maintenanceApi.GET("/tasks/:id/status", controller.GetTaskStatus)       // Get task execution status
	}
}
