package ui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/syntax"
	"seam/internal/domain/ui"
)

func TestEveryColourHasAName(t *testing.T) {
	seen := map[string]bool{}
	for c := range ui.Colours {
		name := c.String()
		require.NotContains(t, name, "Colour(", "role %d has no name", c)
		assert.False(t, seen[name], "%s twice", name)
		seen[name] = true
		got, ok := ui.ParseColour(name)
		require.True(t, ok, name)
		assert.Equal(t, c, got)
	}
	_, ok := ui.ParseColour("inserted")
	assert.False(t, ok)
}

func TestKindsHaveRoles(t *testing.T) {
	assert.Equal(t, "keyword", ui.KindColour(syntax.Keyword).String())
	assert.Equal(t, "plain", ui.KindColour(syntax.Plain).String())
	assert.Equal(t, "#c678dd", ui.Dark().Colour(ui.KindColour(syntax.Keyword)))
	assert.Empty(t, ui.Dark().Colour(ui.KindColour(syntax.Punctuation)), "punctuation keeps the terminal's colour")
}

func TestThemePalette(t *testing.T) {
	dark, err := ui.ThemePalette("dark")
	require.NoError(t, err)
	assert.Equal(t, ui.Dark(), dark)
	light, err := ui.ThemePalette("light")
	require.NoError(t, err)
	assert.Equal(t, ui.Light(), light)
	assert.NotEqual(t, dark.Colour(ui.InsertedBg), light.Colour(ui.InsertedBg))

	_, err = ui.ThemePalette("solarized")
	assert.ErrorIs(t, err, ui.ErrUnknownTheme)
}

func TestBuiltInPalettesColourEveryRoleButPlainText(t *testing.T) {
	for name, p := range map[string]ui.Palette{"dark": ui.Dark(), "light": ui.Light()} {
		for c := range ui.Colours {
			if c == ui.KindColour(syntax.Plain) || c == ui.KindColour(syntax.Punctuation) {
				continue
			}
			assert.NotEmpty(t, p.Colour(c), "%s: %s", name, c)
		}
	}
}

func TestPaletteSet(t *testing.T) {
	tests := []struct {
		value string
		ok    bool
	}{
		{"#d7f5dd", true},
		{"#D7F5DD", true},
		{"", true},
		{"0", true},
		{"255", true},
		{"256", false},
		{"-1", false},
		{"007", false},
		{"#d7f5d", false},
		{"#d7f5ddx", false},
		{"#gggggg", false},
		{"green", false},
	}
	for _, tt := range tests {
		p := ui.Dark()
		err := p.Set(ui.InsertedBg, tt.value)
		if !tt.ok {
			assert.ErrorIs(t, err, ui.ErrInvalidColour, tt.value)
			assert.Equal(t, ui.Dark(), p, "unchanged after %q", tt.value)
			continue
		}
		require.NoError(t, err, tt.value)
		assert.Equal(t, tt.value, p.Colour(ui.InsertedBg))
	}
	p := ui.Dark()
	assert.ErrorIs(t, p.Set(ui.Colours, "#000000"), ui.ErrInvalidColour)
	assert.Empty(t, p.Colour(ui.Colours))
}
