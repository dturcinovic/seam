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
}

func TestNewSettings(t *testing.T) {
	tests := []struct {
		lines int
		delay time.Duration
		ok    bool
	}{
		{0, 0, true},
		{1000, 5 * time.Second, true},
		{-1, 0, false},
		{1001, 0, false},
		{3, -time.Millisecond, false},
		{3, 5*time.Second + 1, false},
	}
	for _, tt := range tests {
		s, err := ui.NewSettings(ui.Light(), ui.DefaultKeymap(), tt.lines, tt.delay)
		if !tt.ok {
			assert.ErrorIs(t, err, ui.ErrInvalidSetting, "%d %s", tt.lines, tt.delay)
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, ui.Settings{Palette: ui.Light(), Keys: ui.DefaultKeymap(), ContextLines: tt.lines, HoverDelay: tt.delay}, s)
	}
}
