package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/in/infakes"
)

func TestTheOpenFieldTakesEveryKeyAsText(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(noteLimit, 0, 3)

	for _, k := range []string{"q", "x", "v", "S", "j"} {
		var cmd tea.Cmd
		m, cmd = key(m, keyMsg(k))
		assert.Nil(t, cmd, k)
	}

	assert.Equal(t, "qxvSj|", marked(m.s.Field))
}

func TestTextWithModifiersIsText(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(noteLimit, 0, 3)

	m, _ = key(m, tea.KeyPressMsg{Code: 'q', Text: "@", Mod: tea.ModCtrl | tea.ModAlt})

	assert.Equal(t, "@|", marked(m.s.Field), "AltGr as some terminals report it")
}

func TestOpeningTheFieldDropsWhatWouldMoveTheCode(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.s.Jump.Inflight, m.s.Note = 7, "no definition found"

	m.openField(noteLimit, 0, 3)

	def := m.p.Define.(*infakes.FakeDefiner)
	require.Equal(t, 1, def.CancelDefinitionCallCount())
	assert.EqualValues(t, 7, def.CancelDefinitionArgsForCall(0))
	assert.Zero(t, m.s.Jump.Inflight)
	assert.Equal(t, fieldHints, footerLine(m), "the old note gives way")
}

func TestCtrlCClosesTheFieldOnly(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(noteLimit, 0, 3)

	m, cmd := key(m, keyMsg("ctrl+c"))

	assert.Nil(t, cmd, "seam runs on")
	assert.False(t, m.s.Field.Open)
	_, cmd = key(m, keyMsg("q"))
	assert.NotNil(t, cmd, "with the field closed, q quits again")
}

func TestAPasteGoesIntoTheField(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(noteLimit, 0, 3)

	m, _ = send(m, tea.PasteMsg{Content: "a\x1bb\x07c\r\nd"})

	assert.Equal(t, "a\x1bb\x07c\nd|", marked(m.s.Field))
	box := boxLines(m.s)
	assert.Equal(t, "│a␛b␇c", strings.TrimRight(box[1], " │"), "controls show replaced")
	assert.Equal(t, "│d", strings.TrimRight(box[2], " │"), "the line break stays a row")
}

func TestTheFooterSaysWhatTheLimitKeptOut(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(3, 0, 3)

	m, _ = send(m, tea.PasteMsg{Content: "four"})

	assert.Empty(t, m.s.Field.Text)
	assert.Equal(t, "text over 3 bytes, not taken", footerLine(m))
	m, _ = key(m, keyMsg("a"))
	assert.Equal(t, fieldHints, footerLine(m), "the next key takes the note away")
}

func TestTheMouseRestsWhileTheFieldIsOpen(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(noteLimit, 0, 3)
	before := m.s.Pos

	m, cmd := send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 40, Y: 10})
	assert.Nil(t, cmd)
	m, cmd = send(m, tea.MouseMotionMsg{X: 40, Y: 10})
	assert.Nil(t, cmd)

	assert.Equal(t, before, m.s.Pos)
	assert.True(t, m.s.Field.Open)
}

func TestAResizeKeepsTheDraft(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(noteLimit, 30, 10)
	m, _ = send(m, tea.PasteMsg{Content: strings.Repeat("word ", 30)})

	m, _ = send(m, tea.WindowSizeMsg{Width: 40, Height: 24})

	assert.Equal(t, strings.Repeat("word ", 30)+"|", marked(m.s.Field))
	assert.Equal(t, 40, m.s.Field.W)
	assert.Equal(t, 0, m.s.Field.X, "the box moves left to stay on the screen")
	for _, l := range boxLines(m.s) {
		assert.Equal(t, 40, textWidth(l, m.s.Cells))
	}
}

// boxLines are the field's lines on the screen without styles.
func boxLines(s state) []string {
	lines := plainLines(&s)
	f := s.Field
	var box []string
	for _, l := range lines[f.Y : f.Y+f.H] {
		box = append(box, cutColumns(l, f.X, f.W, s))
	}
	return box
}

// cutColumns is w columns of the plain line l from column x.
func cutColumns(l string, x, w int, s state) string {
	var b strings.Builder
	cells := seek(l, nil, x, s.Cells)
	for c, ok := cells.Next(); ok && c.Col < x+w; c, ok = cells.Next() {
		b.WriteString(l[c.Start:c.End])
	}
	return b.String()
}

func TestTheGoldenFieldsShowTheirCases(t *testing.T) {
	byName := map[string]*state{}
	for _, f := range fieldFixtures(t) {
		byName[f.name] = f.state
	}
	require.Len(t, byName, 4)

	assert.Len(t, boxLines(*byName["field-empty"]), 3)
	assert.Contains(t, boxLines(*byName["field-line"])[1], "Why does this return nil?")
	wrapped := byName["field-wrap"].Field
	assert.Greater(t, len(wrapped.Rows), 2)
	assert.Equal(t, len(wrapped.Rows)+popupFrame, wrapped.H, "all rows in view")
	scrolled := byName["field-scroll"].Field
	assert.Greater(t, scrolled.Scroll, 0)
	assert.Less(t, scrolled.H, len(scrolled.Rows)+popupFrame)
}

func TestTheFrameNamesTheKeysWhereTheyFit(t *testing.T) {
	for _, c := range []struct {
		width int
		want  string
	}{
		{80, "└ " + fieldHints + "  tab question or finding ─"},
		{40, "└" + strings.Repeat("─", 38) + "┘"},
	} {
		s := screen(t, c.width)
		s.openField(noteLimit, 0, 5)
		s.Field.Hint = "tab question or finding"
		box := boxLines(*s)
		assert.Contains(t, box[len(box)-1], c.want, "width %d", c.width)
	}
}
