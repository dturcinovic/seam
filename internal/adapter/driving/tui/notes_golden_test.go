package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/domain/ui"
)

// notedState is the notes' file of HEAD against the worktree in the full
// view, with a question over two lines, a finding only likely placed and
// one that lost its place, and in the book open notes on it and b.go.
func notedState(t *testing.T, set ui.Settings, width int) *state {
	t.Helper()
	old, new := noteText()
	s := shownWith(t, set, listFiles(), width, 16, filediff.Snapshot{
		Diff: diff.Compute(t.Context(), diff.Split(old), diff.Split(new)), WorktreeBlob: change.BlobID(change.SHA1, []byte(new)),
	})
	s.toggleMode()
	s.Comparison = headToWorktree(t)
	k := worktreeSide()
	k.Path = s.Snapshot.File.NewPath
	s.Notes.Spots = map[note.SnapshotKey][]note.Spot{k: {
		spotAt("q", note.Question, note.Exact, 3, 4),
		spotAt("f", note.Finding, note.Likely, 6, 6),
		spotAt("o", note.Question, note.Orphaned, 0, 0),
	}}
	status := note.Update{Kind: note.StoreStatus, Live: true}
	for _, h := range []struct{ id, path string }{{"q", k.Path}, {"f", k.Path}, {"n", "cmd/main.go"}} {
		status.Heads = append(status.Heads, note.Head{ID: note.ID(h.id), Path: h.path, Origin: k.Origin, Kind: note.Question, State: note.Open})
	}
	s.Notes.status(status)
	return s
}

// noteFixtures are the marks of notes side by side and unified, with the
// counts of the list, the popup of a note, the field writing one, the
// overview and the plus offered for a new one.
func noteFixtures(t *testing.T) []fixture {
	t.Helper()
	split := notedState(t, splitSettings(), 140)
	unified := notedState(t, ui.DefaultSettings(), 100)
	popup := notedState(t, splitSettings(), 140)
	popup.openNotes(noteLine{File: 0, Side: filediff.NewSide, Line: 2}, 80, bodyTop+2)
	field := notedState(t, splitSettings(), 140)
	field.openField(note.MaxText, 80, bodyTop+2)
	field.Field.Title, field.Field.Hint = note.Finding.String(), "tab question or finding"
	field.fieldPaste("Is this error dropped on purpose?")
	overview := notedState(t, splitSettings(), 140)
	overview.Notes.Overview = noteOverview{Open: true}
	overview.indexSides()
	k := worktreeSide()
	k.Path = overview.Snapshot.File.NewPath
	resolved := bookNote("r", "cmd/main.go", k.Origin, 9, 4)
	resolved.State = note.Resolved
	book := []note.Note{resolved, bookNote("n", "cmd/main.go", k.Origin, 3, 5), bookNote("i", "cmd/main.go", note.Origin{Kind: change.KindIndex}, 3, 6)}
	for _, sp := range overview.Notes.Spots[k] {
		n := sp.Note
		n.Anchor = note.Anchor{Path: k.Path, Origin: k.Origin}
		book = append(book, n)
	}
	overview.takeOverview(note.Update{Kind: note.Overview, Notes: book})
	plusSplit := notedState(t, splitSettings(), 140)
	plusSplit.Hover.Plus, plusSplit.Hover.PlusOn = noteLine{Side: filediff.NewSide, Line: 0}, true
	plusUnified := notedState(t, ui.DefaultSettings(), 100)
	plusUnified.Hover.Plus, plusUnified.Hover.PlusOn = noteLine{Side: filediff.NewSide, Line: 0}, true
	return []fixture{
		{"notes-split", split}, {"notes-unified", unified}, {"notes-popup", popup}, {"notes-field", field}, {"notes-overview", overview},
		{"notes-plus-split", plusSplit}, {"notes-plus-unified", plusUnified},
	}
}

func TestTheGoldenNotesShowTheirCases(t *testing.T) {
	byName := map[string][]string{}
	for _, f := range noteFixtures(t) {
		byName[f.name] = plainLines(f.state)
	}
	split := byName["notes-split"]
	require.Greater(t, len(split), bodyTop+6)
	assert.Contains(t, split[bodyTop+2], "│ ?  3   2", "the mark stands before the line number")
	assert.Contains(t, split[bodyTop+3], "│ ╎  4 ~ changed 3")
	assert.Contains(t, split[bodyTop+5], "│ !  6   5")
	assert.Contains(t, split[bodyTop+1], " 2 M logo.png")
	assert.Contains(t, split[bodyTop+5], " 1 M main.go")
	assert.Contains(t, split[len(split)-1], "notes 3 open, 1 not placed")
	unified := byName["notes-unified"]
	assert.Contains(t, unified[bodyTop+2], " ?  3  3   2", "a whole row shows the notes of either side")
	assert.Contains(t, unified[bodyTop+4], " ╎     4 + changed 3")
	assert.Contains(t, strings.Join(byName["notes-popup"], "\n"), "┌ notes ")
	assert.Contains(t, strings.Join(byName["notes-field"], "\n"), "┌ finding ")
	assert.Contains(t, byName["notes-field"][len(byName["notes-field"])-1], "tab question or finding")
	assert.Contains(t, strings.Join(byName["notes-overview"], "\n"), "┌ notes ")
	assert.Contains(t, byName["notes-plus-split"][bodyTop], "│[+] 1   0", "the plus stands where a mark would")
	assert.Contains(t, byName["notes-plus-unified"][bodyTop], "[+] 1  1   0")
	assert.Contains(t, strings.Join(byName["notes-field"], "\n"), "└ enter done  alt+enter new line  esc discard  tab question or finding ─")
}
