package utils

import (
	"gin/internal/models"
	"gin/objects"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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

type JwtClaims struct {
	jwt.RegisteredClaims
	Username      string                    `json:"username" bson:"username"`
	Email         string                    `json:"email"`
	Profile       models.UserProfileEmbed   `json:"profile" bson:"profile"`
	AccountStatus models.AccountStatusEmbed `json:"accountStatus" bson:"accountStatus"`
}

func GetJWTToken(userID string, email string, username string, profile models.UserProfileEmbed, accountStatus models.AccountStatusEmbed, exp int64) (string, error) {

	jwtSecret := GetJWTSecret()

	// create JWT token
	claims := JwtClaims{
		Email:         email,
		Username:      username,
		Profile:       profile,
		AccountStatus: accountStatus,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "Echo",
			Subject:   userID,
			Audience:  jwt.ClaimStrings{"Web-App", "Mobile-App"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ID:        uuid.New().String(),
		},
	}

	// Debug: Print secret length for verification

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return signed, nil
}
