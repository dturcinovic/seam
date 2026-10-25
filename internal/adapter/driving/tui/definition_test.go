package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/in/infakes"
	"seam/internal/domain/change"
	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
	"seam/internal/domain/view"
)

// sixtyRows is a worktree file of 60 lines with lines 10 and 40 changed,
// as the right side of a.go, b.go and c.go.
func sixtyRows(t *testing.T, path string) filediff.Snapshot {
	t.Helper()
	text := numbered(60, 10, 40)
	return filediff.Snapshot{
		Diff:     diff.Compute(t.Context(), diff.Split(numbered(60)), diff.Split(text)),
		RightDoc: hover.Document{Path: path, Lang: lang.Go, Version: 1, Text: text},
	}
}

// defineModel shows a.go of a.go, b.go and c.go with the caret on its
// first text, and answers definitions Sent.
func defineModel(t *testing.T) (model, *infakes.FakeFileDiffLoader, *infakes.FakeDefiner) {
	t.Helper()
	m, fl, _ := startModel(t, "a.go", "b.go", "c.go")
	m.s.Comparison = change.Comparison{Left: change.Commit("HEAD"), Right: change.Worktree()}
	m.p.Docs.(*infakes.FakeDocumentTracker).TrackReturns(gopls(), true)
	df := m.p.Define.(*infakes.FakeDefiner)
	df.DefineReturns(hover.Sent)
	m = answerLoad(t, m, fl, sixtyRows(t, "a.go"))
	m, _ = typed(m, "i")
	require.True(t, m.s.Caret.On)
	return m, fl, df
}

// answerLoad answers the last load with snap.
func answerLoad(t *testing.T, m model, fl *infakes.FakeFileDiffLoader, snap filediff.Snapshot) model {
	t.Helper()
	r := fl.LoadArgsForCall(fl.LoadCallCount() - 1)
	snap.File = r.File
	fl.DrainReturns([]filediff.Update{{Gen: r.Gen, File: r.File, Snapshot: snap}})
	m, _ = send(m, fileDiffsReady{})
	return m
}

func definedAt(req hover.ReqID, l hover.Location, targets int) hover.Update {
	return hover.Update{Kind: hover.DefinitionDone, Req: req, Version: 1, Outcome: hover.Text, Target: l, Targets: targets}
}

func ctrlO() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl} }

// topRow is the row of the top line of the single view.
func topRow(m model) int { return m.s.Lines[m.s.Pos.Scroll].Rows.Start }

func TestDAsksWhereTheWordUnderTheCaretIsDefined(t *testing.T) {
	m, _, df := defineModel(t)

	typed(m, "d")

	require.Equal(t, 1, df.DefineCallCount())
	p := df.DefineArgsForCall(0)
	assert.Equal(t, "a.go", p.Path)
	assert.Equal(t, m.s.Caret.Row, p.Line)
	assert.Equal(t, m.s.Caret.Byte, p.Byte)
}

func TestCtrlClickAsksWhereTheWordUnderTheMouseIsDefined(t *testing.T) {
	m, _, df := defineModel(t)
	// The top line is a fold; below it the first text.
	x, y := at(m, 0)
	y += m.s.rowIndex(m.s.Current, 7) - m.s.top()

	send(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft, Mod: tea.ModCtrl})

	require.Equal(t, 1, df.DefineCallCount())
	assert.Equal(t, "a.go", df.DefineArgsForCall(0).Path)
}

func TestADefinitionInTheChangesetOpensInFull(t *testing.T) {
	m, fl, df := defineModel(t)
	m, _ = typed(m, "d")
	loads := fl.LoadCallCount()

	m = answer(m, definedAt(df.DefineArgsForCall(0).Req, hover.Location{Path: "b.go", Line: 30}, 1))
	require.Equal(t, loads+1, fl.LoadCallCount())
	assert.Equal(t, "b.go", fl.LoadArgsForCall(loads).File.Path())
	m = answerLoad(t, m, fl, sixtyRows(t, "b.go"))

	assert.Equal(t, view.Full, m.s.Mode)
	assert.Equal(t, 30-targetContext, topRow(m), "with three rows above")

	m, _ = send(m, ctrlO())
	require.Equal(t, loads+2, fl.LoadCallCount())
	assert.Equal(t, "a.go", fl.LoadArgsForCall(loads+1).File.Path(), "ctrl+o goes back")
	m = answerLoad(t, m, fl, sixtyRows(t, "a.go"))
	assert.Equal(t, view.Hunks, m.s.Mode, "in the view left")
}

func TestADefinitionInTheSameFileScrolls(t *testing.T) {
	m, fl, df := defineModel(t)
	m, _ = typed(m, "d")
	loads := fl.LoadCallCount()

	m = answer(m, definedAt(df.DefineArgsForCall(0).Req, hover.Location{Path: "a.go", Line: 20}, 3))

	assert.Equal(t, loads, fl.LoadCallCount(), "nothing to load")
	assert.Equal(t, 20-targetContext, topRow(m))
	assert.Equal(t, "definition 1 of 3", footerLine(m))
}

