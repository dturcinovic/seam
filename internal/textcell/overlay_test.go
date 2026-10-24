package textcell_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"seam/internal/textcell"
)

func TestOverlay(t *testing.T) {
	tests := []struct {
		name, under string
		at          int
		want        string
	}{
		{"plain", "abcdefgh", 2, "ab[]efgh"},
		{"a wide grapheme cut on the left", "a日本語x", 2, "a [] 語x"},
		{"a wide grapheme cut on the right", "ab日本x ", 3, "ab [] x "},
		{"a short line", "ab", 4, "ab  []  "},
		{"a cut grapheme's blank in the line's style", "\x1b[32mab日本x \x1b[m", 3, "\x1b[32mab\x1b[m \x1b[m[]\x1b[32m x \x1b[m"},
		{"styles kept after", "\x1b[31mabc\x1b[32mdefgh\x1b[m", 2, "\x1b[31mab\x1b[32m\x1b[m\x1b[m[]\x1b[31m\x1b[32mefgh\x1b[m"},
	}
	for _, tt := range tests {
		for _, m := range []textcell.Method{textcell.Codepoints, textcell.Graphemes} {
			assert.Equal(t, tt.want, textcell.Overlay(tt.under, tt.at, "[]", 8, m), tt.name)
		}
	}
}
