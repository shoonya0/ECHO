package controller

import (
	"gin/internal/services"
	"gin/internal/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

// List your contacts/friends.
// func GetContacts(c *gin.Context) {
// 	// Handler logic for getting contacts
// }

// Send a contact/friend request ({ targetUserId }).
func SendContactRequest(c *gin.Context) {
	userData, err := services.SendContactRequest(c)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}
	utils.SuccessResponse(c, "Contact request sent successfully", userData)
}

// Accept or decline a request ({ action: "accept"|"decline" }).
func AcceptOrDeclineContactRequest(c *gin.Context) {
	userData, err := services.AcceptOrDeclineContactRequest(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, userData)
}

// Remove a contact.
func RemoveContact(c *gin.Context) {
	userData, err := services.RemoveContact(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, userData)
}

// Block a user.
func BlockUser(c *gin.Context) {
	userData, err := services.BlockUser(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, userData)
}

// Unblock a user.
func UnblockUser(c *gin.Context) {
	userData, err := services.UnblockUser(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, userData)
}

// Add a user to your favorites.
func AddToFavorites(c *gin.Context) {
	userData, err := services.AddToFavorites(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, userData)
}

// Remove a user from your favorites.
func RemoveFromFavorites(c *gin.Context) {
	userData, err := services.RemoveFromFavorites(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, userData)
}

func GetFavoriteContacts(c *gin.Context) {
	userData, err := services.GetFavoriteContacts(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, userData)
}
