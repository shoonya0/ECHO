package models

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ WEBSOCKET MODELS FOR REAL-TIME CHAT ============

// WebSocketMessage represents different types of real-time messages
type WebSocketMessage struct {
	Type      string      `json:"type"`                // "message", "typing", "status", "join", "leave", "error"
	ChatID    string      `json:"chatId,omitempty"`    // Optional chat identifier
	UserID    string      `json:"userId,omitempty"`    // Sender user ID
	Username  string      `json:"username,omitempty"`  // Sender username
	Avatar    string      `json:"avatar,omitempty"`    // Sender avatar URL
	Data      interface{} `json:"data,omitempty"`      // Message payload
	Timestamp time.Time   `json:"timestamp"`           // Message timestamp
	RequestID string      `json:"requestId,omitempty"` // For request-response correlation
}

// ChatMessage represents a real-time chat message
type ChatMessage struct {
	ID          string                 `json:"id"`
	ChatID      string                 `json:"chatId"`
	SenderID    string                 `json:"senderId"`
	Content     string                 `json:"content"`
	MessageType string                 `json:"messageType"` // "text", "image", "file", "audio", "video"
	Attachments []AttachmentEmbed      `json:"attachments,omitempty"`
	Mentions    []string               `json:"mentions,omitempty"`
	ReplyTo     *string                `json:"replyTo,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
}

// TypingIndicator represents typing status
type TypingIndicator struct {
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	IsTyping  bool      `json:"isTyping"`
	Timestamp time.Time `json:"timestamp"`
}

// WSPresenceStatus represents user online status for WebSocket messages
type WSPresenceStatus struct {
	UserID     string    `json:"userId"`
	Status     string    `json:"status"` // "online", "away", "busy", "offline"
	LastSeen   time.Time `json:"lastSeen"`
	CustomText string    `json:"customText,omitempty"`
}

// UserJoinLeave represents user joining/leaving events
type UserJoinLeave struct {
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar,omitempty"`
	Action    string    `json:"action"` // "join", "leave"
	Timestamp time.Time `json:"timestamp"`
}

// MessageDelivery represents message delivery status
type MessageDelivery struct {
	MessageID string    `json:"messageId"`
	UserID    string    `json:"userId"`
	Status    string    `json:"status"` // "sent", "delivered", "read"
	Timestamp time.Time `json:"timestamp"`
}

// ErrorMessage represents error responses
type ErrorMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// MessageReaction represents message reaction events
type MessageReaction struct {
	MessageID string    `json:"messageId"`
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	Emoji     string    `json:"emoji"`
	Action    string    `json:"action"` // "add", "remove"
	Timestamp time.Time `json:"timestamp"`
}

// MessageEdit represents message edit events
type MessageEdit struct {
	MessageID   string    `json:"messageId"`
	UserID      string    `json:"userId"`
	Username    string    `json:"username"`
	NewContent  string    `json:"newContent"`
	EditedAt    time.Time `json:"editedAt"`
	EditHistory bool      `json:"editHistory"` // Whether to show edit history
}

// MessageDelete represents message deletion events
type MessageDelete struct {
	MessageID string    `json:"messageId"`
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	DeletedAt time.Time `json:"deletedAt"`
	Reason    string    `json:"reason,omitempty"`
}

// ChatUpdate represents chat information updates
type ChatUpdate struct {
	ChatID     string                 `json:"chatId"`
	UpdatedBy  string                 `json:"updatedBy"`
	Changes    map[string]interface{} `json:"changes"`
	Timestamp  time.Time              `json:"timestamp"`
	UpdateType string                 `json:"updateType"` // "name", "description", "avatar", "settings"
}

// UserInvite represents user invitation events
type UserInvite struct {
	ChatID      string    `json:"chatId"`
	InviterID   string    `json:"inviterId"`
	InviterName string    `json:"inviterName"`
	InviteeID   string    `json:"inviteeId"`
	InviteeName string    `json:"inviteeName"`
	Timestamp   time.Time `json:"timestamp"`
}

