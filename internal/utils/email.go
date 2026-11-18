package utils

import "strings"

// NormalizeEmail returns the form an email is stored and looked up in: trimmed
// and in lower case, so Alice@x.com and alice@x.com are one account (#36).
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
