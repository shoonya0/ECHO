package controller

import "github.com/gin-gonic/gin"

// List your contacts/friends.
// func GetContacts(c *gin.Context) {
// 	// Handler logic for getting contacts
// }

// Send a contact/friend request ({ targetUserId }).
func SendContactRequest(c *gin.Context) {
	// Handler logic for sending contact request
}

// Accept or decline a request ({ action: "accept"|"decline" }).
func AcceptOrDeclineContactRequest(c *gin.Context) {
	// Handler logic for accepting or declining contact request
}

// Remove a contact.
func RemoveContact(c *gin.Context) {
	// Handler logic for removing contact
}

// Get your own presence (online/away/do-not-disturb).
func GetPresence(c *gin.Context) {
	// Handler logic for getting presence
}

// Update your presence/status.
func UpdatePresence(c *gin.Context) {
	// Handler logic for updating presence
}
