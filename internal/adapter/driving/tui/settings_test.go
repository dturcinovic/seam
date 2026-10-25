package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/in/infakes"
	"seam/internal/domain/change"
	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/syntax"
	"seam/internal/domain/ui"
)

// ownModel is a model of a.go and b.go with keys of its own, on a screen
// of 120 by 24.
func ownModel(t *testing.T, bindings map[ui.Action][]string) (model, *infakes.FakeFileDiffLoader) {
	t.Helper()
	keys, err := ui.NewKeymap(bindings)
	require.NoError(t, err)
	set := splitSettings()
	set.Keys = keys
	p := fakePorts()
	m := newModel(change.Changeset{Files: files("a.go", "b.go")}, p, set)
	m, _ = send(m, tea.WindowSizeMsg{Width: 120, Height: 24})
	return m, p.Files.(*infakes.FakeFileDiffLoader)
}

func ctrl(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl} }

func TestOwnKeysReplaceTheDefaults(t *testing.T) {
	m, fl := ownModel(t, map[ui.Action][]string{
		ui.Quit:     {"x"},
		ui.NextFile: {"ctrl+n"},
		ui.Reload:   {},
	})
	require.Equal(t, "ctrl+n", ctrl('n').String(), "as Bubble Tea names it")

	_, cmd := typed(m, "q")
	assert.Nil(t, cmd, "q no longer quits")
	_, cmd = typed(m, "x")
	require.NotNil(t, cmd)
	assert.Equal(t, tea.QuitMsg{}, cmd())
	m, _ = send(m, ctrl('n'))
	assert.Equal(t, 2, fl.LoadCallCount(), "ctrl+n opens the next file")
	_, _ = typed(m, "tab")
	assert.Equal(t, 2, fl.LoadCallCount(), "tab no longer does")

	assert.Equal(t, "x quit  f full/hunks  u unified/split  n/p change  ctrl+n/shift+tab file  s stream  j/k scroll  h/l sideways  b files  i", footerLine(m),
		"the footer names the own keys and drops the action without one")
}

func TestAnUpperLayerHidesTheKeyBelow(t *testing.T) {
	m, _ := ownModel(t, map[ui.Action][]string{ui.Quit: {"x"}, ui.Back: {"x"}})
	m, _ = typed(m, "b")
	require.True(t, m.s.List.Focus)

	assert.Equal(t, "enter open  x back  j/k move  [/] folder  g/G first/last  b close", footerLine(m),
		"x does not quit here, so the reminder leaves it out")
	m, cmd := typed(m, "x")

	assert.Nil(t, cmd, "x leaves the file list rather than quit")
	assert.False(t, m.s.List.Focus)
	_, cmd = typed(m, "x")
	require.NotNil(t, cmd)
	assert.Equal(t, tea.QuitMsg{}, cmd())
}

func TestOwnKeysInTheFileList(t *testing.T) {
	m, fl := ownModel(t, map[ui.Action][]string{ui.Open: {"o"}, ui.Down: {"ctrl+n"}})
	m, _ = typed(m, "b")
	require.True(t, m.s.List.Focus)

	m, _ = send(m, ctrl('n'))
	m, _ = typed(m, "o")

	assert.Equal(t, 2, fl.LoadCallCount(), "o opens the file ctrl+n moved to")
	assert.Equal(t, 1, m.s.Current)
}

func TestTheThemeIsThePalettes(t *testing.T) {
	dark, light := newTheme(ui.Dark()), newTheme(ui.Light())
	st := textStyle(insertedTone, syntax.Keyword, false)

	assert.Contains(t, light.open[st], "48;2;230;255;236", "light inserted background")
	assert.Contains(t, light.open[st], "38;2;166;38;164", "light keyword")
	assert.Contains(t, dark.open[st], "48;2;33;58;40")

	p := ui.Light()
	require.NoError(t, p.Set(ui.InsertedBg, ""))
	assert.NotContains(t, newTheme(p).open[st], "48;", "an empty colour keeps the terminal's")
}

func TestTheContextOfTheHunkView(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split(numbered(20)), diff.Split(numbered(20, 10)))
	for _, tt := range []struct{ context, lines int }{{0, 3}, {3, 9}} {
		set := splitSettings()
		set.ContextLines = tt.context
		s := newState(change.Changeset{Files: files("a.go")}, set)
		s.resize(80, 30)
		r, ok := s.open(0, false)
		require.True(t, ok)
		s.apply(filediff.Update{Gen: r.Gen, Snapshot: filediff.Snapshot{File: r.File, Diff: d}})

		assert.Len(t, s.Lines, tt.lines, "context %d: the change and its context between two folds", tt.context)
	}
}

func TestOwnKeysOfTheCaret(t *testing.T) {
	m, f := hoverModel(t)
	keys, err := ui.NewKeymap(map[ui.Action][]string{ui.Hover: {"?"}})
	require.NoError(t, err)
	m.s.setKeys(keys)

	m, _ = typed(m, "i", "K")
	assert.True(t, strings.HasPrefix(footerLine(m), "? hover"), footerLine(m))
	assert.Zero(t, f.hover.HoverCallCount(), "K no longer hovers")
	typed(m, "?")
	assert.Equal(t, 1, f.hover.HoverCallCount())
}
