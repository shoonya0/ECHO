package controller

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/shoonya0/ECHO/internal/constants"
	"github.com/shoonya0/ECHO/internal/database"
	"github.com/shoonya0/ECHO/internal/logger"
	"github.com/shoonya0/ECHO/internal/models"
	"github.com/shoonya0/ECHO/internal/services"
	"github.com/shoonya0/ECHO/internal/utils"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

type SignupRequest struct {
	ID       bson.ObjectID `json:"_id" bson:"_id"`
	Email    string        `json:"email" binding:"required,email"`
	Username string        `json:"username" binding:"required"`
	Password string        `json:"password" binding:"required,min=8"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Signup creates a new user account.
// POST /echo/v1/signup
func Signup(c *gin.Context) {
	log := logger.WithContext(c.Request.Context())

	var req SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.WithError(err).Debug("invalid request")
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request", err.Error())
		return
	}

	count, err := database.DB.Collection(string(constants.UserColl)).CountDocuments(c.Request.Context(), bson.M{"email": req.Email})
	if err != nil {
		log.WithError(err).Error("failed to check email")
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to check email", err.Error())
		return
	}
	if count > 0 {
		log.Debug("email already registered")
		utils.ErrorResponse(c, http.StatusConflict, "email already registered", nil)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.WithError(err).Debug("could not hash password")
		utils.ErrorResponse(c, http.StatusInternalServerError, "could not hash password", err.Error())
		return
	}

	user := utils.NewUserWithDefaults(req.ID, req.Email, req.Username, string(hash))

	if _, err := database.DB.Collection(string(constants.UserColl)).InsertOne(c.Request.Context(), user); err != nil {
		log.WithError(err).Error("failed to create user")
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to create user", err.Error())
		return
	}

	log.WithField("user", user).Info("user created successfully")
	utils.SuccessResponse(c, "User created successfully", user)
}

// Login verifies credentials and returns a 7-day JWT.
// POST /echo/v1/login
func Login(c *gin.Context) {
	log := logger.WithContext(c.Request.Context())

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Debug("invalid request")
		utils.ErrorResponse(c, http.StatusBadRequest, "invalid request", err.Error())
		return
	}

	if req.Email == "" || req.Password == "" {
		log.Debug("email and password are required")
		utils.ErrorResponse(c, http.StatusBadRequest, "email and password are required", nil)
		return
	}

	userProjection := bson.M{
		"_id":                 1,
		"email":               1,
		"username":            1,
		"passwordHash":        1,
		"profile.displayName": 1,
		"profile.avatar":      1,
		"accountStatus":       1,
		"presence.status":     1,
		"presence.lastSeen":   1,
	}

	// Decode into raw bson.M first to detect missing accountStatus field (legacy seed accounts).
	var rawDoc bson.M
	err := database.DB.Collection(string(constants.UserColl)).FindOne(c.Request.Context(), bson.M{"email": req.Email}, options.FindOne().SetProjection(userProjection)).Decode(&rawDoc)
	if err == mongo.ErrNoDocuments {
		log.Debug("invalid credentials")
		utils.ErrorResponse(c, http.StatusUnauthorized, "invalid credentials", nil)
		return
	} else if err != nil {
		log.WithError(err).Error("failed to find user")
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to find user", err.Error())
		return
	}

	// If accountStatus is missing from the stored document (legacy / direct-seed account),
	// inject a default-active status so the typed decode produces the correct value.
	if _, hasAccountStatus := rawDoc["accountStatus"]; !hasAccountStatus {
		rawDoc["accountStatus"] = bson.M{
			"isActive":   true,
			"isVerified": false,
			"isBanned":   false,
		}
	}

	// Convert raw document to typed struct.
	var user models.LoginUserResponse
	bsonBytes, _ := bson.Marshal(rawDoc)
	if err := bson.Unmarshal(bsonBytes, &user); err != nil {
		log.WithError(err).Error("failed to decode user document")
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to decode user", err.Error())
		return
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash), []byte(req.Password),
	); err != nil {
		log.WithError(err).Debug("invalid credentials")
		utils.ErrorResponse(c, http.StatusUnauthorized, "invalid credentials", nil)
		return
	}

	if user.AccountStatus.IsBanned {
		log.Debug("account is banned")
		utils.ErrorResponse(c, http.StatusForbidden, "account is banned", nil)
		return
	}

	if !user.AccountStatus.IsActive {
		log.Debug("account is not active")
		utils.ErrorResponse(c, http.StatusForbidden, "account is not active", nil)
		return
	}

	// Normalize displayName: legacy/seeded accounts may lack it.
	// Without this the JWT carries an empty "displayName" claim, and
	// AuthMiddleware rejects it with "missing display name in token claims".
	displayNameFixed := false
	normalizedDisplayName := normalizeDisplayName(user.Profile.DisplayName, user.Username, user.Email)
	if normalizedDisplayName != user.Profile.DisplayName {
		user.Profile.DisplayName = normalizedDisplayName
		displayNameFixed = true
	}

	newUser := models.LoginUserResponse{
		ID:            user.ID,
		Email:         user.Email,
		Username:      user.Username,
		Profile:       user.Profile,
		Presence:      user.Presence,
		AccountStatus: user.AccountStatus,
	}

	token, err := utils.GetJWTToken(newUser, time.Now().Add(7*24*time.Hour).Unix())
	if err != nil {
		log.WithError(err).Debug("could not generate token")
		utils.ErrorResponse(c, http.StatusInternalServerError, "could not generate token", err.Error())
		return
	}

	// Self-heal the stored document so the fix persists.
	if displayNameFixed {
		go func() {
			_, uErr := database.DB.Collection(string(constants.UserColl)).UpdateOne(
				context.Background(),
				bson.M{"_id": user.ID},
				bson.M{"$set": bson.M{"profile.displayName": normalizedDisplayName}},
			)
			if uErr != nil {
				log.WithError(uErr).WithField("user", user.ID.Hex()).Debug("failed to self-heal display name")
			}
		}()
	}

	log.WithField("user", newUser.ID.Hex()).Info("login successful")
	utils.SuccessResponse(c, "Login successful", gin.H{
		"token": token,
		"user":  newUser,
	})
}

// Logout invalidates the current JWT by adding its JTI to a Redis blacklist.
func Logout(c *gin.Context) {
	reqCtx, log, ok := ReduceGinContextToContext(c)
	if !ok {
		return
	}

	jwtClaimsValue, exists := c.Get("jwtClaims")
	if !exists {
		log.Debug("jwt claims not found in context")
		utils.ErrorResponse(c, http.StatusUnauthorized, "jwt claims not found", nil)
		return
	}

	claims, ok := jwtClaimsValue.(utils.JwtClaims)
	if !ok {
		log.Debug("invalid jwt claims type in context")
		utils.ErrorResponse(c, http.StatusInternalServerError, "invalid token claims", nil)
		return
	}

	jti := claims.RegisteredClaims.ID
	exp := claims.RegisteredClaims.ExpiresAt.Time

	if err := services.LogoutUser(reqCtx, jti, exp); err != nil {
		log.WithError(err).Debug("failed to logout user")
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to logout", nil)
		return
	}

	log.Info("User logged out successfully")
	utils.SuccessResponse(c, "Logged out successfully", nil)
}

// normalizeDisplayName ensures a display name is never empty by falling
// back through usable alternatives: displayName → username → email local-part → "User".
func normalizeDisplayName(displayName, username, email string) string {
	candidate := strings.TrimSpace(displayName)
	if candidate != "" {
		return candidate
	}

	candidate = strings.TrimSpace(username)
	if candidate != "" {
		return candidate
	}

	if idx := strings.LastIndex(email, "@"); idx > 0 {
		candidate = strings.TrimSpace(email[:idx])
		if candidate != "" {
			return candidate
		}
	}

	return "User"
}
