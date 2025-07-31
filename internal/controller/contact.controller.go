package controller

// Contact List Management

// func GetRecentUsers(ctx *gin.Context) {
// 	userID, ok := ctx.Get("userId")
// 	if !ok {
// 		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user id not found"})
// 		return
// 	}
// 	userData, err := services.GetContacts(userID.(string), objects.RecentContacts, objects.StatusAccepted)
// 	if err != nil {
// 		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}
// 	ctx.JSON(http.StatusOK, userData)
// }

// // ============ CONTACTS & FRIENDS MANAGEMENT ============
// func GetContacts(ctx *gin.Context) {
// 	userID, ok := ctx.Get("userId")
// 	if !ok {
// 		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user id not found"})
// 		return
// 	}
// 	userData, err := services.GetContacts(userID.(string), objects.AllContacts, objects.StatusAccepted)
// 	if err != nil {
// 		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}
// 	ctx.JSON(http.StatusOK, userData)
// }

// func GetContactRequests(ctx *gin.Context) {
// 	userID, ok := ctx.Get("userId")
// 	if !ok {
// 		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user id not found"})
// 		return
// 	}
// 	userData, err := services.GetContacts(userID.(string), objects.RecentContacts, objects.StatusPending)
// 	if err != nil {
// 		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}
// 	ctx.JSON(http.StatusOK, userData)
// }

// func GetSentContactRequests(ctx *gin.Context) {
// 	userID, ok := ctx.Get("userId")
// 	if !ok {
// 		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user id not found"})
// 		return
// 	}
// 	userData, err := services.GetSentContactRequests(userID.(string))
// 	if err != nil {
// 		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to get sent contact requests", err.Error())
// 		return
// 	}
// 	utils.SuccessResponse(ctx, "Sent contact requests fetched successfully", userData)
// }

// func GetBlockedUsers(ctx *gin.Context) {
// 	userID, ok := ctx.Get("userId")
// 	if !ok {
// 		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user id not found"})
// 		return
// 	}
// 	userData, err := services.GetContacts(userID.(string), objects.AllContacts, objects.StatusBlocked)
// 	if err != nil {
// 		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}
// 	ctx.JSON(http.StatusOK, userData)
// }

// // Contact Actions
// func SendContactRequest(ctx *gin.Context) {
// 	userID, ok := ctx.Get("userId")
// 	if !ok {
// 		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "user id not found"})
// 		return
// 	}
// 	userData, err := services.SendContactRequest(userID.(string), ctx.Param("targetUserId"))
// 	if err != nil {
// 		utils.ErrorResponse(ctx, http.StatusInternalServerError, "Failed to send contact request", err.Error())
// 		return
// 	}
// 	utils.SuccessResponse(ctx, "Contact request sent successfully", userData)
// }

// // // Accept or decline a request ({ action: "accept"|"decline" }).
// // func AcceptOrDeclineContactRequest(c *gin.Context) {
// // 	userData, err := services.AcceptOrDeclineContactRequest(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }

// // // Remove a contact.
// // func RemoveContact(c *gin.Context) {
// // 	userData, err := services.RemoveContact(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }

// // // Block a user.
// // func BlockUser(c *gin.Context) {
// // 	userData, err := services.BlockUser(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }

// // // Unblock a user.
// // func UnblockUser(c *gin.Context) {
// // 	userData, err := services.UnblockUser(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }

// // // Add a user to your favorites.
// // func AddToFavorites(c *gin.Context) {
// // 	userData, err := services.AddToFavorites(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }

// // // Remove a user from your favorites.
// // func RemoveFromFavorites(c *gin.Context) {
// // 	userData, err := services.RemoveFromFavorites(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }

// // func GetFavoriteContacts(c *gin.Context) {
// // 	userData, err := services.GetFavoriteContacts(c)
// // 	if err != nil {
// // 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// // 		return
// // 	}
// // 	c.JSON(http.StatusOK, userData)
// // }
