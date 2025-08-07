package models

import (
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

// ============ CLIENT CONNECTION MODELS ============

// Client represents a WebSocket client connection
type Client struct {
	ID           string                 `json:"id"`
	UserID       bson.ObjectID          `json:"userId"`
	Username     string                 `json:"username"`
	Connection   *websocket.Conn        `json:"-"`
	Send         chan WebSocketMessage  `json:"-"`
	ChatRooms    map[string]bool        `json:"chatRooms"` // Set of chat room IDs the client is subscribed to
	LastActivity time.Time              `json:"lastActivity"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ChatRoom represents a chat room with connected clients
type ChatRoom struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Type         string             `json:"type"` // "direct", "group", "channel"
	Clients      map[string]*Client `json:"-"`    // Map of client IDs to clients
	CreatedAt    time.Time          `json:"createdAt"`
	LastActivity time.Time          `json:"lastActivity"`
	Settings     ChatRoomSettings   `json:"settings"`
}

// ChatRoomSettings represents room-specific settings
type ChatRoomSettings struct {
	MaxClients      int  `json:"maxClients"`
	AllowAnonymous  bool `json:"allowAnonymous"`
	MessageHistory  bool `json:"messageHistory"`
	TypingIndicator bool `json:"typingIndicator"`
	ReadReceipts    bool `json:"readReceipts"`
}

// Hub represents the WebSocket hub managing all connections
type Hub struct {
	// Registered clients
	Clients map[string]*Client `json:"-"`

	// Chat rooms
	ChatRooms map[string]*ChatRoom `json:"-"`

	// User to client mapping
	UserClients map[string]map[string]*Client `json:"-"` // UserID -> ClientID -> Client

	// Channel for client registration
	Register chan *Client `json:"-"`

	// Channel for client unregistration
	Unregister chan *Client `json:"-"`

	// Channel for broadcasting messages
	Broadcast chan HubMessage `json:"-"`

	// Channel for joining chat rooms
	JoinRoom chan JoinRoomRequest `json:"-"`

	// Channel for leaving chat rooms
	LeaveRoom chan LeaveRoomRequest `json:"-"`
}

// HubMessage represents a message to be broadcasted
type HubMessage struct {
	ChatID  string           `json:"chatId"`
	Message WebSocketMessage `json:"message"`
	Exclude map[string]bool  `json:"exclude,omitempty"` // Client IDs to exclude from broadcast
}

// JoinRoomRequest represents a request to join a chat room
type JoinRoomRequest struct {
	Client *Client `json:"-"`
	ChatID string  `json:"chatId"`
}

// LeaveRoomRequest represents a request to leave a chat room
type LeaveRoomRequest struct {
	Client *Client `json:"-"`
	ChatID string  `json:"chatId"`
}

// MessageRequest represents incoming message requests
type MessageRequest struct {
	Type        string                 `json:"type"`
	ChatID      string                 `json:"chatId"`
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
	WSMessageTypeChat     = "message"
	WSMessageTypeTyping   = "typing"
	WSMessageTypePresence = "presence"
	WSMessageTypeJoin     = "join"
	WSMessageTypeLeave    = "leave"
	WSMessageTypeDelivery = "delivery"
	WSMessageTypeError    = "error"
	WSMessageTypeResponse = "response"

	// Request types
	WSRequestTypeSendMessage = "send_message"
	WSRequestTypeJoinRoom    = "join_room"
	WSRequestTypeLeaveRoom   = "leave_room"
	WSRequestTypeSetTyping   = "set_typing"
	WSRequestTypeSetPresence = "set_presence"
	WSRequestTypeMarkRead    = "mark_read"
)
