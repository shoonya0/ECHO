package services

import (
	"errors"
	"strings"
	"testing"
)

// Real emoji from a phone's emoji picker are accepted, including ones joined from
// several emoji, which the old 10-byte limit refused.
func TestValidateEmojiAcceptsRealEmoji(t *testing.T) {
	good := []struct {
		name  string
		emoji string
	}{
		{"thumbs up 👍", "\U0001F44D"},
		{"skin tone 👍🏽", "\U0001F44D\U0001F3FD"},
		{"red heart ❤️", "\u2764\ufe0f"},
		{"flag 🇮🇳", "\U0001F1EE\U0001F1F3"},
		{"keycap 1️⃣", "1\ufe0f\u20e3"},
		{"keycap #️⃣", "#\ufe0f\u20e3"},
		{"woman shrugging 🤷‍♀️, 13 bytes", "\U0001F937\u200d\u2640\ufe0f"},
		{"technologist 🧑‍💻, 11 bytes", "\U0001F9D1\u200d\U0001F4BB"},
		{"heart on fire ❤️‍🔥, 13 bytes", "\u2764\ufe0f\u200d\U0001F525"},
		{"rainbow flag 🏳️‍🌈, 14 bytes", "\U0001F3F3\ufe0f\u200d\U0001F308"},
		{"family 👨‍👩‍👧‍👦, 25 bytes", "\U0001F468\u200d\U0001F469\u200d\U0001F467\u200d\U0001F466"},
		{"kiss with two skin tones, 35 bytes", "\U0001F9D1\U0001F3FB\u200d\u2764\ufe0f\u200d\U0001F48B\u200d\U0001F9D1\U0001F3FC"},
		{"Scotland flag 🏴󠁧󠁢󠁳󠁣󠁴󠁿", "\U0001F3F4\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F"},
		{"double exclamation ‼️", "\u203c\ufe0f"},
		{"information ℹ️", "\u2139\ufe0f"},
		{"wavy dash 〰️", "\u3030\ufe0f"},
		{"copyright ©️", "\u00a9\ufe0f"},
		{"left-right arrow ↔️", "\u2194\ufe0f"},
		{"newer than Go's Unicode tables", "\U0001FAE9"},
	}
	for _, tc := range good {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateEmoji(tc.emoji); err != nil {
				t.Fatalf("validateEmoji(%q) = %v, want nil", tc.emoji, err)
			}
		})
	}
}

// A value that would reach other fields (#58), or that isn't made of emoji
// characters, is refused.
func TestValidateEmojiRefusesOtherValues(t *testing.T) {
	bad := []struct {
		name  string
		emoji string
	}{
		{"empty", ""},
		{"dot between emoji", "\U0001F44D.\U0001F44D"},
		{"dollar", "$\U0001F44D"},
		{"NUL", "\U0001F44D\x00"},
		{"not UTF-8", "\xff\xfe"},
		{"over 64 bytes", strings.Repeat("\U0001F44D", 17)},
		{"letters", "lol"},
		{"JavaScript key", "__proto__"},
		{"space", "\U0001F44D \U0001F44D"},
		{"line break", "\U0001F44D\n"},
		{"right-to-left override", "\u202e\U0001F44D"},
		{"zero-width space", "\U0001F44D\u200b"},
		{"tab", "\t"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateEmoji(tc.emoji); !errors.Is(err, ErrInvalidEmoji) {
				t.Fatalf("validateEmoji(%q) = %v, want ErrInvalidEmoji", tc.emoji, err)
			}
		})
	}
}
