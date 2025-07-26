package middleware

import (
	"fmt"
	"gin/objects"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type JwtClaims struct {
	jwt.RegisteredClaims
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Exp    int64  `json:"exp"`
}

func verifyToken(tokenString string) (JwtClaims, error) {
	claims := JwtClaims{}

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (interface{}, error) {
		// Verify the signing method
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(objects.MainConfiguration.JwtSecret), nil
	})

	if err != nil {
		return JwtClaims{}, fmt.Errorf("token parsing failed: %w", err)
	}

	// Check if token is valid
	if !token.Valid {
		return JwtClaims{}, fmt.Errorf("invalid token")
	}

	// Validate required claims
	if claims.UserID == "" {
		return JwtClaims{}, fmt.Errorf("missing user_id in token claims")
	}

	if claims.Email == "" {
		return JwtClaims{}, fmt.Errorf("missing email in token claims")
	}

	return claims, nil
}

func AuthMiddleware(ctx *gin.Context) {
	authHeader := ctx.GetHeader("Authorization")

	// Check if Authorization header exists and has proper format
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

	// Extract token from header
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{
			"error": "token cannot be empty",
			"code":  "EMPTY_TOKEN",
		})
		ctx.Abort()
		return
	}

	// Verify the token
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

	// Set user information in context only after successful verification
	ctx.Set("userId", claims.UserID)
	ctx.Set("email", claims.Email)
	ctx.Set("exp", claims.Exp)
	// Continue to next handler
	ctx.Next()
}
