package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/in/infakes"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/note"
)

// reloadedTo is m after the watcher saw a change that leaves the files
// fs, before any of them loaded afresh.
func reloadedTo(m model, fs ...change.ChangedFile) model {
	m = watchNews(m, change.WatchUpdate{Kind: change.Changed})
	rl := m.p.Reload.(*infakes.FakeChangesetReloader)
	rl.DrainReturns([]change.Update{{Gen: rl.ReloadArgsForCall(rl.ReloadCallCount() - 1), Changeset: change.Changeset{Comparison: m.s.Comparison, Files: fs}}})
	m, _ = send(m, changesetReady{})
	return m
}

// changedTo is reloadedTo with a.go loaded afresh, its worktree side now
// text.
func changedTo(t *testing.T, m model, text string, fs ...change.ChangedFile) model {
	t.Helper()
	m = reloadedTo(m, fs...)
	fl := m.p.Files.(*infakes.FakeFileDiffLoader)
	r := fl.LoadArgsForCall(fl.LoadCallCount() - 1)
	fl.DrainReturns([]filediff.Update{{Gen: r.Gen, Snapshot: noteSnapshot(t, r.File, text)}})
	m, _ = send(m, fileDiffsReady{})
	return m
}

// insertedAbove is the worktree text of noteModel with lines put in after
// its first, above the lines the tests note.
func insertedAbove(lines ...string) string {
	_, text := noteText()
	return strings.Join(slices.Insert(strings.SplitAfter(text, "\n"), 1, lines...), "")
}

// withoutLinesFrom5 is the worktree text of noteModel without its 0-based
// lines 5 to to, the line 10 the tests note among them.
func withoutLinesFrom5(to int) string {
	_, text := noteText()
	return strings.Join(slices.Delete(strings.SplitAfter(text, "\n"), 5, to+1), "")
}

func noteNew() string {
	_, text := noteText()
	return text
}

// toLine10 is the keys that put the caret on line 10, then keys.
func toLine10(keys ...string) []string {
	return slices.Concat([]string{"i"}, slices.Repeat([]string{"j"}, 9), keys)
}

// fieldAtLine10 is noteModel with the field open at line 10 of a.go's
// worktree side and "draft" written in it.
func fieldAtLine10(t *testing.T) (model, noteFakes) {
	t.Helper()
	m, f := noteModel(t, headToWorktree(t))
	m, _ = typed(m, toLine10("c", "d", "r", "a", "f", "t")...)
	require.True(t, m.s.Field.Open)
	require.Equal(t, noteTarget{File: 0, Side: filediff.NewSide, From: 9, To: 9}, m.s.Notes.Draft.Lines)
	return m, f
}

func TestTheFieldFollowsItsLinesThroughAReload(t *testing.T) {
	m, _ := fieldAtLine10(t)

	m = changedTo(t, m, insertedAbove("x\n", "y\n", "z\n"), append(files("0.go"), m.s.Files...)...)

	assert.Equal(t, noteTarget{File: 1, Side: filediff.NewSide, From: 12, To: 12}, m.s.Notes.Draft.Lines, "a.go comes second now, after 0.go")
	assert.Equal(t, linesFound, m.s.Notes.Draft.Lost)
	assert.Equal(t, "draft", m.s.Field.Text)
	assert.Equal(t, len("draft"), m.s.Field.Cursor)
	assert.Equal(t, note.Question, m.s.Notes.Draft.Kind)
	assert.True(t, m.s.drafting(1, filediff.NewSide, 12), "the mark moved along")
	assert.False(t, m.s.drafting(1, filediff.NewSide, 9))
	at, ok := m.s.lineIndex(1, filediff.NewSide, 12)
	require.True(t, ok)
	assert.Equal(t, gapSpot{At: at, H: m.s.Field.H, OK: true}, m.s.gap(), "the box below line 13")
}

