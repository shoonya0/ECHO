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
	DisplayName   string                    `json:"displayName" bson:"displayName"`
	AccountStatus models.AccountStatusEmbed `json:"accountStatus" bson:"accountStatus"`
	Presence      PresenceClaims            `json:"presence" bson:"presence"`
}

type PresenceClaims struct {
	Status       string    `json:"status" bson:"status"`
	LastSeen     time.Time `json:"lastSeen" bson:"lastSeen"`
	LastActivity time.Time `json:"lastActivity" bson:"lastActivity"`
}

func GetJWTToken(user models.LoginUserResponse, exp int64) (string, error) {

	jwtSecret := GetJWTSecret()

	// create JWT token
	claims := JwtClaims{
		Email:         user.Email,
		Username:      user.Username,
		DisplayName:   user.Profile.DisplayName,
		AccountStatus: user.AccountStatus,
		Presence: PresenceClaims{
			Status:       user.Presence.Status,
			LastSeen:     user.Presence.LastSeen,
			LastActivity: user.Presence.LastActivity,
		},
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "Echo",
			Subject:   user.ID.Hex(),
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
