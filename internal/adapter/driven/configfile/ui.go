package configfile

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/dturcinovic/seam/internal/domain/ui"
)

type uiEntry struct {
	Theme        *string `toml:"theme"`
	ContextLines *int    `toml:"context_lines"`
	// HoverDelay is a duration as Go writes it, such as "300ms".
	HoverDelay    *string `toml:"hover_delay"`
	Layout        *string `toml:"layout"`
	SplitMinWidth *int    `toml:"split_min_width"`
	View          *string `toml:"view"`
}

// settings lays the ui, colors and keys sections over the defaults, the
// colours over the theme's palette.
func (d document) settings() (ui.Settings, error) {
	palette, err := d.palette()
	if err != nil {
		return ui.Settings{}, err
	}
	keys, err := d.keymap()
	if err != nil {
		return ui.Settings{}, err
	}
	s := ui.DefaultSettings()
	s.Palette, s.Keys = palette, keys
	set(&s.ContextLines, d.UI.ContextLines)
	set(&s.SplitMinWidth, d.UI.SplitMinWidth)
	if d.UI.HoverDelay != nil {
		if s.HoverDelay, err = time.ParseDuration(*d.UI.HoverDelay); err != nil {
			return ui.Settings{}, fmt.Errorf("ui.hover_delay: %w", err)
		}
	}
	if d.UI.Layout != nil {
		if s.Layout, err = ui.ParseLayout(*d.UI.Layout); err != nil {
			return ui.Settings{}, fmt.Errorf("ui.layout: %w", err)
		}
	}
	if d.UI.View != nil {
		if s.View, err = ui.ParseView(*d.UI.View); err != nil {
			return ui.Settings{}, fmt.Errorf("ui.view: %w", err)
		}
	}
	if s, err = ui.NewSettings(s); err != nil {
		return ui.Settings{}, fmt.Errorf("ui: %w", err)
	}
	return s, nil
}

// palette is the theme's palette with the colours set, in the order of
// their names, so an error names the same entry on every run.
func (d document) palette() (ui.Palette, error) {
	p := ui.DefaultSettings().Palette
	if d.UI.Theme != nil {
		var err error
		if p, err = ui.ThemePalette(*d.UI.Theme); err != nil {
			return ui.Palette{}, fmt.Errorf("ui.theme: %w", err)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(d.Colors)) {
		c, ok := ui.ParseColour(name)
		if !ok {
			return ui.Palette{}, fmt.Errorf("colors.%s: %w: no such colour", name, ui.ErrInvalidColour)
		}
		if err := p.Set(c, d.Colors[name]); err != nil {
			return ui.Palette{}, fmt.Errorf("colors.%s: %w", name, err)
		}
	}
	return p, nil
}

func (d document) keymap() (ui.Keymap, error) {
	bindings := make(map[ui.Action][]string, len(d.Keys))
	for _, name := range slices.Sorted(maps.Keys(d.Keys)) {
		a, ok := ui.ParseAction(name)
		if !ok {
			return ui.Keymap{}, fmt.Errorf("keys.%s: %w: no such action", name, ui.ErrInvalidKeys)
		}
		bindings[a] = d.Keys[name]
	}
	k, err := ui.NewKeymap(bindings)
	if err != nil {
		return ui.Keymap{}, fmt.Errorf("keys: %w", err)
	}
	return k, nil
}
