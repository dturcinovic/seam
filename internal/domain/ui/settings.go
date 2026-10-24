package ui

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidSetting is a context or hover delay out of its bounds.
var ErrInvalidSetting = errors.New("invalid setting")

// The bounds and defaults of the settings; the bounds catch a typo rather
// than limit a taste.
const (
	DefaultContextLines = 3
	MaxContextLines     = 1000
	DefaultHoverDelay   = 300 * time.Millisecond
	MaxHoverDelay       = 5 * time.Second
)

// Settings are how the viewer looks and takes keys. The zero value is not
// valid; use NewSettings or DefaultSettings.
type Settings struct {
	Palette Palette
	Keys    Keymap
	// ContextLines is how many unchanged lines the hunk view keeps around
	// a change.
	ContextLines int
	// HoverDelay is how long the mouse rests on a word before it is asked
	// about.
	HoverDelay time.Duration
}

// NewSettings checks the context and the delay and returns the settings.
func NewSettings(p Palette, k Keymap, contextLines int, hoverDelay time.Duration) (Settings, error) {
	if contextLines < 0 || contextLines > MaxContextLines {
		return Settings{}, fmt.Errorf("%w: context_lines %d, want 0 to %d", ErrInvalidSetting, contextLines, MaxContextLines)
	}
	if hoverDelay < 0 || hoverDelay > MaxHoverDelay {
		return Settings{}, fmt.Errorf("%w: hover_delay %s, want 0 to %s", ErrInvalidSetting, hoverDelay, MaxHoverDelay)
	}
	return Settings{Palette: p, Keys: k, ContextLines: contextLines, HoverDelay: hoverDelay}, nil
}

// DefaultSettings are the settings without a configuration.
func DefaultSettings() Settings {
	return Settings{Palette: Dark(), Keys: DefaultKeymap(), ContextLines: DefaultContextLines, HoverDelay: DefaultHoverDelay}
}
