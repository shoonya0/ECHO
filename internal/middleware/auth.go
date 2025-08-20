package middleware

import (
	"fmt"
	"gin/internal/models"
	"gin/internal/utils"
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
		Email:         "",
		Username:      "",
		Profile:       models.UserProfileEmbed{},
		AccountStatus: models.AccountStatusEmbed{},
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
	authHeader := ctx.GetHeader("Authorization")

	if authHeader == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": "authorization header is required",
			"code":  "MISSING_AUTH_HEADER",
		})
		ctx.Abort()
		return
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": "authorization header must be in 'Bearer <token>' format",
			"code":  "INVALID_AUTH_FORMAT",
		})
		ctx.Abort()
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": "token cannot be empty",
			"code":  "EMPTY_TOKEN",
		})
		ctx.Abort()
		return
	}

	claims, err := verifyToken(token)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"code":    "TOKEN_VERIFICATION_FAILED",
			"details": err.Error(),
			"error":   "invalid or expired token",
		})
		ctx.Abort()
		return
	}

	ctx.Set("userId", claims.RegisteredClaims.Subject)

	objectID, err := bson.ObjectIDFromHex(claims.RegisteredClaims.Subject)
	if err != nil {
		utils.ErrorResponse(ctx, http.StatusUnauthorized, "invalid user id format", err.Error())
		ctx.Abort()
		return
	}

	ctx.Set("user", models.LoginUserResponse{
		ID:            objectID,
		Email:         claims.Email,
		Username:      claims.Username,
		Profile:       claims.Profile,
		AccountStatus: claims.AccountStatus,
	})

	ctx.Next()
}
