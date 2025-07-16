package routes

import (
	"gin/internal/controller"
	"gin/objects"

	"github.com/gin-gonic/gin"
)

// RegisterAuthRoutes registers all authentication-related routes
func RegisterAuthRoutes(r *gin.Engine) {
	// ============ BASIC AUTHENTICATION ============
	authRoutes := r.Group(objects.AuthBasePath)
	{
		// Core Authentication
		authRoutes.POST("/register", controller.Register)    // Create new account
		authRoutes.POST("/login", controller.Login)          // Login with credentials
		authRoutes.POST("/refresh", controller.Refresh)      // Refresh access token
		authRoutes.POST("/logout", controller.Logout)        // Logout current session
		authRoutes.POST("/logout-all", controller.LogoutAll) // Logout from all devices

		// Password Management
		authRoutes.POST("/forgot-password", controller.ForgotPassword) // Request password reset
		authRoutes.POST("/reset-password", controller.ResetPassword)   // Reset password with token
		authRoutes.POST("/change-password", controller.ChangePassword) // Change password (authenticated)
		authRoutes.POST("/verify-password", controller.VerifyPassword) // Verify current password
	}

	// ============ EMAIL/PHONE VERIFICATION ============
	verificationRoutes := authRoutes.Group("/verification")
	{
		// Email Verification
		verificationRoutes.POST("/email/send", controller.SendEmailVerification)     // Send verification email
		verificationRoutes.POST("/email/verify", controller.VerifyEmail)             // Verify email with token
		verificationRoutes.POST("/email/resend", controller.ResendEmailVerification) // Resend verification email

		// Phone Verification
		verificationRoutes.POST("/phone/send", controller.SendPhoneVerification)     // Send SMS verification
		verificationRoutes.POST("/phone/verify", controller.VerifyPhone)             // Verify phone with code
		verificationRoutes.POST("/phone/resend", controller.ResendPhoneVerification) // Resend SMS code
	}

	// ============ TWO-FACTOR AUTHENTICATION ============
	twoFactorRoutes := authRoutes.Group("/2fa")
	{
		// TOTP (Time-based One-Time Password)
		twoFactorRoutes.POST("/totp/enable", controller.EnableTOTP)   // Enable TOTP 2FA
		twoFactorRoutes.POST("/totp/verify", controller.VerifyTOTP)   // Verify TOTP code
		twoFactorRoutes.POST("/totp/disable", controller.DisableTOTP) // Disable TOTP 2FA
		twoFactorRoutes.GET("/totp/qr", controller.GetTOTPQRCode)     // Get QR code for setup

		// SMS-based 2FA
		twoFactorRoutes.POST("/sms/enable", controller.EnableSMS2FA)   // Enable SMS 2FA
		twoFactorRoutes.POST("/sms/send", controller.SendSMS2FA)       // Send SMS code
		twoFactorRoutes.POST("/sms/verify", controller.VerifySMS2FA)   // Verify SMS code
		twoFactorRoutes.POST("/sms/disable", controller.DisableSMS2FA) // Disable SMS 2FA

		// Backup Codes
		twoFactorRoutes.GET("/backup-codes", controller.GetBackupCodes)                    // Get backup codes
		twoFactorRoutes.POST("/backup-codes/regenerate", controller.RegenerateBackupCodes) // Regenerate backup codes
		twoFactorRoutes.POST("/backup-codes/verify", controller.VerifyBackupCode)          // Verify backup code

		// 2FA Status
		twoFactorRoutes.GET("/status", controller.Get2FAStatus) // Get 2FA status
	}

	// ============ OAUTH AUTHENTICATION ============
	oauthRoutes := authRoutes.Group("/oauth")
	{
		// OAuth Providers
		oauthRoutes.GET("/google", controller.GoogleOAuthLogin)                 // Initiate Google OAuth
		oauthRoutes.GET("/google/callback", controller.GoogleOAuthCallback)     // Google OAuth callback
		oauthRoutes.GET("/github", controller.GitHubOAuthLogin)                 // Initiate GitHub OAuth
		oauthRoutes.GET("/github/callback", controller.GitHubOAuthCallback)     // GitHub OAuth callback
		oauthRoutes.GET("/facebook", controller.FacebookOAuthLogin)             // Initiate Facebook OAuth
		oauthRoutes.GET("/facebook/callback", controller.FacebookOAuthCallback) // Facebook OAuth callback
		oauthRoutes.GET("/discord", controller.DiscordOAuthLogin)               // Initiate Discord OAuth
		oauthRoutes.GET("/discord/callback", controller.DiscordOAuthCallback)   // Discord OAuth callback

		// Account Linking
		oauthRoutes.POST("/link/google", controller.LinkGoogleAccount)       // Link Google account
		oauthRoutes.POST("/link/github", controller.LinkGitHubAccount)       // Link GitHub account
		oauthRoutes.DELETE("/unlink/google", controller.UnlinkGoogleAccount) // Unlink Google account
		oauthRoutes.DELETE("/unlink/github", controller.UnlinkGitHubAccount) // Unlink GitHub account
		oauthRoutes.GET("/linked-accounts", controller.GetLinkedAccounts)    // Get linked accounts
	}

	// ============ SESSION MANAGEMENT ============
	sessionRoutes := authRoutes.Group("/sessions")
	{
		sessionRoutes.GET("/", controller.GetActiveSessions)                // Get all active sessions
		sessionRoutes.GET("/current", controller.GetCurrentSession)         // Get current session info
		sessionRoutes.DELETE("/:sessionID", controller.TerminateSession)    // Terminate specific session
		sessionRoutes.DELETE("/", controller.TerminateAllSessions)          // Terminate all other sessions
		sessionRoutes.PUT("/:sessionID/refresh", controller.RefreshSession) // Refresh specific session
	}

	// ============ SECURITY & AUDIT ============
	securityRoutes := authRoutes.Group("/security")
	{
		// Login Activity
		securityRoutes.GET("/login-history", controller.GetLoginHistory)                        // Get login history
		securityRoutes.GET("/security-events", controller.GetSecurityEvents)                    // Get security events
		securityRoutes.POST("/suspicious-activity/report", controller.ReportSuspiciousActivity) // Report suspicious activity

		// Device Management
		securityRoutes.GET("/devices", controller.GetTrustedDevices)                // Get trusted devices
		securityRoutes.POST("/devices/trust", controller.TrustDevice)               // Trust current device
		securityRoutes.DELETE("/devices/:deviceID", controller.RemoveTrustedDevice) // Remove trusted device
		securityRoutes.POST("/devices/verify", controller.VerifyDevice)             // Verify device with code

		// Account Security
		securityRoutes.POST("/account/lock", controller.LockAccount)             // Lock account (admin)
		securityRoutes.POST("/account/unlock", controller.UnlockAccount)         // Unlock account (admin)
		securityRoutes.POST("/account/deactivate", controller.DeactivateAccount) // Deactivate account (user)
		securityRoutes.POST("/account/reactivate", controller.ReactivateAccount) // Reactivate account (user)
		securityRoutes.DELETE("/account/delete", controller.DeleteAccount)       // Delete account permanently
	}

	// ============ RATE LIMITING & SECURITY CHECKS ============
	securityCheckRoutes := authRoutes.Group("/checks")
	{
		securityCheckRoutes.POST("/password-strength", controller.CheckPasswordStrength)       // Check password strength
		securityCheckRoutes.POST("/email-available", controller.CheckEmailAvailability)        // Check email availability
		securityCheckRoutes.POST("/username-available", controller.CheckUsernameAvailability)  // Check username availability
		securityCheckRoutes.GET("/security-questions", controller.GetSecurityQuestions)        // Get security questions
		securityCheckRoutes.POST("/security-answers", controller.SetSecurityAnswers)           // Set security answers
		securityCheckRoutes.POST("/verify-security-answers", controller.VerifySecurityAnswers) // Verify security answers
	}

	// ============ ADMIN AUTHENTICATION ============
	adminAuthRoutes := authRoutes.Group("/admin")
	{
		// Admin-specific authentication
		adminAuthRoutes.POST("/login", controller.AdminLogin)                     // Admin login with elevated privileges
		adminAuthRoutes.POST("/impersonate/:userID", controller.ImpersonateUser)  // Impersonate user (admin)
		adminAuthRoutes.POST("/stop-impersonation", controller.StopImpersonation) // Stop impersonating user
		adminAuthRoutes.GET("/impersonation-log", controller.GetImpersonationLog) // Get impersonation log
	}

	// ============ API KEYS & TOKENS ============
	apiRoutes := authRoutes.Group("/api-keys")
	{
		apiRoutes.GET("/", controller.GetAPIKeys)                         // Get user's API keys
		apiRoutes.POST("/", controller.CreateAPIKey)                      // Create new API key
		apiRoutes.PUT("/:keyID", controller.UpdateAPIKey)                 // Update API key
		apiRoutes.DELETE("/:keyID", controller.RevokeAPIKey)              // Revoke API key
		apiRoutes.POST("/:keyID/regenerate", controller.RegenerateAPIKey) // Regenerate API key
		apiRoutes.GET("/:keyID/usage", controller.GetAPIKeyUsage)         // Get API key usage stats
	}

	// ============ MAGIC LINKS & PASSWORDLESS ============
	passwordlessRoutes := authRoutes.Group("/passwordless")
	{
		passwordlessRoutes.POST("/magic-link/send", controller.SendMagicLink)                       // Send magic link
		passwordlessRoutes.GET("/magic-link/verify/:token", controller.VerifyMagicLink)             // Verify magic link
		passwordlessRoutes.POST("/webauthn/register/begin", controller.BeginWebAuthnRegistration)   // Begin WebAuthn registration
		passwordlessRoutes.POST("/webauthn/register/finish", controller.FinishWebAuthnRegistration) // Finish WebAuthn registration
		passwordlessRoutes.POST("/webauthn/login/begin", controller.BeginWebAuthnLogin)             // Begin WebAuthn login
		passwordlessRoutes.POST("/webauthn/login/finish", controller.FinishWebAuthnLogin)           // Finish WebAuthn login
	}
}
