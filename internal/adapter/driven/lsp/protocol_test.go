package lsp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCharacterCountsInTheNegotiatedEncoding: the offset before a grapheme
// in bytes for utf-8, in code units for utf-16, where a code point beyond
// the BMP takes two.
func TestCharacterCountsInTheNegotiatedEncoding(t *testing.T) {
	text := "first\ncafé x\n日本 x\n👨‍👩 x\n𝒳 x\n"
	tests := []struct {
		name        string
		line, byte  int
		utf8, utf16 int
	}{
		{"after a combining accent", 1, len("café "), 7, 6},
		{"after CJK", 2, len("日本 "), 7, 3},
		{"after a ZWJ sequence", 3, len("👨‍👩 "), 12, 6},
		{"after a surrogate pair", 4, len("𝒳 "), 5, 3},
		{"inside a code point, at its start", 2, 1, 0, 0},
		{"past the end of the line", 1, 100, 8, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := character(text, tt.line, tt.byte, "utf-8")
			assert.True(t, ok)
			assert.Equal(t, tt.utf8, got, "utf-8")
			got, ok = character(text, tt.line, tt.byte, "utf-16")
			assert.True(t, ok)
			assert.Equal(t, tt.utf16, got, "utf-16")
		})
	}
	_, ok := character(text, 9, 0, "utf-16")
	assert.False(t, ok, "no such line")
}
