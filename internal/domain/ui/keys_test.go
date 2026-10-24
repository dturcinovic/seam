package ui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/ui"
)

func TestEveryActionHasANameAndDefaultKeys(t *testing.T) {
	k := ui.DefaultKeymap()
	for a := range ui.Actions {
		name := a.String()
		require.NotContains(t, name, "Action(", "action %d has no name", a)
		got, ok := ui.ParseAction(name)
		require.True(t, ok, name)
		assert.Equal(t, a, got)
		assert.NotEmpty(t, k.Keys(a), name)
	}
	_, ok := ui.ParseAction("exit")
	assert.False(t, ok)
	assert.Nil(t, k.Keys(ui.Actions))
}

func TestTheDefaultKeysHaveNoConflict(t *testing.T) {
	k, err := ui.NewKeymap(nil)
	require.NoError(t, err)
	assert.Equal(t, ui.DefaultKeymap(), k)
}

func TestTheDefaultKeys(t *testing.T) {
	k := ui.DefaultKeymap()
	tests := []struct {
		layer ui.Layer
		key   string
		want  ui.Action
	}{
		{ui.ViewLayer, "q", ui.Quit},
		{ui.ViewLayer, "esc", ui.Quit},
		{ui.ViewLayer, "j", ui.Down},
		{ui.ViewLayer, "G", ui.Bottom},
		{ui.ViewLayer, "shift+tab", ui.PrevFile},
		{ui.ViewLayer, "[", ui.PrevFile},
		{ui.ListLayer, "[", ui.PrevFolder},
		{ui.ListLayer, "j", ui.Down},
		{ui.ListLayer, "esc", ui.Back},
		{ui.CaretLayer, "j", ui.CaretDown},
		{ui.CaretLayer, "b", ui.WordBack},
		{ui.CaretLayer, "esc", ui.LeaveCaret},
		{ui.PopupLayer, "esc", ui.ClosePopup},
	}
	for _, tt := range tests {
		got, ok := k.Action(tt.layer, tt.key)
		require.True(t, ok, "%d %q", tt.layer, tt.key)
		assert.Equal(t, tt.want, got, "%d %q", tt.layer, tt.key)
	}
	_, ok := k.Action(ui.ListLayer, "q")
	assert.False(t, ok, "the list leaves quitting to the view")
	_, ok = k.Action(ui.CaretLayer, "ctrl+d")
	assert.False(t, ok, "the caret leaves scrolling to the view")
	_, ok = k.Action(ui.Layers, "q")
	assert.False(t, ok)
}

func TestABindingReplacesTheDefaultsOfItsAction(t *testing.T) {
	k, err := ui.NewKeymap(map[ui.Action][]string{
		ui.NextChange: {"ctrl+n", "n"},
		ui.Hover:      {"?"},
		ui.Reload:     {},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"ctrl+n", "n"}, k.Keys(ui.NextChange))
	got, ok := k.Action(ui.CaretLayer, "?")
	require.True(t, ok)
	assert.Equal(t, ui.Hover, got)
	_, ok = k.Action(ui.CaretLayer, "K")
	assert.False(t, ok, "K no longer hovers")
	_, ok = k.Action(ui.ViewLayer, "r")
	assert.False(t, ok, "reload has no key")
	assert.Empty(t, k.Keys(ui.Reload))
	assert.Equal(t, []string{"p"}, k.Keys(ui.PrevChange), "other actions keep theirs")
}

func TestNewKeymapRefuses(t *testing.T) {
	tests := map[string]map[ui.Action][]string{
		"no key to quit":       {ui.Quit: {}},
		"a key twice in view":  {ui.Reload: {"n"}},
		"a motion in the list": {ui.Open: {"j"}},
		"no action":            {ui.Actions: {"x"}},
		"two characters":       {ui.Quit: {"qq"}},
		"empty":                {ui.Quit: {""}},
		"a space":              {ui.Quit: {" "}},
		"a control character":  {ui.Quit: {"\x01"}},
		"shifted character":    {ui.Quit: {"shift+q"}},
		"capital with alt":     {ui.Quit: {"alt+Q"}},
		"capital with ctrl":    {ui.Quit: {"ctrl+G"}},
		"capital with shift":   {ui.Quit: {"alt+shift+Q"}},
		"modifier order":       {ui.Quit: {"alt+ctrl+q"}},
		"modifier twice":       {ui.Quit: {"ctrl+ctrl+q"}},
		"unknown modifier":     {ui.Quit: {"cmd+q"}},
		"unknown name":         {ui.Quit: {"escape"}},
		"function key 0":       {ui.Quit: {"f0"}},
		"function key 64":      {ui.Quit: {"f64"}},
		"function key padded":  {ui.Quit: {"f01"}},
		"modifier alone":       {ui.Quit: {"ctrl+"}},
	}
	for name, bindings := range tests {
		_, err := ui.NewKeymap(bindings)
		assert.ErrorIs(t, err, ui.ErrInvalidKeys, name)
	}
}

func TestNewKeymapTakes(t *testing.T) {
	for _, key := range []string{"q", "Q", "+", "ctrl++", "ctrl+shift+tab", "alt+x", "ctrl+alt+shift+meta+hyper+super+x", "f1", "f63", "space", "pgdown", "ü", "?"} {
		_, err := ui.NewKeymap(map[ui.Action][]string{ui.ClosePopup: {key}})
		assert.NoError(t, err, key)
	}
}

func TestLayersHideTheKeysBelowThem(t *testing.T) {
	k, err := ui.NewKeymap(map[ui.Action][]string{ui.Hover: {"q"}})
	require.NoError(t, err, "a caret key may hide a view key")

	got, _ := k.Action(ui.CaretLayer, "q")
	assert.Equal(t, ui.Hover, got)
	got, _ = k.Action(ui.ViewLayer, "q")
	assert.Equal(t, ui.Quit, got)
}

func TestTheConflictNamesBothActions(t *testing.T) {
	_, err := ui.NewKeymap(map[ui.Action][]string{ui.Reload: {"n"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"n" is a key of both reload and next_change`)
}

func TestKeysAreCopies(t *testing.T) {
	keys := []string{"x"}
	k, err := ui.NewKeymap(map[ui.Action][]string{ui.Reload: keys})
	require.NoError(t, err)
	keys[0] = "y"
	k.Keys(ui.Reload)[0] = "z"
	assert.Equal(t, []string{"x"}, k.Keys(ui.Reload))
}
