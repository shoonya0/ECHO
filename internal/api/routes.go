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
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

type SignupRequest struct {
	ID       bson.ObjectID `json:"_id" bson:"_id"`
	Email    string        `json:"email" binding:"required,email"`
	Password string        `json:"password" binding:"required,min=8"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func Signup() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SignupRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		count, err := objects.DB.Collection(string(objects.UserColl)).CountDocuments(c.Request.Context(), bson.M{"email": req.Email})
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to check email", err)
			return
		}
		if count > 0 {
			utils.ErrorResponse(c, http.StatusConflict, "email already registered", nil)
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "could not hash password", err)
			return
		}

		user := utils.NewUserWithDefaults(req.ID, req.Email, string(hash))

		if _, err := objects.DB.Collection(string(objects.UserColl)).InsertOne(c.Request.Context(), user); err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to create user", err)
			return
		}

		utils.SuccessResponse(c, "User created successfully", user)
	}
}

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

		userProjection := bson.M{
			"_id":           1,
			"email":         1,
			"hash":          1,
			"profile":       1,
			"accountStatus": 1,
		}

		var user models.LoginUserResponse
		err := objects.DB.Collection(string(objects.UserColl)).FindOne(c.Request.Context(), bson.M{"email": req.Email}, options.FindOne().SetProjection(userProjection)).Decode(&user)
		if err == mongo.ErrNoDocuments {
			utils.ErrorResponse(c, http.StatusUnauthorized, "invalid credentials", err)
			return
		} else if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to find user", err)
			return
		}

		if err := bcrypt.CompareHashAndPassword(
			[]byte(user.PasswordHash), []byte(req.Password),
		); err != nil {
			utils.ErrorResponse(c, http.StatusUnauthorized, "invalid credentials", err)
			return
		}

		token, err := utils.GetJWTToken(user.ID.Hex(), user.Email, user.Username, user.Profile, user.AccountStatus, time.Now().Add(7*24*time.Hour).Unix())
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

func RegisterAPIRoutes(r *gin.Engine) {
	r.POST(objects.ApiBasePath+"login", Login())
	r.POST(objects.ApiBasePath+"signup", Signup())

	RegisterUserRoutes(r)

	RegisterChatRoutes(r)
}

// in contacts , delete the CHat ID
// only create the chat when any user send a message to the other user
// if any things is deleted don't marked it's deleted , just remove the chat from the user's contacts
