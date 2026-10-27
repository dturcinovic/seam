package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/in/infakes"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/note"
)

func TestTheOpenFieldTakesEveryKeyAsText(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(noteLimit)

	for _, k := range []string{"q", "x", "v", "S", "j"} {
		var cmd tea.Cmd
		m, cmd = key(m, keyMsg(k))
		assert.Nil(t, cmd, k)
	}

	assert.Equal(t, "qxvSj|", marked(m.s.Field))
}

func TestTextWithModifiersIsText(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(noteLimit)

	m, _ = key(m, tea.KeyPressMsg{Code: 'q', Text: "@", Mod: tea.ModCtrl | tea.ModAlt})

	assert.Equal(t, "@|", marked(m.s.Field), "AltGr as some terminals report it")
}

func TestOpeningTheFieldDropsWhatWouldMoveTheCode(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.s.Jump.Inflight, m.s.Note = 7, "no definition found"

	m.openField(noteLimit)

	def := m.p.Define.(*infakes.FakeDefiner)
	require.Equal(t, 1, def.CancelDefinitionCallCount())
	assert.EqualValues(t, 7, def.CancelDefinitionArgsForCall(0))
	assert.Zero(t, m.s.Jump.Inflight)
	assert.Equal(t, noteKeys, footerLine(m), "the old note gives way; with no lines for the box, the footer names the keys")
}

func TestCtrlCClosesTheFieldOnly(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(noteLimit)

	m, cmd := key(m, keyMsg("ctrl+c"))

	assert.Nil(t, cmd, "seam runs on")
	assert.False(t, m.s.Field.Open)
	_, cmd = key(m, keyMsg("q"))
	assert.NotNil(t, cmd, "with the field closed, q quits again")
}

func TestAPasteGoesIntoTheField(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.s = *screen(t, 80)
	m.s.toggleLayout()
	m.openField(noteLimit)

	m, _ = send(m, tea.PasteMsg{Content: "a\x1bb\x07c\r\nd"})

	assert.Equal(t, "a\x1bb\x07c\nd|", marked(m.s.Field))
	assert.Empty(t, footerLine(m), "the box names the keys")
	box := boxLines(m.s)
	assert.Equal(t, "│ a␛b␇c", strings.TrimRight(box[1+fieldGap], " │"), "controls show replaced, off the frame")
	assert.Equal(t, "│ d", strings.TrimRight(box[2+fieldGap], " │"), "the line break stays a row")
}

func TestTheFooterSaysWhatTheLimitKeptOut(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(3)

	m, _ = send(m, tea.PasteMsg{Content: "four"})

	assert.Empty(t, m.s.Field.Text)
	assert.Equal(t, "text over 3 bytes, not taken", footerLine(m))
	m, _ = key(m, keyMsg("a"))
	assert.Equal(t, noteKeys, footerLine(m), "the next key takes the note away")
}

func TestTheMouseRestsWhileTheFieldIsOpen(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(noteLimit)
	before := m.s.Pos

	m, cmd := send(m, tea.MouseMotionMsg{X: 40, Y: 10})
	assert.Nil(t, cmd)
	m, cmd = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: 40, Y: 10})
	assert.Nil(t, cmd)

	assert.Equal(t, before, m.s.Pos)
	assert.False(t, m.s.Hover.PlusOn)
	assert.True(t, m.s.Field.Open)
}

func TestAResizeKeepsTheDraft(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	m.openField(noteLimit)
	m, _ = send(m, tea.PasteMsg{Content: strings.Repeat("word ", 30)})

	m, _ = send(m, tea.WindowSizeMsg{Width: 40, Height: 24})

	assert.Equal(t, strings.Repeat("word ", 30)+"|", marked(m.s.Field))
	from, to := m.s.gapColumns()
	assert.Equal(t, to-from, m.s.Field.W, "the box keeps to the text of the side")
	assert.Equal(t, from, m.s.Field.X)
	for _, l := range boxLines(m.s) {
		assert.Equal(t, m.s.Field.W, textWidth(l, m.s.Cells))
	}
}

// draftAt opens the field for a note of kind k at the lines of t.
func draftAt(s *state, t noteTarget, k note.Kind) {
	s.Notes.Draft = noteDraft{Kind: k, Lines: t}
	s.openField(noteLimit)
}

// boxLines are the field's lines on the screen without styles.
func boxLines(s state) []string {
	lines := plainLines(&s)
	f := s.Field
	g := s.gap()
	if !g.OK {
		return nil
	}
	y := bodyTop + g.At - s.top() + 1
	var box []string
	for _, l := range lines[y : y+f.H] {
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

	assert.Len(t, boxLines(*byName["field-empty"]), fieldMinRows+popupFrame+fieldGap, "room for the text before it is written")
	assert.Contains(t, boxLines(*byName["field-line"])[1+fieldGap], "Why does this return nil?")
	wrapped := byName["field-wrap"].Field
	assert.Greater(t, len(wrapped.Rows), 2)
	assert.Equal(t, len(wrapped.Rows)+popupFrame+fieldGap, wrapped.H, "all rows in view")
	scrolled := byName["field-scroll"].Field
	assert.Greater(t, scrolled.Scroll, 0)
	assert.Less(t, scrolled.H, len(scrolled.Rows)+popupFrame)
}

func TestTheFrameNamesTheKeysWhereTheyFit(t *testing.T) {
	s := screen(t, 80)
	s.toggleLayout()
	s.openField(noteLimit)
	box := boxLines(*s)
	assert.True(t, strings.HasSuffix(box[len(box)-1], " "+noteKeys+" ─┘"), box[len(box)-1])

	s = screen(t, 80)
	draftAt(s, noteTarget{Side: filediff.OldSide}, note.Question)
	box = boxLines(*s)
	assert.Equal(t, "└"+strings.Repeat("─", s.Field.W-popupFrame)+"┘", box[len(box)-1], "too narrow beside the other side")
	assert.Equal(t, noteKeys, strings.TrimSpace(plainLines(s)[s.Layout.Height-1]), "the footer names them")
}
