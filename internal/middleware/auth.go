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

	// The token may have been logged out, or its account banned, deactivated or
	// deleted since login (#33). Open WebSockets re-run the same check (#76). If it
	// can't be made, refuse with 503 rather than let the token through. Expiry was
	// checked above, so it isn't passed.
	if err := services.CheckSession(reqCtx, objectID, claims.ID, time.Time{}); err != nil {
		refuseSession(ctx, log, err)
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

	log.WithField("userId", user.ID.Hex()).Debug("User authenticated successfully")

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

// sessionRefusals is the reply for each services.SessionEndCode: 403 for a banned
// or inactive account, as Login answers, and 401 otherwise.
var sessionRefusals = map[string]struct {
	status  int
	message string
}{
	services.CodeTokenExpired:    {http.StatusUnauthorized, "token has expired"},
	services.CodeTokenRevoked:    {http.StatusUnauthorized, "token has been revoked"},
	services.CodeAccountNotFound: {http.StatusUnauthorized, "account no longer exists"},
	services.CodeAccountBanned:   {http.StatusForbidden, "account is banned"},
	services.CodeAccountInactive: {http.StatusForbidden, "account is not active"},
}

// refuseSession answers a valid token whose session may no longer be used, or 503
// when the check couldn't be made.
func refuseSession(ctx *gin.Context, log *logrus.Entry, err error) {
	code, ok := services.SessionEndCode(err)
	refusal, known := sessionRefusals[code]
	if !ok || !known {
		refuseUnavailable(ctx, log, err)
		return
	}
	log.WithField("code", code).Info("Refused a token whose session ended")
	ctx.JSON(refusal.status, gin.H{"error": refusal.message, "code": code})
	ctx.Abort()
}
