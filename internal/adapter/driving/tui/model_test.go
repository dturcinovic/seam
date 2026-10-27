package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/dturcinovic/seam/internal/app/port/in/infakes"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fakePorts are fakes of every port the TUI calls.
func fakePorts() Ports {
	return Ports{
		Files: &infakes.FakeFileDiffLoader{}, Stream: &infakes.FakeStreamLoader{}, Reload: &infakes.FakeChangesetReloader{},
		Docs: &infakes.FakeDocumentTracker{}, LSP: &infakes.FakeLSPUpdates{}, Hover: &infakes.FakeHoverer{}, Define: &infakes.FakeDefiner{},
		Watch:      &infakes.FakeWatchUpdates{},
		NoteWriter: &infakes.FakeNoteWriter{}, NoteReader: &infakes.FakeNoteReader{}, NoteOverview: &infakes.FakeNoteOverview{},
		NoteReleaser: &infakes.FakeNoteReleaser{}, NoteSender: &infakes.FakeNoteSender{}, NoteUpdates: &infakes.FakeNoteUpdates{},
	}
}

func startModel(t *testing.T, paths ...string) (model, *infakes.FakeFileDiffLoader, *infakes.FakeChangesetReloader) {
	t.Helper()
	p := fakePorts()
	m := newModel(change.Changeset{Files: files(paths...)}, p, splitSettings())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(model), p.Files.(*infakes.FakeFileDiffLoader), p.Reload.(*infakes.FakeChangesetReloader)
}

func key(m model, k tea.KeyPressMsg) (model, tea.Cmd) {
	return send(m, k)
}

// send is m after it took msg, and the command it gave.
func send(m model, msg tea.Msg) (model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(model), cmd
}

func snapshotFor(t *testing.T, r filediff.Request) filediff.Update {
	t.Helper()
	d := diff.Compute(t.Context(), diff.Split("a\n"), diff.Split("b\n"))
	return filediff.Update{Gen: r.Gen, Snapshot: filediff.Snapshot{File: r.File, Diff: d}}
}

func TestTheFirstFileIsAskedForAtOnce(t *testing.T) {
	_, fl, _ := startModel(t, "a.go", "b.go")

	require.Equal(t, 1, fl.LoadCallCount())
	r := fl.LoadArgsForCall(0)
	assert.Equal(t, "a.go", r.File.Path())
	assert.False(t, r.Fresh)
}

func TestOnlyTheLastRequestIsShown(t *testing.T) {
	m, fl, _ := startModel(t, "a.go", "b.go")
	m, _ = key(m, tea.KeyPressMsg{Code: tea.KeyTab})
	require.Equal(t, 2, fl.LoadCallCount())
	first, second := fl.LoadArgsForCall(0), fl.LoadArgsForCall(1)
	fl.DrainReturns([]filediff.Update{snapshotFor(t, second), snapshotFor(t, first)})

	next, _ := m.Update(fileDiffsReady{})

	s := next.(model).s
	require.True(t, s.Loaded)
	assert.Equal(t, "b.go", s.Snapshot.File.Path(), "a's late answer does not cover b")
}

func TestReloadThenLoadFresh(t *testing.T) {
	m, fl, rl := startModel(t, "a.go", "b.go")
	m, _ = key(m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	require.Equal(t, 1, rl.ReloadCallCount())
	rl.DrainReturns([]change.Update{{Gen: rl.ReloadArgsForCall(0), Changeset: change.Changeset{Files: files("b.go")}}})

	m.Update(changesetReady{})

	require.Equal(t, 2, fl.LoadCallCount())
	r := fl.LoadArgsForCall(1)
	assert.Equal(t, "b.go", r.File.Path(), "a is gone, b takes its place")
	assert.True(t, r.Fresh)
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{{Code: 'q', Text: "q"}, {Code: tea.KeyEscape}, {Code: 'c', Mod: tea.ModCtrl}} {
		m, _, _ := startModel(t, "a.go")
		_, cmd := key(m, k)
		require.NotNil(t, cmd, k.String())
		assert.Equal(t, tea.Quit(), cmd(), k.String())
	}
}

func TestNavigationKeysCallNoPort(t *testing.T) {
	m, fl, rl := startModel(t, "a.go")
	fl.DrainReturns([]filediff.Update{snapshotFor(t, fl.LoadArgsForCall(0))})
	next, _ := m.Update(fileDiffsReady{})
	m = next.(model)

	for _, k := range []string{"j", "k", "g", "G", "n", "p", "f", "h", "l"} {
		m, _ = key(m, tea.KeyPressMsg{Code: rune(k[0]), Text: k})
	}

	assert.Equal(t, 1, fl.LoadCallCount())
	assert.Zero(t, rl.ReloadCallCount())
}
