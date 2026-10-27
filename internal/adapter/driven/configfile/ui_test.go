package configfile_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/adapter/driven/configfile"
	"github.com/dturcinovic/seam/internal/domain/limits"
	"github.com/dturcinovic/seam/internal/domain/syntax"
	"github.com/dturcinovic/seam/internal/domain/ui"
)

func loadFile(t *testing.T, text string) configfile.File {
	t.Helper()
	f, err := configfile.Load(write(t, text))
	require.NoError(t, err)
	return f
}

func TestTheViewersSettings(t *testing.T) {
	f := loadFile(t, `
[ui]
theme = "light"
context_lines = 0
hover_delay = "1s"
layout = "unified"
split_min_width = 0
view = "stream"

[colors]
inserted_bg = "#d7f5dd"
keyword = "5"
plain = ""

[keys]
next_change = ["ctrl+n", "n"]
reload = []
`)
	want := ui.Light()
	require.NoError(t, want.Set(ui.InsertedBg, "#d7f5dd"))
	require.NoError(t, want.Set(ui.KindColour(syntax.Keyword), "5"))
	assert.Equal(t, want, f.UI.Palette, "the colours over the theme")
	assert.Equal(t, 0, f.UI.ContextLines)
	assert.Equal(t, time.Second, f.UI.HoverDelay)
	assert.Equal(t, ui.UnifiedLayout, f.UI.Layout)
	assert.Equal(t, 0, f.UI.SplitMinWidth)
	assert.Equal(t, ui.StreamView, f.UI.View)
	assert.Equal(t, []string{"ctrl+n", "n"}, f.UI.Keys.Keys(ui.NextChange))
	assert.Empty(t, f.UI.Keys.Keys(ui.Reload))
	assert.Equal(t, []string{"p"}, f.UI.Keys.Keys(ui.PrevChange))
}

func TestColoursOverTheDefaultTheme(t *testing.T) {
	f := loadFile(t, "colors.dim_fg = \"#808080\"\n")
	want := ui.Dark()
	require.NoError(t, want.Set(ui.DimFg, "#808080"))
	assert.Equal(t, want, f.UI.Palette)
}

func TestTheLimits(t *testing.T) {
	f := loadFile(t, "[limits]\nmax_file_size = \"16M\"\nmax_instances = 2\n")
	assert.Equal(t, limits.FileSize(16<<20), f.MaxFileSize)
	assert.Equal(t, limits.Instances(2), f.MaxInstances)

	f = loadFile(t, "[ui]\ntheme = \"dark\"\n")
	assert.Zero(t, f.MaxFileSize, "not set: the flag or the default decides")
	assert.Zero(t, f.MaxInstances)
	assert.Equal(t, ui.DefaultSettings(), f.UI)
}

func TestBadSettingsNameTheirKey(t *testing.T) {
	tests := map[string]string{
		"ui.theme":                 `ui.theme = "solarized"`,
		"ui.hover_delay":           `ui.hover_delay = "300"`,
		"hover_delay 10s":          `ui.hover_delay = "10s"`,
		"context_lines -1":         `ui.context_lines = -1`,
		"ui.layout":                `ui.layout = "stacked"`,
		"ui.view":                  `ui.view = "all"`,
		"split_min_width 1001":     `ui.split_min_width = 1001`,
		"colors.inserted":          `colors.inserted = "#000000"`,
		"colors.inserted_bg":       `colors.inserted_bg = "green"`,
		"keys.exit":                `keys.exit = ["q"]`,
		`"x" is a key of both`:     "keys.reload = [\"x\"]\nkeys.next_change = [\"x\"]",
		"shift+q":                  `keys.quit = ["shift+q"]`,
		"limits.max_file_size":     `limits.max_file_size = "16MB"`,
		"limits.max_instances":     `limits.max_instances = 0`,
		"unknown key ui.colour":    `ui.colour = "dark"`,
		"unknown key limits.files": `limits.files = 3`,
	}
	for want, text := range tests {
		p := write(t, text+"\n")
		_, err := configfile.Load(p)
		require.Error(t, err, text)
		assert.Contains(t, err.Error(), want, text)
		assert.Contains(t, err.Error(), p, text)
	}
}

func TestTheUpdateCheck(t *testing.T) {
	assert.True(t, loadFile(t, "").UpdateCheck, "on unless turned off")
	assert.True(t, loadFile(t, "[updates]\ncheck = true\n").UpdateCheck)
	assert.False(t, loadFile(t, "[updates]\ncheck = false\n").UpdateCheck)
}
