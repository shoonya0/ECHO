package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"gin/internal/models"
	"io"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// NewUserWithDefaults creates a new User with all embedded structures properly initialized
func NewUserWithDefaults(id bson.ObjectID, email, username, passwordHash string) models.User {
	now := time.Now()

	return models.User{
		ID:           id,
		Email:        email,
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,

		// Initialize Profile with defaults
		Profile: models.UserProfileEmbed{
			DisplayName:   username, // Use email as default display name
			Avatar:        "",
			StatusMessage: "",
			Bio:           "",
		},

		// Initialize Presence with defaults
		Presence: models.PresenceEmbed{
			Status:       "offline",
			IsOnline:     false,
			LastSeen:     now,
			LastActivity: now,
			DeviceInfo:   "",
			Location:     "",
		},

		// Initialize ContactInfo with empty arrays
		ContactInfo: models.ContactInfoEmbed{
			BlockedChats: make([]bson.ObjectID, 0),
			PendingOut:   make([]bson.ObjectID, 0),
			PendingIn:    make([]bson.ObjectID, 0),
			Favorites:    make([]bson.ObjectID, 0),
			Contacts:     make([]bson.ObjectID, 0),
			CreatedAt:    now,
			UpdatedAt:    now,
		},

		// Initialize AccountStatus with defaults
		AccountStatus: models.AccountStatusEmbed{
			IsActive:   true,
			IsVerified: false,
			IsBanned:   false,
		},

		// Initialize Settings with defaults
		Settings: models.UserSettingsEmbed{
			Theme:    "system",
			Language: "en",
			SoundsOn: true,
			Notifications: models.NotificationPrefsEmbed{
				PushEnabled:    true,
				EmailEnabled:   true,
				SoundEnabled:   true,
				MentionsOnly:   false,
				MessagePreview: true,
			},
			Privacy: models.PrivacySettingsEmbed{
				ShowOnlineStatus: "everyone",
				ShowLastSeen:     false,
				AllowContactBy:   "everyone",
			},
			MessagePreferences: models.MessagePrefsEmbed{
				AutoDownloadImages:   true,
				AutoDownloadFiles:    false,
				ShowEmojiSuggestions: true,
			},
		},
		Chats: make(map[bson.ObjectID]int),
	}
}

func NewChatWithDefaults(id bson.ObjectID, chatType string) models.Chat {
	now := time.Now()

	return models.Chat{
		ChatID:        bson.NewObjectIDFromTimestamp(now),
		ChatType:      chatType,
		Name:          "",
		Description:   "",
		Avatar:        "",
		Participants:  make(map[bson.ObjectID]models.ParticipantEmbed),
		OwnerID:       id,
		AdminIDs:      []bson.ObjectID{},
		LastMessageID: bson.ObjectID{},
		Stats: models.ChatStatsEmbed{
			ParticipantCount: 0,
			MessageCount:     0,
			UnreadCount:      make(map[bson.ObjectID]int),
			ImageCount:       0,
			FileCount:        0,
		},
		Settings: models.ChatSettingsEmbed{
			IsPrivate:        true,
			AllowInvites:     true,
			AllowFileSharing: true,
			MessageRetention: 0,
			MaxParticipants:  0,
		},
		ReadReceipts:  make(map[bson.ObjectID]time.Time),
		TypingUsers:   make(map[bson.ObjectID]time.Time),
		ActiveClients: make(map[string]*models.Client),
		LastActivity:  now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func NewParticipantWithDefaults(requestStatus string, onlineStatus string, requestedBy bson.ObjectID, permissions []string, userInfo models.ContactUserInfo) models.ParticipantEmbed {
	return models.ParticipantEmbed{
		RequestStatus: requestStatus,
		OnlineStatus:  onlineStatus,
		RequestedBy:   requestedBy,
		Permissions:   permissions,
		IsBlocked:     false,
		UserInfo:      userInfo,
		LastSeen:      time.Now(),
		IsMuted:       false,
		Role:          "member",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		JoinedAt:      time.Now(),
		LeftAt:        time.Now(),
	}
}

const secretKey = "thisIsASecureAndLongKeyForAES256"

// Encrypt a byte slice using AES-256 GCM.
func Encrypt(data []byte) ([]byte, error) {
	key := []byte(secretKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// Decrypt a byte slice using AES-256 GCM.
func Decrypt(data []byte) (string, error) {
	key := []byte(secretKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func NewInviteCodeWithDefaults(user models.LoginUserResponse, chatID bson.ObjectID, expireTime time.Time) (models.InviteCodeEmbed, bool) {

	dataString := fmt.Sprintf("%s_%s_%s_%s", chatID.Hex(), user.ID.Hex(), user.Profile.DisplayName, expireTime.Format(time.RFC3339))

	encryptedData, err := Encrypt([]byte(dataString))
	if err != nil {
		return models.InviteCodeEmbed{}, false
	}

	inviteCode := hex.EncodeToString(encryptedData)

	status := "active"
	deleted := false

	invite := models.InviteCodeEmbed{
		InviteCode:  inviteCode,
		UserIDs:     []bson.ObjectID{},
		ExpiredDate: expireTime,
		Status:      status,
		Deleted:     deleted,
	}

	return invite, true
}
