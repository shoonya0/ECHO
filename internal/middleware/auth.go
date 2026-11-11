package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/shoonya0/ECHO/internal/config"
	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// validateClaims checks the token's own fields. Whether the account may still use the
// token (banned, inactive, deleted) is decided from the database in AuthMiddleware,
// not from the accountStatus copied into the token at login (#33).
func validateClaims(claims *utils.JwtClaims) error {
	if claims.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("token is expired")
	}

	if claims.NotBefore.After(time.Now()) {
		return fmt.Errorf("token is not valid yet")
	}

	if claims.Subject == "" {
		return fmt.Errorf("missing user_id in token claims")
	}
	// Logout revokes a token by its ID, so a token without one can't be revoked alone.
	if claims.ID == "" {
		return fmt.Errorf("missing jti in token claims")
	}
	if claims.Username == "" {
		return fmt.Errorf("missing username in token claims")
	}
	if claims.Email == "" {
		return fmt.Errorf("missing email in token claims")
	}

	// Tokens from accounts created before displayName was required.
	if claims.DisplayName == "" {
		claims.DisplayName = claims.Username
	}

	return nil
}

func verifyToken(tokenString string) (utils.JwtClaims, error) {
	claims := utils.JwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "",
			Issuer:    "",
			Audience:  jwt.ClaimStrings{},
			ExpiresAt: jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        "",
		},
		Email:       "",
		Username:    "",
		DisplayName: "",
		AccountStatus: models.AccountStatusEmbed{
			IsActive:   false,
			IsVerified: false,
			IsBanned:   false,
		},
		Presence: utils.PresenceClaims{
			Status:       "",
			LastSeen:     time.Now(),
			LastActivity: time.Now(),
		},
	}

	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(config.Cfg.JWTSecret), nil
	})

	if err != nil {
		return utils.JwtClaims{}, fmt.Errorf("token parsing failed: %w", err)
	}

	if !token.Valid {
		return utils.JwtClaims{}, fmt.Errorf("invalid token")
	}

	if err := validateClaims(&claims); err != nil {
		return utils.JwtClaims{}, err
	}

	return claims, nil
}

func AuthMiddleware(ctx *gin.Context) {
	// Create a new request context with transaction ID
	reqCtx := logger.WithTransactionID(ctx.Request.Context())
	log := logger.WithContext(reqCtx)

	// Update request context
	ctx.Request = ctx.Request.WithContext(reqCtx)

	authHeader := ctx.GetHeader("Authorization")

	if authHeader == "" {
		log.Warn("Missing authorization header")
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": "authorization header is required",
			"code":  "MISSING_AUTH_HEADER",
		})
		ctx.Abort()
		return
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		log.Warn("Invalid authorization format")
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": "authorization header must be in 'Bearer <token>' format",
			"code":  "INVALID_AUTH_FORMAT",
		})
		ctx.Abort()
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		log.Warn("Empty token provided")
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": "token cannot be empty",
			"code":  "EMPTY_TOKEN",
		})
		ctx.Abort()
		return
	}

	claims, err := verifyToken(token)
	if err != nil {
		log.WithError(err).Debug("Token verification failed")
		refuseToken(ctx)
		return
	}

	objectID, err := bson.ObjectIDFromHex(claims.Subject)
	if err != nil {
		log.WithError(err).Debug("Token subject is not a user ID")
		refuseToken(ctx)
		return
	}

	// Add user ID to context for logging
	reqCtx = logger.WithUserID(reqCtx, claims.Subject)
	ctx.Request = ctx.Request.WithContext(reqCtx)
	log = logger.WithContext(reqCtx)

	// Check the token blacklist (server-side logout). If Redis can't answer, refuse
	// with 503 rather than let a revoked token through (#33).
	revoked, err := services.IsTokenRevoked(reqCtx, claims.ID)
	if err != nil {
		refuseUnavailable(ctx, log, err)
		return
	}
	if revoked {
		log.Warn("Token has been revoked")
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": "token has been revoked",
			"code":  "TOKEN_REVOKED",
		})
		ctx.Abort()
		return
	}

	// The account may have been banned, deactivated or deleted since login (#33).
	if err := services.CheckAccountAccess(reqCtx, objectID); err != nil {
		refuseAccount(ctx, log, err)
		return
	}

	user := models.LoginUserResponse{
		ID:       objectID,
		Email:    claims.Email,
		Username: claims.Username,
		Profile: models.UserProfileEmbed{
			DisplayName: claims.DisplayName,
		},
		Presence: models.PresenceEmbed{
			Status:       claims.Presence.Status,
			LastSeen:     claims.Presence.LastSeen,
			LastActivity: claims.Presence.LastActivity,
		},
		AccountStatus: claims.AccountStatus,
	}

	ctx.Set("userId", claims.Subject)
	ctx.Set("user", user)
	ctx.Set("jwtClaims", claims)

	log.WithFields(map[string]interface{}{
		"userId":   user.ID.Hex(),
		"email":    user.Email,
		"username": user.Username,
	}).Debug("User authenticated successfully")

	ctx.Next()
}

// refuseToken answers a token that can't be used, without saying why: the reason is
// logged, not sent (#33, #41).
func refuseToken(ctx *gin.Context) {
	ctx.JSON(http.StatusUnauthorized, gin.H{
		"error": "invalid or expired token",
		"code":  "TOKEN_VERIFICATION_FAILED",
	})
	ctx.Abort()
}

// refuseUnavailable answers when the token or the account couldn't be checked. A
// client that hung up (context canceled) isn't an outage, so it isn't logged as one.
func refuseUnavailable(ctx *gin.Context, log *logrus.Entry, err error) {
	if errors.Is(err, context.Canceled) {
		log.WithError(err).Debug("Auth check stopped: the client went away")
	} else {
		log.WithError(err).Error("Auth check failed; refusing the request")
	}
	ctx.JSON(http.StatusServiceUnavailable, gin.H{
		"error": "authentication is temporarily unavailable",
		"code":  "AUTH_UNAVAILABLE",
	})
	ctx.Abort()
}

// refuseAccount answers a valid token whose account may no longer use it: 403 for a
// banned or inactive account, as Login answers, and 401 for a deleted one.
func refuseAccount(ctx *gin.Context, log *logrus.Entry, err error) {
	status := http.StatusForbidden
	var message, code string
	switch {
	case errors.Is(err, services.ErrAccountBanned):
		message, code = "account is banned", "ACCOUNT_BANNED"
	case errors.Is(err, services.ErrAccountInactive):
		message, code = "account is not active", "ACCOUNT_INACTIVE"
	case errors.Is(err, services.ErrUserNotFound):
		status, message, code = http.StatusUnauthorized, "account no longer exists", "ACCOUNT_NOT_FOUND"
	default:
		refuseUnavailable(ctx, log, err)
		return
	}
	log.WithField("code", code).Info("Refused a token for an account that can't use it")
	ctx.JSON(status, gin.H{"error": message, "code": code})
	ctx.Abort()
}
