package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/adapter/driving/tui"
	"seam/internal/app/port/in"
	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
	"seam/internal/testsupport"
)

// TestAHoverOnTheOldSideIsAnsweredFromACheckout asks seam's graph, with
// git and fakelsp, through the ports the TUI calls, about HEAD's side of a
// changed file: the first hover checks HEAD out, a server starts in the
// checkout and answers, and the stop removes the checkout again.
func TestAHoverOnTheOldSideIsAnsweredFromACheckout(t *testing.T) {
	fakelsp := buildFakelsp(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := testsupport.NewGitRepo(t)
	repo.Write("a.go", "package old\n")
	// In the commit, so the server in the checkout finds it.
	repo.Write("fakelsp.script", "hover func old()\n")
	repo.Git("add", ".")
	repo.Git("commit", "-q", "-m", "first")
	repo.Write("a.go", "package now\n")
	var (
		ports      tui.Ports
		changesets in.ChangesetLoader
	)
	stop, log := fakeGraph(t, repo.Dir, fakelsp, &changesets, &ports.Files, &ports.Docs, &ports.LSP, &ports.Hover, &ports.Define, &ports.Watch)
	cs, err := changesets.LoadChangeset(t.Context(), change.Comparison{Left: change.Commit("HEAD"), Right: change.Worktree()})
	require.NoError(t, err)
	ports.Files.Load(filediff.Request{Gen: 1, Comparison: cs.Comparison, File: fileAt(t, cs, "a.go")})
	left := testsupport.Collect(t, ports.Files, func(u []filediff.Update) bool { return len(u) > 0 })[0].Snapshot.LeftDoc
	require.Equal(t, "package old\n", left.Text)
	_, ok := ports.Docs.Track(left)
	require.False(t, ok, "no server before the checkout")
	pos := hover.Position{Req: 1, Commit: left.Commit, Path: left.Path, Version: left.Version, Byte: len("package ")}

	require.Equal(t, hover.NotReady, ports.Hover.Hover(pos))
	testsupport.Collect(t, ports.LSP, func(us []hover.Update) bool {
		return slices.ContainsFunc(us, func(u hover.Update) bool { return u.Kind == hover.CheckoutChanged && u.Status == hover.Ready })
	})
	server, ok := ports.Docs.Track(left)
	require.True(t, ok)
	require.Equal(t, left.Commit, server.Commit)
	testsupport.Collect(t, ports.LSP, statusOf(server, hover.Ready))
	pos.Req = 2
	require.Equal(t, hover.Sent, ports.Hover.Hover(pos))
	got := testsupport.Collect(t, ports.LSP, func(us []hover.Update) bool {
		return slices.ContainsFunc(us, func(u hover.Update) bool { return u.Kind == hover.HoverDone })
	})

	assert.Equal(t, hover.Text, got[len(got)-1].Outcome)
	assert.Equal(t, 2, strings.Count(repo.Git("worktree", "list"), "\n"), "the checkout")
	require.NoError(t, stop())
	assert.Equal(t, 1, strings.Count(repo.Git("worktree", "list"), "\n"), "no checkout left")
	store, err := os.ReadDir(filepath.Join(filepath.Dir(log), "worktrees"))
	require.NoError(t, err)
	assert.Empty(t, store, "nor anything of it, its repository's directory neither")
}

func statusOf(i lang.Instance, s hover.ServerStatus) func([]hover.Update) bool {
	return func(us []hover.Update) bool {
		return slices.ContainsFunc(us, func(u hover.Update) bool { return u.Instance == i && u.Status == s })
	}
}
