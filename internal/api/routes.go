package routes

import (
	"gin/internal/models"
	"gin/internal/utils"
	"gin/objects"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"golang.org/x/crypto/bcrypt"
)

// type User struct {
// 	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
// 	Email        string             `bson:"email" json:"email"`
// 	PasswordHash string             `bson:"passwordHash,omitempty"`
// 	CreatedAt    time.Time          `bson:"createdAt" json:"createdAt"`
// }

// request bodies
type SignupRequest struct {
	ID       bson.ObjectID `json:"_id" bson:"_id"`
	Email    string        `json:"email" binding:"required,email"`
	Password string        `json:"password" binding:"required,min=8"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// Signup creates a new user
func Signup() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SignupRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// check if email already exists
		count, err := objects.DB.Collection(string(objects.UserColl)).CountDocuments(c.Request.Context(), bson.M{"email": req.Email})
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to check email", err)
			return
		}
		if count > 0 {
			utils.ErrorResponse(c, http.StatusConflict, "email already registered", nil)
			return
		}

		// hash password
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "could not hash password", err)
			return
		}

		// Create user with all embedded structures properly initialized
		user := utils.NewUserWithDefaults(req.ID, req.Email, string(hash))

		if _, err := objects.DB.Collection(string(objects.UserColl)).InsertOne(c.Request.Context(), user); err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to create user", err)
			return
		}

		utils.SuccessResponse(c, "User created successfully", user)
	}
}

// Login verifies credentials and returns a JWT
func Login() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			utils.ErrorResponse(c, http.StatusBadRequest, "invalid request", err)
			return
		}

		if req.Email == "" || req.Password == "" {
			utils.ErrorResponse(c, http.StatusBadRequest, "email and password are required", nil)
			return
		}

		// find user by email
		var user models.User
		err := objects.DB.Collection(string(objects.UserColl)).FindOne(c.Request.Context(),
			bson.M{"email": req.Email},
		).Decode(&user)
		if err == mongo.ErrNoDocuments {
			utils.ErrorResponse(c, http.StatusUnauthorized, "invalid credentials", err)
			return
		} else if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to find user", err)
			return
		}

		// compare password
		if err := bcrypt.CompareHashAndPassword(
			[]byte(user.PasswordHash), []byte(req.Password),
		); err != nil {
			utils.ErrorResponse(c, http.StatusUnauthorized, "invalid credentials", err)
			return
		}

		// generate JWT token
		token, err := utils.GetJWTToken(user.ID.Hex(), user.Email, time.Now().Add(24*time.Hour).Unix())
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "could not generate token", err)
			return
		}

		utils.SuccessResponse(c, "Login successful", gin.H{
			"token": token,
			"user":  user,
		})

	}
}

// Register all routes here
func RegisterAPIRoutes(r *gin.Engine) {

	// Public routes (no authentication required)
	r.POST(objects.ApiBasePath+"login", Login())
	r.POST(objects.ApiBasePath+"signup", Signup())

	// Register authentication routes
	// RegisterAuthRoutes(r)

	// // Register chat routes
	// RegisterChatRoutes(r)

	// // Register WebSocket routes
	// RegisterWebSocketRoutes(r)

	// Register user management routes (with authentication)
	RegisterUserRoutes(r)

	// // Register admin routes
	// RegisterAdminRoutes(r)
}
