package models

type UserDetails struct {
	UserId        string `json:"user_id" bson:"user_id"`
	FirstName     string `json:"first_name" bson:"first_name"`
	LastName      string `json:"last_name" bson:"last_name"`
	Email         string `json:"email" bson:"email"`
	Phone         string `json:"phone" bson:"phone"`
	Avatar        string `json:"avatar" bson:"avatar"`
	PasswwardHash string `json:"-" bson:"password_hash"`
	IsActive      bool   `json:"is_active" bson:"is_active"`
	IsVerified    bool   `json:"is_verified" bson:"is_verified"`
	CreatedAt     string `json:"created_at" bson:"created_at"`
}

type UserRegister struct {
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name" binding:"required"`
	Email     string `json:"email" binding:"required"`
	Password  string `json:"password" binding:"required"`
}

type UserLogin struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}