// UserRemove represents user removal events
type UserRemove struct {
	ChatID      string    `json:"chatId"`
	RemovedByID string    `json:"removedById"`
	RemovedBy   string    `json:"removedBy"`
	RemovedID   string    `json:"removedId"`
	RemovedUser string    `json:"removedUser"`
	Reason      string    `json:"reason,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// ChatNotification represents various chat notifications
type ChatNotification struct {
	ChatID    string                 `json:"chatId"`
	Type      string                 `json:"type"` // "mention", "invite", "role_change", "system"
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Priority  string                 `json:"priority"` // "low", "normal", "high", "urgent"
	Timestamp time.Time              `json:"timestamp"`
}

// ============ CLIENT CONNECTION MODELS ============

// Client represents a WebSocket client connection (simplified - no redundant user data)
type Client struct {
	ID           string                 `json:"id"`
	UserID       bson.ObjectID          `json:"userId"`
	Connection   *websocket.Conn        `json:"-"`
	Send         chan WebSocketMessage  `json:"-"`
	ActiveChats  []string               `json:"activeChats"` // Chat IDs the client is subscribed to
	LastActivity time.Time              `json:"lastActivity"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// UserDisplayInfo represents cached user display information for UI
type UserDisplayInfo struct {
	UserID      bson.ObjectID `json:"userId"`
	Username    string        `json:"username"`
	DisplayName string        `json:"displayName"`
	Avatar      string        `json:"avatar"`
	Status      string        `json:"status"`
	IsOnline    bool          `json:"isOnline"`
	LastSeen    time.Time     `json:"lastSeen"`
}

// Hub represents the WebSocket hub managing all connections (simplified)
type Hub struct {
	// Registered clients
	Clients map[string]*Client `json:"-"`

	// Chat to clients mapping (replaces ChatRooms)
	// group chat
	ChatClients map[string]map[string]*Client `json:"-"` // ChatID -> ClientID -> Client

	// User to client mapping
	// direct chat
	UserClients map[string]map[string]*Client `json:"-"` // UserID -> ClientID -> Client

	// User display info cache (TTL: 5 minutes)
	UserInfoCache map[string]*UserDisplayInfo `json:"-"`
	CacheExpiry   map[string]time.Time        `json:"-"`

	// Mutex for thread safety
	Mutex sync.RWMutex `json:"-"`
}

// HubMessage represents a message to be broadcasted
type HubMessage struct {
	ChatID  string           `json:"chatId"`
	Message WebSocketMessage `json:"message"`
	Exclude map[string]bool  `json:"exclude,omitempty"` // Client IDs to exclude from broadcast
}

// JoinChatRequest represents a request to join a chat
type JoinChatRequest struct {
	Client *Client `json:"-"`
	ChatID string  `json:"chatId"`
}

// LeaveChatRequest represents a request to leave a chat
type LeaveChatRequest struct {
	Client *Client `json:"-"`
	ChatID string  `json:"chatId"`
}

// MessageRequest represents incoming message requests
type MessageRequest struct {
	// here type represents the type of the message eg: send_message, join_chat, leave_chat, set_typing, set_presence, mark_read, edit_message, delete_message, add_reaction, remove_reaction, invite_user, remove_user, update_chat
	Type        string                 `json:"type"`
	ChatID      string                 `json:"chatId"`
	SenderID    string                 `json:"senderId" bson:"senderId"`
	Content     string                 `json:"content,omitempty"`
	MessageType string                 `json:"messageType,omitempty"`
	Attachments []AttachmentEmbed      `json:"attachments,omitempty"`
	Mentions    []string               `json:"mentions,omitempty"`
	ReplyTo     *string                `json:"replyTo,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	RequestID   string                 `json:"requestId,omitempty"`
}

// MessageResponse represents outgoing message responses
type MessageResponse struct {
	Success   bool                   `json:"success"`
	MessageID string                 `json:"messageId,omitempty"`
	Error     *ErrorMessage          `json:"error,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	RequestID string                 `json:"requestId,omitempty"`
}

// ============ WEBSOCKET EVENTS ============

const (
	// Message types
	WSMessageTypeChat          = "message"
	WSMessageTypeTyping        = "typing"
	WSMessageTypePresence      = "presence"
	WSMessageTypeJoin          = "join"
	WSMessageTypeLeave         = "leave"
	WSMessageTypeDelivery      = "delivery"
	WSMessageTypeError         = "error"
	WSMessageTypeResponse      = "response"
	WSMessageTypeUserUpdated   = "user_updated"
	WSMessageTypeChatUpdated   = "chat_updated"
	WSMessageTypeMessageEdit   = "message_edited"
	WSMessageTypeMessageDelete = "message_deleted"
	WSMessageTypeReaction      = "reaction"
	WSMessageTypeNotification  = "notification"

	// Request types (updated from "room" to "chat")
	WSRequestTypeSendMessage    = "send_message"
	WSRequestTypeJoinChat       = "join_chat"
	WSRequestTypeLeaveChat      = "leave_chat"
	WSRequestTypeSetTyping      = "set_typing"
	WSRequestTypeSetPresence    = "set_presence"
	WSRequestTypeMarkRead       = "mark_read"
	WSRequestTypeEditMessage    = "edit_message"
	WSRequestTypeDeleteMessage  = "delete_message"
	WSRequestTypeAddReaction    = "add_reaction"
	WSRequestTypeRemoveReaction = "remove_reaction"
	WSRequestTypeInviteUser     = "invite_user"
	WSRequestTypeRemoveUser     = "remove_user"
	WSRequestTypeUpdateChat     = "update_chat"
)
