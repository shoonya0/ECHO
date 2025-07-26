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
		count, err := objects.DBClient.Database("Echo").Collection("users").CountDocuments(c.Request.Context(),
			bson.M{"email": req.Email},
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		if count > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}

		// hash password
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
			return
		}

		user := models.User{
			ID:           req.ID,
			Email:        req.Email,
			PasswordHash: string(hash),
			CreatedAt:    time.Now(),
		}

		if _, err := objects.DBClient.Database("Echo").Collection("users").InsertOne(c.Request.Context(), user); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"id":      req.ID.Hex(),
			"real_id": req.ID,
			"email":   user.Email,
		})
	}
}

// Login verifies credentials and returns a JWT
func Login() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if req.Email == "" || req.Password == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "email and password are required"})
			return
		}

		// find user by email
		var user models.User
		err := objects.DBClient.Database("Echo").Collection("users").FindOne(c.Request.Context(),
			bson.M{"email": req.Email},
		).Decode(&user)
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}

		// compare password
		if err := bcrypt.CompareHashAndPassword(
			[]byte(user.PasswordHash), []byte(req.Password),
		); err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}

		// generate JWT token
		token, err := utils.GetJWTToken(user.ID.Hex(), user.Email, time.Now().Add(24*time.Hour).Unix())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
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
