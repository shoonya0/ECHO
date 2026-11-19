package utils

import (
	"regexp"
	"strings"
)

// usernamePattern is the format of a new or changed username (#36): 3 to 30 ASCII
// letters, digits, _ or . so it can be typed as an @handle. Names stored before #36
// may not match; they stay valid until their owner changes them.
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.]{3,30}$`)

// NormalizeUsername returns the form a username is stored in: trimmed, with its
// capitals kept. Uniqueness ignores case (the username_ci index), not storage.
func NormalizeUsername(username string) string {
	return strings.TrimSpace(username)
}

// ValidUsername reports whether a normalized username has the format new names need.
func ValidUsername(username string) bool {
	return usernamePattern.MatchString(username)
}
