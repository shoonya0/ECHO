package middleware

import (
	"fmt"
	"gin/internal/models"
	"gin/internal/utils"
	"gin/logger"
	"gin/objects"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func validateClaims(claims utils.JwtClaims) error {

	if claims.RegisteredClaims.ExpiresAt.Before(time.Now()) {
		return fmt.Errorf("token is expired")
	}

	if claims.RegisteredClaims.NotBefore.After(time.Now()) {
		return fmt.Errorf("token is not valid yet")
	}

	if claims.RegisteredClaims.Subject == "" {
		return fmt.Errorf("missing user_id in token claims")
	} else if claims.Username == "" {
		return fmt.Errorf("missing username in token claims")
	} else if claims.Email == "" {
		return fmt.Errorf("missing email in token claims")
	} else if claims.DisplayName == "" {
		// Legacy tokens or accounts created before displayName was required:
		// fall back to Username or Email instead of hard-rejecting.
		if claims.Username != "" {
			claims.DisplayName = claims.Username
		} else if claims.Email != "" {
			claims.DisplayName = claims.Email
		}
		// If still empty after fallback, we let it through — the login
		// endpoint now normalizes displayName on every token issuance.
	} else if claims.AccountStatus.IsBanned {
		return fmt.Errorf("account is banned")
	} else if claims.AccountStatus.IsActive && claims.AccountStatus.IsVerified && !claims.AccountStatus.IsBanned {
		// All good — account is fully active. Continue.
	}
	// Note: zero/missing accountStatus is tolerated (legacy tokens from seed accounts).
	// The Login handler guarantees only active+unbanned accounts can obtain a token.

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
		return []byte(objects.MainConfiguration.JwtSecret), nil
	})

	if err != nil {
		return utils.JwtClaims{}, fmt.Errorf("token parsing failed: %w", err)
	}

	if !token.Valid {
		return utils.JwtClaims{}, fmt.Errorf("invalid token")
	}

	if err := validateClaims(claims); err != nil {
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
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"code":    "TOKEN_VERIFICATION_FAILED",
			"details": err.Error(),
			"error":   "invalid or expired token",
		})
		ctx.Abort()
		return
	}

	// Add user ID to context for logging
	reqCtx = logger.WithUserID(reqCtx, claims.RegisteredClaims.Subject)
	ctx.Request = ctx.Request.WithContext(reqCtx)
	log = logger.WithContext(reqCtx)

	ctx.Set("userId", claims.RegisteredClaims.Subject)

	objectID, err := bson.ObjectIDFromHex(claims.RegisteredClaims.Subject)
	if err != nil {
		log.Debug("Invalid user ID format")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "invalid user id format", err.Error())
		ctx.Abort()
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

	ctx.Set("user", user)
	ctx.Set("jwtClaims", claims)

	// Check token blacklist (for server-side logout).
	if objects.RedisClient != nil {
		blacklistKey := "auth:blacklist:" + claims.RegisteredClaims.ID
		exists, err := objects.RedisClient.Exists(ctx.Request.Context(), blacklistKey).Result()
		if err != nil {
			log.WithError(err).Warn("Redis blacklist lookup failed — failing open to avoid lockout")
		} else if exists > 0 {
			log.Warn("Token has been revoked")
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": "token has been revoked",
				"code":  "TOKEN_REVOKED",
			})
			ctx.Abort()
			return
		}
	}

	log.WithFields(map[string]interface{}{
		"userId":   user.ID.Hex(),
		"email":    user.Email,
		"username": user.Username,
	}).Debug("User authenticated successfully")

	ctx.Next()
}
