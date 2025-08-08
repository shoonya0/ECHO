package services

import (
	"fmt"
	"gin/internal/models"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ CHAT HELPER FUNCTIONS FOR CONSOLIDATED MODELS ============

// ExpandChatParticipants converts ParticipantRef to ParticipantEmbed for API responses
func ExpandChatParticipants(chat *models.Chat) (*models.ChatDetailResponse, error) {
	if chat == nil {
		return nil, fmt.Errorf("chat is nil")
	}

	// Build participant embeds from participant refs
	participantEmbeds, err := BuildMultipleParticipantEmbeds(chat.Participants)
	if err != nil {
		return nil, fmt.Errorf("failed to build participant embeds: %w", err)
	}

	// Convert map to slice for response
	participants := make([]models.ParticipantDetail, 0, len(participantEmbeds))
	for _, embed := range participantEmbeds {
		participants = append(participants, models.ParticipantDetail{
			ParticipantEmbed: embed,
			IsContact:        false, // TODO: Check if user is in contacts
			IsBlocked:        embed.IsBlocked,
		})
	}

	// Create detailed response
	response := &models.ChatDetailResponse{
		Chat: models.Chat{
			ChatID:        chat.ChatID,
			Type:          chat.Type,
			Name:          chat.Name,
			Description:   chat.Description,
			Avatar:        chat.Avatar,
			Participants:  chat.Participants, // Keep original refs for internal use
			OwnerID:       chat.OwnerID,
			AdminIDs:      chat.AdminIDs,
			LastMessageID: chat.LastMessageID,
			Stats:         chat.Stats,
			Settings:      chat.Settings,
			ReadReceipts:  chat.ReadReceipts,
			TypingUsers:   chat.TypingUsers,
			IsActive:      chat.IsActive,
			LastActivity:  chat.LastActivity,
			CreatedAt:     chat.CreatedAt,
			UpdatedAt:     chat.UpdatedAt,
		},
		Messages:     []models.Message{}, // Will be filled separately
		Participants: participants,
		CanLoadMore:  false, // Will be determined by message pagination
	}

	return response, nil
}

// GetChatParticipantUserIDs extracts user IDs from chat participants
func GetChatParticipantUserIDs(chat *models.Chat) []bson.ObjectID {
	userIDs := make([]bson.ObjectID, 0, len(chat.Participants))
	for _, participant := range chat.Participants {
		userIDs = append(userIDs, participant.UserID)
	}
	return userIDs
}

// CheckUserChatPermission checks if a user can access a chat
func CheckUserChatPermission(chat *models.Chat, userID bson.ObjectID) bool {
	if chat == nil {
		return false
	}

	participant, exists := chat.Participants[userID.Hex()]
	if !exists {
		return false
	}

	// Check if user is blocked
	if participant.IsBlocked {
		return false
	}

	return true
}

// GetChatDisplayName returns the display name for a chat based on its type and user context
func GetChatDisplayName(chat *models.Chat, currentUserID bson.ObjectID) (string, error) {
	if chat.Type == "direct" {
		// For direct chats, find the other user and return their display name
		for userIDStr, participant := range chat.Participants {
			if participant.UserID != currentUserID {
				userInfo, err := GetUserDisplayInfo(participant.UserID)
				if err != nil {
					return fmt.Sprintf("User %s", userIDStr[:8]), nil // Fallback
				}
				return userInfo.DisplayName, nil
			}
		}
		return "Direct Chat", nil
	}

	// For group chats, use the chat name or generate one
	if chat.Name != "" {
		return chat.Name, nil
	}

	// Generate name from participants for unnamed group chats
	participantNames := make([]string, 0, min(3, len(chat.Participants)))
	count := 0
	for _, participant := range chat.Participants {
		if participant.UserID == currentUserID {
			continue // Skip current user
		}
		if count >= 2 {
			break // Limit to 2 other participants for name
		}

		userInfo, err := GetUserDisplayInfo(participant.UserID)
		if err != nil {
			continue
		}
		participantNames = append(participantNames, userInfo.DisplayName)
		count++
	}

	if len(participantNames) == 0 {
		return "Group Chat", nil
	}

	if len(participantNames) == 1 {
		return participantNames[0], nil
	}

	if len(chat.Participants) > 3 {
		return fmt.Sprintf("%s, %s and %d others",
			participantNames[0], participantNames[1], len(chat.Participants)-3), nil
	}

	return fmt.Sprintf("%s and %s", participantNames[0], participantNames[1]), nil
}

// UpdateChatActivity updates the chat's last activity timestamp
func UpdateChatActivity(chatID bson.ObjectID) error {
	// This will be called when messages are sent, users join/leave, etc.
	// TODO: Implement database update for last activity
	return nil
}

// GetActiveChatClients returns the count of active clients in a chat
func GetActiveChatClients(chatID string) int {
	clients, exists := GetChatClients(chatID)
	if !exists {
		return 0
	}
	return len(clients)
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
