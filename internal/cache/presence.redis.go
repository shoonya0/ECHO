package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"gin/internal/models"
	"gin/objects"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// PresencePubSubManager manages Redis-based pub/sub for presence
type PresencePubSubManager struct {
	redisClient *redis.Client
	ctx         context.Context
	subscribers map[string]chan models.PresenceEvent
	mu          sync.RWMutex
	isRunning   bool
}

// NewPresencePubSubManager creates a new Redis-based presence pub/sub manager
func NewPresencePubSubManager() *PresencePubSubManager {
	manager := &PresencePubSubManager{
		redisClient: objects.RedisClient,
		ctx:         context.Background(),
		subscribers: make(map[string]chan models.PresenceEvent),
		isRunning:   false,
	}

	return manager
}

// Redis channel patterns and keys
const (
	PresenceChannelPattern   = "presence:*"        // presence:user_id
	GlobalPresenceChannel    = "presence:global"   // All presence updates
	CustomStatusChannel      = "presence:custom:*" // presence:custom:user_id
	PresenceDataKey          = "presence_data"     // Hash key for presence data
	PresenceHistoryKeyPrefix = "presence_history:" // prefix:user_id
	OnlineUsersKey           = "online_users"      // Set of online users
	UserSessionsKeyPrefix    = "user_sessions:"    // prefix:user_id
)

// StartPubSub starts the Redis pub/sub listener
func (p *PresencePubSubManager) StartPubSub() error {
	if p.isRunning {
		return fmt.Errorf("pub/sub manager is already running")
	}

	p.isRunning = true
	go p.startRedisSubscriber()
	log.Println("Presence pub/sub manager started")
	return nil
}

// StopPubSub stops the Redis pub/sub listener
func (p *PresencePubSubManager) StopPubSub() {
	p.isRunning = false
	p.mu.Lock()
	defer p.mu.Unlock()

	// Close all subscriber channels
	for subscriberID, ch := range p.subscribers {
		close(ch)
		delete(p.subscribers, subscriberID)
	}
	log.Println("Presence pub/sub manager stopped")
}

// Subscribe adds a subscriber to presence events
func (p *PresencePubSubManager) Subscribe(subscriberID string) <-chan models.PresenceEvent {
	p.mu.Lock()
	defer p.mu.Unlock()

	ch := make(chan models.PresenceEvent, 100) // Buffered channel
	p.subscribers[subscriberID] = ch
	return ch
}

// Unsubscribe removes a subscriber
func (p *PresencePubSubManager) Unsubscribe(subscriberID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if ch, exists := p.subscribers[subscriberID]; exists {
		close(ch)
		delete(p.subscribers, subscriberID)
	}
}

// UpdatePresence updates user presence and publishes event
func (p *PresencePubSubManager) UpdatePresence(userID string, status string, customStatus string, deviceInfo *string, location *string) error {
	now := time.Now()

	// Get existing presence or create new one
	presence, err := p.GetPresence(userID)
	if err != nil && err != redis.Nil {
		// Create new presence
		presence = models.PresenceStatus{
			UserID:    userID,
			CreatedAt: now,
		}
	}

	// Update presence fields
	isOnline := status != models.StatusInvisible && status != models.StatusOffline
	presence.IsOnline = isOnline
	presence.Status = status
	// presence.CustomStatus = customStatus
	presence.LastSeen = now
	// presence.LastActivity = now
	presence.UpdatedAt = now

	if deviceInfo != nil {
		presence.DeviceInfo = deviceInfo
	}
	if location != nil {
		presence.Location = location
	}

	// Store in Redis hash
	presenceJSON, err := json.Marshal(presence)
	if err != nil {
		return fmt.Errorf("failed to marshal presence: %w", err)
	}

	err = p.redisClient.HSet(p.ctx, PresenceDataKey, userID, presenceJSON).Err()
	if err != nil {
		return fmt.Errorf("failed to store presence: %w", err)
	}

	// Update online users set
	if isOnline {
		p.redisClient.SAdd(p.ctx, OnlineUsersKey, userID)
	} else {
		p.redisClient.SRem(p.ctx, OnlineUsersKey, userID)
	}

	// Add to history
	err = p.addToHistory(userID, presence)
	if err != nil {
		log.Printf("Failed to add presence to history: %v", err)
	}

	// Publish event
	event := models.PresenceEvent{
		Type:      models.EventPresenceUpdate,
		UserID:    userID,
		Presence:  presence,
		Timestamp: now,
		EventID:   uuid.New().String(),
	}

	return p.publishEvent(fmt.Sprintf("presence:%s", userID), event)
}

