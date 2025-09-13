package routes

import (
	"gin/internal/controller"
	"gin/internal/middleware"
	"gin/internal/models"
	"gin/internal/utils"
	"gin/logger"
	"gin/objects"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
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

func Signup() gin.HandlerFunc {
	return func(c *gin.Context) {
		log := logger.WithContext(c.Request.Context())

		var req SignupRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			log.WithError(err).Debug("invalid request")
			utils.ErrorResponse(c, http.StatusBadRequest, "invalid request", err.Error())
			return
		}

		count, err := objects.DB.Collection(string(objects.UserColl)).CountDocuments(c.Request.Context(), bson.M{"email": req.Email})
		if err != nil {
			log.WithError(err).Error("failed to check email")
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to check email", err.Error())
			return
		}
		if count > 0 {
			log.WithError(err).Debug("email already registered")
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

		if _, err := objects.DB.Collection(string(objects.UserColl)).InsertOne(c.Request.Context(), user); err != nil {
			log.WithError(err).Error("failed to create user")
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to create user", err.Error())
			return
		}

		log.WithField("user", user).Info("user created successfully")
		utils.SuccessResponse(c, "User created successfully", user)
	}
}

func Login() gin.HandlerFunc {
	return func(c *gin.Context) {
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
		}

		var user models.LoginUserResponse

		err := objects.DB.Collection(string(objects.UserColl)).FindOne(c.Request.Context(), bson.M{"email": req.Email}, options.FindOne().SetProjection(userProjection)).Decode(&user)
		if err == mongo.ErrNoDocuments {
			log.Debug("invalid credentials")
			utils.ErrorResponse(c, http.StatusUnauthorized, "invalid credentials", err.Error())
			return
		} else if err != nil {
			log.WithError(err).Error("failed to find user")
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to find user", err.Error())
			return
		}

		if err := bcrypt.CompareHashAndPassword(
			[]byte(user.PasswordHash), []byte(req.Password),
		); err != nil {
			log.WithError(err).Debug("invalid credentials")
			utils.ErrorResponse(c, http.StatusUnauthorized, "invalid credentials", err.Error())
			return
		}

		token, err := utils.GetJWTToken(user.ID.Hex(), user.Email, user.Username, user.Profile, user.AccountStatus, time.Now().Add(7*24*time.Hour).Unix())
		if err != nil {
			log.WithError(err).Debug("could not generate token")
			utils.ErrorResponse(c, http.StatusInternalServerError, "could not generate token", err.Error())
			return
		}

		log.WithField("user", user.ID.Hex()).Info("login successful")
		utils.SuccessResponse(c, "Login successful", gin.H{
			"token": token,
			"user":  user,
		})
	}
}

func RegisterAPIRoutes(r *gin.Engine) {
	r.POST(objects.ApiBasePath+"login", Login())
	r.POST(objects.ApiBasePath+"signup", Signup())

	// Register WebSocket routes BEFORE auth middleware (they handle auth internally)
	RegisterWebSocketRoutes(r)

	r.Use(middleware.AuthMiddleware)
	r.Use(middleware.LoggerMiddleware())

	RegisterUserRoutes(r)
	RegisterChatRoutes(r)
}

// RegisterWebSocketRoutes registers WebSocket endpoints that handle authentication internally
func RegisterWebSocketRoutes(r *gin.Engine) {
	wsGroup := r.Group(objects.ApiBasePath + "ws")
	wsGroup.Use(middleware.AuthMiddleware) // Apply auth middleware to WebSocket routes
	wsGroup.Use(middleware.LoggerMiddleware())

	wsGroup.GET("/chat", controller.HandleWebSocketChat)
}
