package utils

import (
	"gin/objects"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

func GetJWTSecret() []byte {
	secret := objects.MainConfiguration.JwtSecret

	// Trim whitespace and quotes that might come from env file
	secret = strings.TrimSpace(secret)
	secret = strings.Trim(secret, "\"'")

	if secret == "" {
		panic("JWT_SECRET is not set in configuration")
	}

	return []byte(secret)
}

func GetJWTToken(userID string, email string, exp int64) (string, error) {

	// create JWT token
	claims := jwt.MapClaims{
		"user_id": userID, // Convert ObjectID to string
		"email":   email,
		"exp":     exp,
	}

	// Debug: Print secret length for verification
	jwtSecret := GetJWTSecret()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return signed, nil
}
