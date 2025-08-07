package utils

import (
	"strconv"
)

// ParseInt parses a string to int with a default value
func ParseInt(s string, defaultVal int) int {
	if val, err := strconv.Atoi(s); err == nil {
		return val
	}
	return defaultVal
}