func TestAReloadPlacesTheFieldAsItFindsItsLines(t *testing.T) {
	_, text := noteText()
	all := strings.SplitAfter(text, "\n")
	b := func(m model) []change.ChangedFile { return m.s.Files[1:] }
	tests := map[string]struct {
		text   string
		files  func(model) []change.ChangedFile
		lines  noteTarget
		lost   draftLost
		footer string
	}{
		"inserted above, in sight": {text: insertedAbove(slices.Repeat([]string{"x\n"}, 10)...), lines: noteTarget{From: 19, To: 19}},
		"changed within":           {text: strings.Replace(text, "\n9\n", "\nnine\n", 1), lines: noteTarget{From: 9, To: 9}, footer: linesChanged},
		"deleted":                  {text: withoutLinesFrom5(12), lines: noteTarget{From: 9, To: 9}, lost: linesNotFound},
		"deleted near the end":     {text: withoutLinesFrom5(78), lines: noteTarget{From: 5, To: 5}, lost: linesNotFound},
		"copied below":             {text: text + strings.Join(all[6:13], ""), lines: noteTarget{From: 9, To: 9}, lost: linesAmbiguous},
		"the file shrunk to them":  {text: strings.Join(all[6:13], ""), lines: noteTarget{From: 3, To: 3}},
		"the side emptied":         {text: "", lines: noteTarget{File: -1, From: 9, To: 9}, lost: sideNotShown},
		"the file gone":            {text: "b\n", files: b, lines: noteTarget{File: -1, From: 9, To: 9}, lost: sideNotShown},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m, _ := fieldAtLine10(t)
			fs := m.s.Files
			if tt.files != nil {
				fs = tt.files(m)
			}

			m = changedTo(t, m, tt.text, fs...)

			want := tt.lines
			want.Side = filediff.NewSide
			assert.Equal(t, want, m.s.Notes.Draft.Lines)
			assert.Equal(t, tt.lost, m.s.Notes.Draft.Lost)
			assert.Equal(t, "draft", m.s.Field.Text, "the draft stays whatever comes")
			if tt.lost == linesFound {
				assert.True(t, m.s.gapOnScreen(m.s.gap()), "the box in sight")
				if tt.footer == "" {
					assert.NotContains(t, footerLine(m), "lines noted")
				}
				assert.True(t, strings.HasPrefix(footerLine(m), tt.footer), footerLine(m))
				return
			}
			assert.False(t, m.s.drafting(0, filediff.NewSide, want.To), "no line marked")
			assert.True(t, strings.HasPrefix(footerLine(m), tt.lost.String()), footerLine(m))
			if want.File < 0 {
				assert.False(t, m.s.gap().OK, "no box in another file")
				assert.Contains(t, footerLine(m), "esc discard", "the keys, as the field shows nowhere")
			}
		})
	}
}

func TestTheFooterTellsOfTheLinesAsTheyAreNow(t *testing.T) {
	tests := map[string]struct {
		before, text, want string
	}{
		"found, no longer likely":      {probablyHere, insertedAbove("x\n"), ""},
		"lost, no longer likely":       {probablyHere, withoutLinesFrom5(12), linesNotFound.String()},
		"lost while saving":            {"saving the note…", withoutLinesFrom5(12), linesNotFound.String()},
		"found, saving still told":     {"saving the note…", insertedAbove("x\n"), "saving the note…"},
		"changed, saving still told":   {"saving the note…", strings.Replace(noteNew(), "\n9\n", "\nnine\n", 1), "saving the note…"},
		"not shown, saving still told": {"saving the note…", "", "saving the note…"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m, _ := fieldAtLine10(t)
			m.s.Note = tt.before

			m = changedTo(t, m, tt.text, m.s.Files...)

			assert.NotContains(t, footerLine(m), probablyHere)
			assert.True(t, strings.HasPrefix(footerLine(m), tt.want), footerLine(m))
		})
	}
}

func TestLinesNotFoundSaySoAndKeepTheirAnchor(t *testing.T) {
	m, f := fieldAtLine10(t)
	f.writer.AddReturns(4)

	m = changedTo(t, m, withoutLinesFrom5(12), m.s.Files...)

	assert.True(t, strings.HasPrefix(footerLine(m), linesNotFound.String()), "while the field shows its keys nowhere else")
	m, _ = typed(m, "!")
	assert.True(t, strings.HasPrefix(footerLine(m), linesNotFound.String()), "says so while it is so")
	g := m.s.gap()
	require.True(t, g.OK)
	top := plainLines(&m.s)[bodyTop+g.At-m.s.top()+1]
	assert.Contains(t, top, " L? ─┐", "the top edge names no line")
	assert.NotContains(t, top, noteHook, "nor hangs from one")

	m, _ = typed(m, "enter")
	require.Equal(t, 1, f.writer.AddCallCount())
	assert.Equal(t, 10, f.writer.AddArgsForCall(0).Anchor.Start, "the note goes to the lines meant")
}

func TestLinesFoundAgainAreMarkedAgain(t *testing.T) {
	_, text := noteText()
	for name, gone := range map[string]string{"lines deleted": withoutLinesFrom5(12), "side emptied": ""} {
		t.Run(name, func(t *testing.T) {
			m, _ := fieldAtLine10(t)
			m = changedTo(t, m, gone, m.s.Files...)
			require.NotEqual(t, linesFound, m.s.Notes.Draft.Lost)

			m = changedTo(t, m, text, m.s.Files...)

			assert.Equal(t, linesFound, m.s.Notes.Draft.Lost)
			assert.Equal(t, noteTarget{File: 0, Side: filediff.NewSide, From: 9, To: 9}, m.s.Notes.Draft.Lines)
			assert.True(t, m.s.drafting(0, filediff.NewSide, 9))
		})
	}
}

func TestAFieldOfAFileGoneStaysWithNoneThroughTheNextReload(t *testing.T) {
	m, _ := fieldAtLine10(t)
	b := m.s.Files[1]
	m = changedTo(t, m, "b\n", b)

	m = changedTo(t, m, "b\n", b)

	assert.Equal(t, -1, m.s.Notes.Draft.Lines.File, "not at the file that took its place")
	assert.False(t, m.s.gap().OK)
}

