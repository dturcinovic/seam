package ui_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/ui"
)

func TestDefaultSettings(t *testing.T) {
	s := ui.DefaultSettings()
	assert.Equal(t, ui.Dark(), s.Palette)
	assert.Equal(t, ui.DefaultKeymap(), s.Keys)
	assert.Equal(t, 3, s.ContextLines)
	assert.Equal(t, 300*time.Millisecond, s.HoverDelay)
	assert.Equal(t, ui.AutoLayout, s.Layout)
	assert.Equal(t, 120, s.SplitMinWidth)
}

func TestNewSettings(t *testing.T) {
	tests := []struct {
		name string
		set  func(*ui.Settings)
		ok   bool
	}{
		{"least", func(s *ui.Settings) { s.ContextLines, s.HoverDelay, s.SplitMinWidth = 0, 0, 0 }, true},
		{"most", func(s *ui.Settings) { s.ContextLines, s.HoverDelay, s.SplitMinWidth = 1000, 5*time.Second, 1000 }, true},
		{"unified", func(s *ui.Settings) { s.Layout = ui.UnifiedLayout }, true},
		{"negative context", func(s *ui.Settings) { s.ContextLines = -1 }, false},
		{"too much context", func(s *ui.Settings) { s.ContextLines = 1001 }, false},
		{"negative delay", func(s *ui.Settings) { s.HoverDelay = -time.Millisecond }, false},
		{"too long a delay", func(s *ui.Settings) { s.HoverDelay = 5*time.Second + 1 }, false},
		{"no layout", func(s *ui.Settings) { s.Layout = 3 }, false},
		{"negative width", func(s *ui.Settings) { s.SplitMinWidth = -1 }, false},
		{"too wide", func(s *ui.Settings) { s.SplitMinWidth = 1001 }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := ui.DefaultSettings()
			want.Palette = ui.Light()
			tt.set(&want)

			s, err := ui.NewSettings(want)

			if !tt.ok {
				assert.ErrorIs(t, err, ui.ErrInvalidSetting)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, want, s)
		})
	}
}

func TestLayoutNames(t *testing.T) {
	for _, l := range []ui.Layout{ui.AutoLayout, ui.SplitLayout, ui.UnifiedLayout} {
		got, err := ui.ParseLayout(l.String())
		require.NoError(t, err)
		assert.Equal(t, l, got)
	}
	assert.Equal(t, "unified", ui.UnifiedLayout.String())
	assert.Equal(t, "Layout(3)", ui.Layout(3).String())
	_, err := ui.ParseLayout("Unified")
	assert.ErrorIs(t, err, ui.ErrInvalidSetting)
}