// SetCustomStatus sets or updates custom status
func (p *PresencePubSubManager) SetCustomStatus(userID, customStatus string) error {
	// Get existing presence
	presence, err := p.GetPresence(userID)
	if err != nil {
		if err == redis.Nil {
			// Create new presence if doesn't exist
			presence = models.PresenceStatus{
				UserID:    userID,
				IsOnline:  true,
				Status:    models.StatusOnline,
				LastSeen:  time.Now(),
				CreatedAt: time.Now(),
			}
		} else {
			return fmt.Errorf("failed to get existing presence: %w", err)
		}
	}

	// Update custom status
	// presence.CustomStatus = customStatus
	// presence.LastActivity = time.Now()
	presence.UpdatedAt = time.Now()

	// Store updated presence
	presenceJSON, err := json.Marshal(presence)
	if err != nil {
		return fmt.Errorf("failed to marshal presence: %w", err)
	}

	err = p.redisClient.HSet(p.ctx, PresenceDataKey, userID, presenceJSON).Err()
	if err != nil {
		return fmt.Errorf("failed to store presence: %w", err)
	}

	// Publish custom status update event
	event := models.PresenceEvent{
		Type:      models.EventCustomStatusUpdate,
		UserID:    userID,
		Presence:  presence,
		Timestamp: time.Now(),
		EventID:   uuid.New().String(),
	}

	return p.publishEvent(fmt.Sprintf("presence:custom:%s", userID), event)
}

// ClearCustomStatus clears user's custom status
func (p *PresencePubSubManager) ClearCustomStatus(userID string) error {
	return p.SetCustomStatus(userID, "")
}

// GetPresence gets user presence from Redis
func (p *PresencePubSubManager) GetPresence(userID string) (models.PresenceStatus, error) {
	data, err := p.redisClient.HGet(p.ctx, PresenceDataKey, userID).Result()
	if err != nil {
		return models.PresenceStatus{}, err
	}

	var presence models.PresenceStatus
	err = json.Unmarshal([]byte(data), &presence)
	if err != nil {
		return models.PresenceStatus{}, fmt.Errorf("failed to unmarshal presence: %w", err)
	}

	return presence, nil
}

// GetAllPresences gets all user presences
func (p *PresencePubSubManager) GetAllPresences() (map[string]models.PresenceStatus, error) {
	data, err := p.redisClient.HGetAll(p.ctx, PresenceDataKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get all presences: %w", err)
	}

	result := make(map[string]models.PresenceStatus)
	for userID, jsonData := range data {
		var presence models.PresenceStatus
		if json.Unmarshal([]byte(jsonData), &presence) == nil {
			// Apply visibility rules for invisible users
			if presence.Status == models.StatusInvisible {
				presence.IsOnline = false
				presence.Status = models.StatusOffline
			}
			result[userID] = presence
		}
	}
	return result, nil
}

// GetOnlineUsers gets list of currently online users
func (p *PresencePubSubManager) GetOnlineUsers() ([]string, error) {
	users, err := p.redisClient.SMembers(p.ctx, OnlineUsersKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get online users: %w", err)
	}
	return users, nil
}

// GetStatusHistory gets user's presence history
func (p *PresencePubSubManager) GetStatusHistory(userID string, limit int) ([]models.PresenceHistory, error) {
	if limit <= 0 {
		limit = 50 // Default limit
	}

	historyKey := PresenceHistoryKeyPrefix + userID
	data, err := p.redisClient.LRange(p.ctx, historyKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get status history: %w", err)
	}

	var history []models.PresenceHistory
	for _, jsonData := range data {
		var entry models.PresenceHistory
		if json.Unmarshal([]byte(jsonData), &entry) == nil {
			history = append(history, entry)
		}
	}

	return history, nil
}

// UserLogin handles user login event
func (p *PresencePubSubManager) UserLogin(userID string, deviceInfo *string) error {
	return p.UpdatePresence(userID, models.StatusOnline, "", deviceInfo, nil)
}

// UserLogout handles user logout event
func (p *PresencePubSubManager) UserLogout(userID string) error {
	now := time.Now()

	// Get current presence
	presence, err := p.GetPresence(userID)
	if err != nil {
		presence = models.PresenceStatus{
			UserID: userID,
		}
	}

	// Update to offline
	presence.IsOnline = false
	presence.Status = models.StatusOffline
	presence.LastSeen = now
	presence.UpdatedAt = now

	// Store updated presence
	presenceJSON, err := json.Marshal(presence)
	if err != nil {
		return fmt.Errorf("failed to marshal presence: %w", err)
	}

	err = p.redisClient.HSet(p.ctx, PresenceDataKey, userID, presenceJSON).Err()
	if err != nil {
		return fmt.Errorf("failed to store presence: %w", err)
	}

	// Remove from online users
	p.redisClient.SRem(p.ctx, OnlineUsersKey, userID)

	// Add to history
	p.addToHistory(userID, presence)

	// Publish logout event
	event := models.PresenceEvent{
		Type:      models.EventUserLogout,
		UserID:    userID,
		Presence:  presence,
		Timestamp: now,
		EventID:   uuid.New().String(),
	}

	return p.publishEvent(fmt.Sprintf("presence:%s", userID), event)
}

