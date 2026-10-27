package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/in/infakes"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/hover"
)

// diskModel shows the file path of a comparison of files on disk, which
// the server knows as docPath in the worktree.
func diskModel(t *testing.T, path, docPath string) (model, *infakes.FakeFileDiffLoader, *infakes.FakeDefiner) {
	t.Helper()
	m, fl, df := defineModelOf(t, change.Comparison{Left: change.File("/tmp/git-blob/x_a.go"), Right: change.File(path)}, docPath, []string{path})
	m.s.Files[0].WorktreePath = docPath
	return m, fl, df
}

func TestADefinitionFromAFileOnDiskVisitsTheWorktree(t *testing.T) {
	m, fl, df := diskModel(t, "a.go", "a.go")
	m, _ = typed(m, "d")

	answer(m, definedAt(df.DefineArgsForCall(0).Req, hover.Location{Path: "lib/d.go", Line: 5}, 1))

	r := fl.LoadArgsForCall(fl.LoadCallCount() - 1)
	assert.Equal(t, change.ChangedFile{NewPath: "lib/d.go", NewMode: 0o100644}, r.File)
	assert.Equal(t, change.KindWorktree, r.Comparison.Right.Kind(), "read from the worktree, not below the file on disk")
}

func TestADefinitionInAFileOnDiskGivenByAnotherPathScrolls(t *testing.T) {
	m, fl, df := diskModel(t, "./a.go", "sub/a.go")
	m, _ = typed(m, "d")
	loads := fl.LoadCallCount()

	m = answer(m, definedAt(df.DefineArgsForCall(0).Req, hover.Location{Path: "sub/a.go", Line: 20}, 1))

	assert.Equal(t, loads, fl.LoadCallCount(), "the file shown, known by its place in the worktree")
	assert.Equal(t, 20-targetContext, topRow(m))
}

func TestADefinitionInAWorktreeFileNamedLikeTheFileOnDiskVisitsIt(t *testing.T) {
	m, fl, df := diskModel(t, "a.go", "sub/a.go")
	m, _ = typed(m, "d")
	loads := fl.LoadCallCount()

	m = answer(m, definedAt(df.DefineArgsForCall(0).Req, hover.Location{Path: "a.go", Line: 5}, 1))

	require.Equal(t, loads+1, fl.LoadCallCount(), "the root's a.go is another file than sub/a.go")
	r := fl.LoadArgsForCall(loads)
	assert.Equal(t, "a.go", r.File.NewPath)
	assert.Equal(t, change.KindWorktree, r.Comparison.Right.Kind())

	m.s.reloaded(change.Update{Gen: m.s.reload(), Changeset: change.Changeset{Comparison: m.s.Comparison, Files: m.s.Files}})
	assert.True(t, m.s.visiting(), "a reload keeps the visit rather than taking it for the file on disk")
}

// diskChangeset starts a model on a.go of a comparison of files on disk,
// known to the server as sub/a.go.
func diskChangeset(t *testing.T) (model, *infakes.FakeFileDiffLoader, *infakes.FakeChangesetReloader) {
	t.Helper()
	p := fakePorts()
	c := change.Comparison{Left: change.File("/tmp/git-blob/x_a.go"), Right: change.File("a.go")}
	file := change.ChangedFile{Status: change.Modified, OldPath: "a.go", NewPath: "a.go", WorktreePath: "sub/a.go"}
	m := newModel(change.Changeset{Comparison: c, Files: []change.ChangedFile{file}}, p, splitSettings())
	m, _ = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return m, p.Files.(*infakes.FakeFileDiffLoader), p.Reload.(*infakes.FakeChangesetReloader)
}

func TestAFileOnDiskWithoutTextIsUntrackedByItsPlaceInTheWorktree(t *testing.T) {
	m, fl, _ := diskChangeset(t)
	fl.DrainReturns([]filediff.Update{snapshotFor(t, fl.LoadArgsForCall(0))})

	m, _ = send(m, fileDiffsReady{})

	docs := m.p.Docs.(*infakes.FakeDocumentTracker)
	require.Equal(t, 1, docs.UntrackCallCount())
	assert.Equal(t, hover.DocKey{Path: "sub/a.go"}, docs.UntrackArgsForCall(0))
}

func TestAFileOnDiskGoneIsUntrackedByItsPlaceInTheWorktree(t *testing.T) {
	m, _, rl := diskChangeset(t)
	m, _ = key(m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	rl.DrainReturns([]change.Update{{Gen: rl.ReloadArgsForCall(0), Changeset: change.Changeset{Comparison: m.s.Comparison}}})

	m, _ = send(m, changesetReady{})

	docs := m.p.Docs.(*infakes.FakeDocumentTracker)
	require.Equal(t, 1, docs.UntrackCallCount())
	assert.Equal(t, hover.DocKey{Path: "sub/a.go"}, docs.UntrackArgsForCall(0))
}