func TestADefinitionOutsideTheChangesetIsReadOnly(t *testing.T) {
	m, fl, df := defineModel(t)
	m, _ = typed(m, "d")

	m = answer(m, definedAt(df.DefineArgsForCall(0).Req, hover.Location{Path: "lib/d.go", Line: 5}, 1))
	r := fl.LoadArgsForCall(fl.LoadCallCount() - 1)
	assert.Equal(t, change.ChangedFile{NewPath: "lib/d.go", NewMode: 0o100644}, r.File)
	snap := sixtyRows(t, "lib/d.go")
	snap.Diff = diff.Compute(t.Context(), diff.Split(""), diff.Split(numbered(60)))
	m = answerLoad(t, m, fl, snap)

	lines := plainLines(&m.s)
	assert.Equal(t, "visit  lib/d.go  not changed", strings.TrimSpace(lines[0]))
	assert.NotContains(t, strings.Join(lines[1:len(lines)-1], "\n"), "+", "no line shows as inserted")
	assert.True(t, m.s.Layout.Unified, "one side only")
	assert.Equal(t, 5-targetContext, topRow(m))
}

func TestADefinitionOutsideTheRepositoryIsNamed(t *testing.T) {
	m, fl, df := defineModel(t)
	m, _ = typed(m, "d")
	loads := fl.LoadCallCount()

	m = answer(m, definedAt(df.DefineArgsForCall(0).Req, hover.Location{Path: "/usr/lib/go/src/fmt/print.go", Outside: true, Line: 41}, 1))

	assert.Equal(t, loads, fl.LoadCallCount())
	assert.Equal(t, "definition in /usr/lib/go/src/fmt/print.go:42", footerLine(m))
	m, _ = typed(m, "j")
	assert.NotContains(t, footerLine(m), "print.go", "until the next key")
}

func TestNoDefinitionSaysSo(t *testing.T) {
	m, _, df := defineModel(t)
	m, _ = typed(m, "d")

	m = answer(m, hover.Update{Kind: hover.DefinitionDone, Req: df.DefineArgsForCall(0).Req, Outcome: hover.Empty})

	assert.Equal(t, "no definition found", footerLine(m))
}

func TestADefinitionWaitsForItsServer(t *testing.T) {
	m, _, df := defineModel(t)
	df.DefineReturnsOnCall(0, hover.NotReady)
	m, _ = typed(m, "d")

	answer(m, hover.Update{Kind: hover.StatusChanged, Instance: gopls(), Status: hover.Ready})

	require.Equal(t, 2, df.DefineCallCount())
	first, again := df.DefineArgsForCall(0), df.DefineArgsForCall(1)
	assert.NotEqual(t, first.Req, again.Req)
	first.Req = again.Req
	assert.Equal(t, first, again)
}

func TestTheStreamJumpsWithinItself(t *testing.T) {
	m, fl, df := defineModel(t)
	m, _ = typed(m, "i", "s")
	sl := m.p.Stream.(*infakes.FakeStreamLoader)
	var us []filediff.Update
	for _, r := range sl.WantArgsForCall(sl.WantCallCount() - 1) {
		snap := sixtyRows(t, r.File.Path())
		snap.File = r.File
		us = append(us, filediff.Update{Gen: r.Gen, File: r.File, Snapshot: snap})
	}
	sl.DrainReturns(us)
	m, _ = send(m, streamReady{})
	before := m.s.Stream.Pos
	m, _ = typed(m, "i", "d")
	loads := fl.LoadCallCount()

	// Line 25 lies in the fold between the changes at 10 and 40.
	m = answer(m, definedAt(df.DefineArgsForCall(0).Req, hover.Location{Path: "b.go", Line: 25}, 1))

	require.True(t, m.s.Stream.On, "still the stream")
	assert.Equal(t, loads, fl.LoadCallCount())
	top := m.s.Stream.View.At(m.s.top())
	assert.Equal(t, 1, top.File)
	_, l, _ := m.s.rowLine(m.s.top())
	assert.False(t, l.Fold)
	assert.Equal(t, 25-targetContext, l.Rows.Start, "the row, unfolded")
	m, _ = send(m, ctrlO())
	assert.Equal(t, before.File, m.s.Stream.Pos.File)
	assert.Equal(t, before.Scroll, m.s.Stream.Pos.Scroll)
}

func TestTheJumpsBackAreBounded(t *testing.T) {
	m, _, _ := defineModel(t)
	for i := range jumpHistory + 5 {
		m.s.Jump.Target = nil
		m.s.remember()
		m.s.Jump.Back[len(m.s.Jump.Back)-1].Path = fmt.Sprint(i)
	}

	assert.Len(t, m.s.Jump.Back, jumpHistory)
	assert.Equal(t, "5", m.s.Jump.Back[0].Path, "the oldest fell out")
}

