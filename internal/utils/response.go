package utils

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// APIResponse represents a standard API response structure
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

// SuccessResponse sends a standardized success response
func SuccessResponse(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// ErrorResponse sends a standardized error response
func ErrorResponse(c *gin.Context, statusCode int, message string, err interface{}) {
	c.JSON(statusCode, APIResponse{
		Success: false,
		Message: message,
		Error:   err,
	})
}

// CreatedResponse sends a standardized response for resource creation
func CreatedResponse(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusCreated, APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// PaginatedResponse sends a standardized paginated response
func PaginatedResponse(c *gin.Context, message string, data interface{}, limit, total int) {
	response := APIResponse{
		Success: true,
		Message: message,
		Data: map[string]interface{}{
			"items": data,
			"pagination": map[string]interface{}{
				"limit":       limit,
				"total":       total,
				"total_pages": (total + limit - 1) / limit,
			},
		},
	}
	c.JSON(http.StatusOK, response)
}

// SplitAndTrim splits a string by delimiter and trims whitespace from each part
func SplitAndTrim(s, delimiter string) []string {
	if s == "" {
		return []string{}
	}

	parts := strings.Split(s, delimiter)
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}