func TestAFieldThatEditsFollowsItsNote(t *testing.T) {
	_, text := noteText()
	a, err := note.NewAnchor("a.go", worktreeOrigin(), worktreeSide().Blob, strings.Split(strings.TrimSuffix(text, "\n"), "\n"), 10, 10)
	require.NoError(t, err)
	n := note.Note{ID: "e", Kind: note.Finding, Anchor: a, Text: "was", State: note.Open}
	m, _ := placedAt(t, note.Spot{Note: n, Place: note.Placement{Where: note.Exact, Start: 10, End: 10}})
	m, _ = typed(m, toLine10("E")...)
	require.Equal(t, note.ID("e"), m.s.Notes.Draft.Edit)

	m = changedTo(t, m, insertedAbove("x\n", "y\n", "z\n"), m.s.Files...)

	assert.Equal(t, noteTarget{File: 0, Side: filediff.NewSide, From: 12, To: 12}, m.s.Notes.Draft.Lines)
	assert.Equal(t, "was", m.s.Field.Text)
}

// streamAnswers hands the stream of m a.go with text as its worktree side,
// b.go as it was.
func streamAnswers(t *testing.T, m model, text string) model {
	t.Helper()
	sl := m.p.Stream.(*infakes.FakeStreamLoader)
	var us []filediff.Update
	for _, r := range sl.WantArgsForCall(sl.WantCallCount() - 1) {
		body := text
		if r.File.Path() != "a.go" {
			_, body = noteText()
		}
		us = append(us, filediff.Update{Gen: r.Gen, File: r.File, Snapshot: noteSnapshot(t, r.File, body)})
	}
	sl.DrainReturns(us)
	m, _ = send(m, streamReady{})
	return m
}

func TestTheFieldFollowsItsLinesInTheStream(t *testing.T) {
	m, _ := noteModel(t, headToWorktree(t))
	// a.go goes on at the top as it was loaded, too long for b.go to come
	// into the window.
	m, _ = typed(m, "s")
	require.True(t, m.s.Stream.On)
	m, _ = typed(m, toLine10("c", "d")...)
	require.True(t, m.s.Field.Open)
	require.Equal(t, noteTarget{File: 0, Side: filediff.NewSide, From: 9, To: 9}, m.s.Notes.Draft.Lines)

	m = reloadedTo(m, append(files("0.go"), m.s.Files...)...)
	assert.Equal(t, 1, m.s.Notes.Draft.Lines.File, "with its file while it loads afresh")
	m = streamAnswers(t, m, insertedAbove("x\n", "y\n", "z\n"))

	assert.Equal(t, noteTarget{File: 1, Side: filediff.NewSide, From: 12, To: 12}, m.s.Notes.Draft.Lines, "a.go comes second now, after 0.go")
	assert.Equal(t, "d", m.s.Field.Text)
}

func TestAStreamReloadedWithoutFilesAndBackKeepsGoing(t *testing.T) {
	m, _ := noteModel(t, headToWorktree(t))
	m, _ = typed(m, "s")
	fs := m.s.Files

	m = reloadedTo(m)
	m = reloadedTo(m, fs...)

	assert.Len(t, m.s.Files, 2)
	assert.Zero(t, m.s.Notes.Draft, "no field, no draft to move")
}

func TestAFieldWithItsLinesLostAsksAboutTheLinesNoted(t *testing.T) {
	var s state
	s.Notes.Draft = noteDraft{Kind: note.Question, Lines: noteTarget{From: 4, To: 4}, Lost: linesNotFound}
	assert.Equal(t, "Ask about the lines noted…", s.notePlaceholder())
}

func TestAReloadLeavesTheCodeTheWheelWentTo(t *testing.T) {
	m, _ := fieldAtLine10(t)
	for range 10 {
		m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 40, Y: bodyTop + 2})
	}
	top := m.s.top()
	require.True(t, m.s.Notes.Draft.Away)

	m = changedTo(t, m, insertedAbove("x\n"), m.s.Files...)

	assert.Equal(t, top, m.s.top())
	assert.Equal(t, 10, m.s.Notes.Draft.Lines.To, "though the field followed")

	m, _ = typed(m, "!")
	assert.False(t, m.s.Notes.Draft.Away, "typing brings it back")
}

func TestAFieldFollowedIntoAFoldOpensIt(t *testing.T) {
	m, _ := noteModel(t, headToWorktree(t))
	m, _ = typed(m, "f", "i", "j", "j", "j", "j", "c", "x")
	require.Equal(t, 4, m.s.Notes.Draft.Lines.To)
	old, new := noteText()

	// Line 3 as it was and one more at the end: line 5 is far from any change.
	m = changedTo(t, m, old+"80\n", m.s.Files...)
	assert.True(t, m.s.gapOnScreen(m.s.gap()), "not folded away")

	// Back, and then the same version again, which lays out its folds anew.
	m = changedTo(t, m, new, m.s.Files...)
	m = changedTo(t, m, old+"80\n", m.s.Files...)
	m = changedTo(t, m, old+"80\n", m.s.Files...)
	assert.True(t, m.s.gap().OK, "nor in the same version")
}
