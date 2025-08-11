package utils

import (
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ============ DEMO FUNCTIONS FOR PRETTY PRINT SYSTEM ============

// DemoStruct represents a complex nested structure for testing
type DemoStruct struct {
	ID           bson.ObjectID          `json:"id"`
	Name         string                 `json:"name"`
	Age          int                    `json:"age"`
	IsActive     bool                   `json:"isActive"`
	Score        float64                `json:"score"`
	Tags         []string               `json:"tags"`
	Metadata     map[string]interface{} `json:"metadata"`
	CreatedAt    time.Time              `json:"createdAt"`
	UpdatedAt    *time.Time             `json:"updatedAt,omitempty"`
	NestedData   *NestedStruct          `json:"nestedData,omitempty"`
	Numbers      []int                  `json:"numbers"`
	Settings     map[string]bool        `json:"settings"`
	privateField string                 // This won't be printed by default
}

type NestedStruct struct {
	Level       int                    `json:"level"`
	Description string                 `json:"description"`
	Attributes  map[string]interface{} `json:"attributes"`
	SubNested   *SubNestedStruct       `json:"subNested,omitempty"`
}

type SubNestedStruct struct {
	Value   string `json:"value"`
	Count   int    `json:"count"`
	Enabled bool   `json:"enabled"`
}

// CreateDemoData creates sample data for testing the print system
func CreateDemoData() *DemoStruct {
	now := time.Now()
	future := now.Add(24 * time.Hour)

	return &DemoStruct{
		ID:       bson.NewObjectID(),
		Name:     "John Doe",
		Age:      30,
		IsActive: true,
		Score:    95.7,
		Tags:     []string{"developer", "golang", "mongodb", "websocket"},
		Metadata: map[string]interface{}{
			"department": "Engineering",
			"level":      "Senior",
			"skills":     []string{"Go", "MongoDB", "WebSocket", "gRPC"},
			"experience": map[string]int{
				"Go":        5,
				"MongoDB":   3,
				"WebSocket": 2,
				"gRPC":      4,
			},
			"certifications": nil,
		},
		CreatedAt: now,
		UpdatedAt: &future,
		NestedData: &NestedStruct{
			Level:       2,
			Description: "This is a nested structure example",
			Attributes: map[string]interface{}{
				"color":      "blue",
				"size":       "large",
				"priority":   1,
				"features":   []string{"feature1", "feature2"},
				"deprecated": false,
			},
			SubNested: &SubNestedStruct{
				Value:   "nested value",
				Count:   42,
				Enabled: true,
			},
		},
		Numbers: []int{1, 2, 3, 4, 5, 10, 20, 30},
		Settings: map[string]bool{
			"notifications": true,
			"darkMode":      false,
			"autoSave":      true,
		},
		privateField: "this is private",
	}
}

// RunPrintDemos demonstrates all the print functions
func RunPrintDemos() {
	data := CreateDemoData()

	// Basic pretty print
	PrintWithTitle("Basic Pretty Print", data)

	// With colors (if terminal supports it)
	PrintWithTitle("Colored Output", "Using PrintColored:")
	PrintColored(data)

	// With type information
	PrintWithTitle("With Type Information", "Using PrintWithTypes:")
	PrintWithTypes(data)

	// Compact format
	PrintWithTitle("Compact Format", "Using PrintCompact:")
	PrintCompact(data)

	// JSON format
	PrintWithTitle("JSON Format", "Using PrettyPrintJSON:")
	PrettyPrintJSON(data)

	// Test different data types
	PrintWithTitle("Various Data Types", map[string]interface{}{
		"string":   "Hello World",
		"integer":  42,
		"float":    3.14159,
		"boolean":  true,
		"null":     nil,
		"array":    []int{1, 2, 3, 4, 5},
		"map":      map[string]string{"key1": "value1", "key2": "value2"},
		"time":     time.Now(),
		"objectId": bson.NewObjectID(),
	})

	// Test circular reference handling
	type CircularDemo struct {
		Name string
		Self *CircularDemo
	}

	circular := &CircularDemo{Name: "test"}
	circular.Self = circular // Create circular reference

	PrintWithTitle("Circular Reference Handling", circular)

	// Test with custom options
	PrintWithTitle("Custom Options", "Custom depth and formatting:")
	customOpts := &PrintOptions{
		Indent:            "    ", // 4 spaces
		MaxDepth:          3,      // Limit depth to 3
		ShowTypes:         true,   // Show type info
		CompactArrays:     true,   // Compact small arrays
		ColorOutput:       false,  // No colors for this demo
		TimeFormat:        "2006-01-02 15:04:05",
		MaxStringLength:   30,    // Truncate long strings
		ShowPrivateFields: false, // Hide private fields
	}
	PrettyPrintWithOptions(data, customOpts)
}

// PrintWebSocketMessage demonstrates printing WebSocket-related structures
func PrintWebSocketMessage(msgType, chatID, content string, metadata map[string]interface{}) {
	message := map[string]interface{}{
		"type":      msgType,
		"chatId":    chatID,
		"content":   content,
		"metadata":  metadata,
		"timestamp": time.Now(),
		"sender": map[string]interface{}{
			"userId":   bson.NewObjectID().Hex(),
			"username": "demo_user",
			"avatar":   "https://example.com/avatar.jpg",
		},
	}

	PrintWithTitle("WebSocket Message", message)
}

// QuickPrint is a convenience function for quick debugging
func QuickPrint(label string, v interface{}) {
	fmt.Printf("\n🔍 %s:\n", label)
	PrettyPrint(v)
	fmt.Println(strings.Repeat("-", 50))
}