// UpdateActivity updates user's last activity timestamp
func (p *PresencePubSubManager) UpdateActivity(userID string) error {
	presence, err := p.GetPresence(userID)
	if err != nil {
		return err
	}

	// presence.LastActivity = time.Now()
	presence.UpdatedAt = time.Now()

	// Store updated presence
	presenceJSON, err := json.Marshal(presence)
	if err != nil {
		return fmt.Errorf("failed to marshal presence: %w", err)
	}

	return p.redisClient.HSet(p.ctx, PresenceDataKey, userID, presenceJSON).Err()
}

// publishEvent publishes an event to Redis
func (p *PresencePubSubManager) publishEvent(channel string, event models.PresenceEvent) error {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Publish to specific channel
	err = p.redisClient.Publish(p.ctx, channel, eventJSON).Err()
	if err != nil {
		return fmt.Errorf("failed to publish to specific channel: %w", err)
	}

	// Also publish to global channel
	err = p.redisClient.Publish(p.ctx, GlobalPresenceChannel, eventJSON).Err()
	if err != nil {
		return fmt.Errorf("failed to publish to global channel: %w", err)
	}

	// Notify local subscribers
	p.notifyLocalSubscribers(event)

	return nil
}

// startRedisSubscriber starts listening to Redis pub/sub
func (p *PresencePubSubManager) startRedisSubscriber() {
	// Subscribe to all presence channels
	pubsub := p.redisClient.PSubscribe(p.ctx, PresenceChannelPattern, CustomStatusChannel, GlobalPresenceChannel)
	defer pubsub.Close()

	log.Println("Started Redis presence subscriber")

	ch := pubsub.Channel()
	for msg := range ch {
		if !p.isRunning {
			break
		}

		var event models.PresenceEvent
		if json.Unmarshal([]byte(msg.Payload), &event) == nil {
			p.notifyLocalSubscribers(event)
		}
	}

	log.Println("Stopped Redis presence subscriber")
}

// notifyLocalSubscribers sends events to local subscribers
func (p *PresencePubSubManager) notifyLocalSubscribers(event models.PresenceEvent) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for subscriberID, ch := range p.subscribers {
		select {
		case ch <- event:
		default:
			// Channel is full, close and remove subscriber
			log.Printf("Subscriber %s channel is full, removing", subscriberID)
			close(ch)
			delete(p.subscribers, subscriberID)
		}
	}
}

// addToHistory adds presence change to history
func (p *PresencePubSubManager) addToHistory(userID string, presence models.PresenceStatus) error {
	historyEntry := models.PresenceHistory{
		UserID:    userID,
		Status:    presence.Status,
		Presence:  presence,
		Timestamp: time.Now(),
	}

	historyJSON, err := json.Marshal(historyEntry)
	if err != nil {
		return fmt.Errorf("failed to marshal history entry: %w", err)
	}

	historyKey := PresenceHistoryKeyPrefix + userID

	// Add to history list (newest first)
	err = p.redisClient.LPush(p.ctx, historyKey, historyJSON).Err()
	if err != nil {
		return fmt.Errorf("failed to add to history: %w", err)
	}

	// Keep only last 100 entries
	p.redisClient.LTrim(p.ctx, historyKey, 0, 99)

	// Set expiration (30 days)
	p.redisClient.Expire(p.ctx, historyKey, 30*24*time.Hour)

	return nil
}

// CleanupInactiveUsers removes inactive users from online set
func (p *PresencePubSubManager) CleanupInactiveUsers(inactiveThreshold time.Duration) error {
	onlineUsers, err := p.GetOnlineUsers()
	if err != nil {
		return err
	}

	now := time.Now()
	for _, userID := range onlineUsers {
		presence, err := p.GetPresence(userID)
		if err != nil {
			continue
		}

		// If user has been inactive for too long, mark as away
		if now.Sub(presence.LastSeen) > inactiveThreshold {
			if presence.Status == models.StatusOnline {
				p.UpdatePresence(userID, models.StatusAway, "", presence.DeviceInfo, presence.Location)
			}
		}
	}

	return nil
}

// Global presence manager instance
var PresenceManager *PresencePubSubManager

// InitializePresenceManager initializes the global presence manager
func InitializePresenceManager() error {
	PresenceManager = NewPresencePubSubManager()
	return PresenceManager.StartPubSub()
}