// visited is defineModel visiting lib/d.go, loaded, with the caret on.
func visited(t *testing.T) (model, *infakes.FakeFileDiffLoader, *infakes.FakeDefiner) {
	t.Helper()
	m, fl, df := defineModel(t)
	m, _ = typed(m, "d")
	m = answer(m, definedAt(df.DefineArgsForCall(df.DefineCallCount()-1).Req, hover.Location{Path: "lib/d.go", Line: 5}, 1))
	snap := sixtyRows(t, "lib/d.go")
	snap.Diff = diff.Compute(t.Context(), diff.Split(""), diff.Split(numbered(60)))
	m = answerLoad(t, m, fl, snap)
	require.True(t, m.s.visiting())
	return m, fl, df
}

func TestBackFromTheStreamLoadsTheFileLeft(t *testing.T) {
	m, fl, df := defineModel(t)
	m, _ = typed(m, "d")
	m = answer(m, definedAt(df.DefineArgsForCall(0).Req, hover.Location{Path: "b.go", Line: 30}, 1))
	m = answerLoad(t, m, fl, sixtyRows(t, "b.go"))
	m, _ = typed(m, "s")
	m.s.streamJump(0)
	loads := fl.LoadCallCount()

	m, _ = send(m, ctrlO())

	require.Equal(t, loads+1, fl.LoadCallCount(), "b.go's snapshot does not pass for a.go")
	assert.Equal(t, "a.go", fl.LoadArgsForCall(loads).File.Path())
	assert.False(t, m.s.Loaded)
}

func TestLeavingTheStreamByAJumpEndsItsLoads(t *testing.T) {
	m, _, df := defineModel(t)
	m, _ = typed(m, "i", "s", "i", "d")
	sl := m.p.Stream.(*infakes.FakeStreamLoader)

	m = answer(m, definedAt(df.DefineArgsForCall(df.DefineCallCount()-1).Req, hover.Location{Path: "lib/d.go", Line: 5}, 1))

	require.False(t, m.s.Stream.On)
	assert.Empty(t, sl.WantArgsForCall(sl.WantCallCount()-1))
}

func TestMovingOnForgetsTheJumpWaitedFor(t *testing.T) {
	m, fl, df := defineModel(t)
	df.DefineReturnsOnCall(0, hover.NotReady)
	m, _ = typed(m, "d", "j")
	answer(m, hover.Update{Kind: hover.StatusChanged, Instance: gopls(), Status: hover.Ready})
	assert.Equal(t, 1, df.DefineCallCount(), "not asked again")

	m, _ = typed(m, "d")
	m = answer(m, definedAt(df.DefineArgsForCall(1).Req, hover.Location{Path: "b.go", Line: 30}, 1))
	m, _ = typed(m, "k")
	m = answerLoad(t, m, fl, sixtyRows(t, "b.go"))
	assert.Equal(t, 0, topRow(m), "the row is not brought up after the user moved on")
}

func TestARetrySentClearsTheNote(t *testing.T) {
	m, _, df := defineModel(t)
	df.DefineReturnsOnCall(0, hover.NotReady)
	m, _ = typed(m, "d")
	require.Contains(t, footerLine(m), "starting")

	m = answer(m, hover.Update{Kind: hover.StatusChanged, Instance: gopls(), Status: hover.Ready})
	assert.NotContains(t, footerLine(m), "starting")
	m = answer(m, definedAt(df.DefineArgsForCall(1).Req, hover.Location{Path: "a.go", Line: 20}, 3))
	assert.True(t, strings.HasPrefix(footerLine(m), "definition 1 of 3"), footerLine(m))
}

func TestAVisitedFileThatChangesJoinsTheChangeset(t *testing.T) {
	m, _, _ := visited(t)
	fs := append(files("a.go", "b.go", "c.go"), files("lib/d.go")...)

	m.s.reloaded(change.Update{Gen: m.s.reload(), Changeset: change.Changeset{Comparison: m.s.Comparison, Files: fs}})

	assert.False(t, m.s.visiting())
	f, _ := m.s.file()
	assert.Equal(t, change.Modified, f.Status)
}

func TestTheStreamFromAVisitStartsAtTheTop(t *testing.T) {
	m, _, _ := visited(t)
	m.s.scrollTo(30)

	m, _ = typed(m, "i", "s")

	assert.NotContains(t, m.s.Stream.Held, -1)
	assert.Equal(t, view.StreamLine{File: 0}, m.s.Stream.View.At(m.s.top()), "at the header of the first file")
}

func TestADefinitionWithinTheFileVisitedScrolls(t *testing.T) {
	m, fl, df := visited(t)
	loads, asked := fl.LoadCallCount(), df.DefineCallCount()

	m, _ = typed(m, "i", "d")
	require.Equal(t, asked+1, df.DefineCallCount())
	m = answer(m, definedAt(df.DefineArgsForCall(asked).Req, hover.Location{Path: "lib/d.go", Line: 30}, 1))

	assert.Equal(t, loads, fl.LoadCallCount(), "not read again")
	assert.Equal(t, 30-targetContext, topRow(m))
}
