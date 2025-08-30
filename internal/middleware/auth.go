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
	} else if claims.Profile.DisplayName == "" {
		return fmt.Errorf("missing display name in token claims")
	} else if !claims.AccountStatus.IsActive {
		return fmt.Errorf("account is not active")
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
		Email:    "",
		Username: "",
		Profile: models.UserProfileEmbed{
			DisplayName:   "",
			Avatar:        "",
			StatusMessage: "",
			Bio:           "",
		},
		AccountStatus: models.AccountStatusEmbed{
			IsActive:   false,
			IsVerified: false,
			IsBanned:   false,
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
	log.WithField("path", ctx.Request.URL.Path).Debug("Processing authentication")

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
		log.WithError(err).Error("Token verification failed")
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
		log.WithError(err).Error("Invalid user ID format")
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "invalid user id format", err.Error())
		ctx.Abort()
		return
	}

	user := models.LoginUserResponse{
		ID:            objectID,
		Email:         claims.Email,
		Username:      claims.Username,
		Profile:       claims.Profile,
		AccountStatus: claims.AccountStatus,
	}

	ctx.Set("user", user)
	log.WithFields(map[string]interface{}{
		"user_id":  user.ID.Hex(),
		"email":    user.Email,
		"username": user.Username,
	}).Info("User authenticated successfully")

	ctx.Next()
}
