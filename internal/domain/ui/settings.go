package ui

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// ErrInvalidSetting is a setting out of its bounds or of no known name.
var ErrInvalidSetting = errors.New("invalid setting")

// The bounds and defaults of the settings; the bounds catch a typo rather
// than limit a taste.
const (
	DefaultContextLines  = 3
	MaxContextLines      = 1000
	DefaultHoverDelay    = 300 * time.Millisecond
	MaxHoverDelay        = 5 * time.Second
	DefaultSplitMinWidth = 120
	MaxSplitMinWidth     = 1000
)

// Layout is how the viewer lays out the two sides.
type Layout uint8

// The layouts. Auto is side by side from SplitMinWidth on and unified
// below it.
const (
	AutoLayout Layout = iota
	SplitLayout
	UnifiedLayout
	layouts
)

func layoutNames() [layouts]string {
	return [layouts]string{AutoLayout: "auto", SplitLayout: "split", UnifiedLayout: "unified"}
}

// String is the name of l in the configuration file.
func (l Layout) String() string {
	if l < layouts {
		return layoutNames()[l]
	}
	return fmt.Sprintf("Layout(%d)", uint8(l))
}

// ParseLayout is the layout named name.
func ParseLayout(name string) (Layout, error) {
	names := layoutNames()
	if i := slices.Index(names[:], name); i >= 0 {
		return Layout(i), nil
	}
	return 0, fmt.Errorf("%w: layout %q, want one of %q", ErrInvalidSetting, name, names)
}

// View is what the viewer shows of the files.
type View uint8

// The views. FileView shows one file at a time, StreamView all of them
// one below the other.
const (
	FileView View = iota
	StreamView
	views
)

func viewNames() [views]string {
	return [views]string{FileView: "file", StreamView: "stream"}
}

// String is the name of v in the configuration file.
func (v View) String() string {
	if v < views {
		return viewNames()[v]
	}
	return fmt.Sprintf("View(%d)", uint8(v))
}

// ParseView is the view named name.
func ParseView(name string) (View, error) {
	names := viewNames()
	if i := slices.Index(names[:], name); i >= 0 {
		return View(i), nil
	}
	return 0, fmt.Errorf("%w: view %q, want one of %q", ErrInvalidSetting, name, names)
}

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
	// Layout is how the sides stand: side by side, unified, or by width.
	Layout Layout
	// SplitMinWidth is the narrowest diff, in columns, that AutoLayout
	// shows side by side; zero never unifies.
	SplitMinWidth int
	// View is the view the viewer starts in.
	View View
}

// NewSettings checks the bounds of s and returns it.
func NewSettings(s Settings) (Settings, error) {
	switch {
	case s.ContextLines < 0 || s.ContextLines > MaxContextLines:
		return Settings{}, fmt.Errorf("%w: context_lines %d, want 0 to %d", ErrInvalidSetting, s.ContextLines, MaxContextLines)
	case s.HoverDelay < 0 || s.HoverDelay > MaxHoverDelay:
		return Settings{}, fmt.Errorf("%w: hover_delay %s, want 0 to %s", ErrInvalidSetting, s.HoverDelay, MaxHoverDelay)
	case s.Layout >= layouts:
		return Settings{}, fmt.Errorf("%w: no layout %s", ErrInvalidSetting, s.Layout)
	case s.View >= views:
		return Settings{}, fmt.Errorf("%w: no view %s", ErrInvalidSetting, s.View)
	case s.SplitMinWidth < 0 || s.SplitMinWidth > MaxSplitMinWidth:
		return Settings{}, fmt.Errorf("%w: split_min_width %d, want 0 to %d", ErrInvalidSetting, s.SplitMinWidth, MaxSplitMinWidth)
	}
	return s, nil
}

// DefaultSettings are the settings without a configuration.
func DefaultSettings() Settings {
	return Settings{
		Palette: Dark(), Keys: DefaultKeymap(), ContextLines: DefaultContextLines, HoverDelay: DefaultHoverDelay,
		SplitMinWidth: DefaultSplitMinWidth,
	}
}
