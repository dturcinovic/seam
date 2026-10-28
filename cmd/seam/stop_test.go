package main

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/dturcinovic/seam/internal/adapter/driven/configfile"
	"github.com/dturcinovic/seam/internal/app/port/in"
	"github.com/dturcinovic/seam/internal/config"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/observability"
	"github.com/dturcinovic/seam/internal/testsupport"
)

// TestTheGraphStopsInTimeWithAStubbornServer runs the session's graph with
// git and a language server that neither reads nor answers shutdown: the
// stop takes the servers' share of the budget and leaves git enough to end
// cat-file.
func TestTheGraphStopsInTimeWithAStubbornServer(t *testing.T) {
	// Built here, as from inside the test's repository go knows no module.
	fakelsp := buildFakelsp(t)
	inRepo(t)
	root, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile("a.go", []byte("package a\n"), 0o600))
	require.NoError(t, os.WriteFile("fakelsp.script", []byte("stop-reading\nno-shutdown\n"), 0o600))
	var (
		changesets in.ChangesetLoader
		files      in.FileDiffLoader
		docs       in.DocumentTracker
		updates    in.LSPUpdates
	)
	stop, log := fakeGraph(t, root, fakelsp, &docs, &updates, &files, &changesets)

	c := change.Comparison{Left: change.Commit("HEAD"), Right: change.Worktree()}
	cs, err := changesets.LoadChangeset(t.Context(), c)
	require.NoError(t, err)
	file := fileAt(t, cs, "committed.txt")
	files.Load(filediff.Request{Gen: 1, Comparison: cs.Comparison, File: file})
	require.NoError(t, testsupport.Collect(t, files, func(u []filediff.Update) bool { return len(u) > 0 })[0].Err, "cat-file runs")
	files.Load(filediff.Request{Gen: 2, Comparison: cs.Comparison, File: fileAt(t, cs, "a.go")})
	shown := testsupport.Collect(t, files, func(u []filediff.Update) bool { return len(u) > 0 })[0]
	_, ok := docs.Track(shown.Snapshot.RightDoc)
	require.True(t, ok)
	testsupport.Collect(t, updates, func(u []hover.Update) bool {
		return len(u) > 0 && u[len(u)-1].Status == hover.Ready
	})

	require.ElementsMatch(t, []string{"git", "fakelsp"}, children(t))

	begin := time.Now()
	require.NoError(t, stop())

	assert.Less(t, time.Since(begin), stopBudget)
	assert.Empty(t, children(t), "cat-file and the server have ended")
	assert.Equal(t, []string{"lsp.newModulePool", "git.newCheckouts", "git.newRepo"}, stopOrder(t, log),
		"the checkouts once their servers are gone, git last")
}

// stopOrder is the constructors in the adapters that added the stop hooks,
// as fx logged running them; the use cases' workers stop in the order
// their ports were asked for.
func stopOrder(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	require.NoError(t, err)
	var order []string
	for line := range strings.Lines(string(data)) {
		if !strings.Contains(line, `"msg":"OnStop hook executing"`) {
			continue
		}
		_, caller, _ := strings.Cut(line, `"caller":"github.com/dturcinovic/seam/internal/`)
		caller, _, _ = strings.Cut(caller, `"`)
		if strings.HasPrefix(caller, "adapter/") {
			order = append(order, path.Base(caller))
		}
	}
	return order
}

// fakeGraph starts seam's graph on root, fakelsp serving Go, and fills
// targets from it. stop stops it; the test's end does, if a failure came
// first, as the servers must not outlive it. log is the debug log, next to
// which the checkouts lie, in worktrees.
func fakeGraph(t *testing.T, root, fakelsp string, targets ...any) (stop func() error, log string) {
	t.Helper()
	table, err := lang.NewTable([]lang.Language{{Lang: lang.Go, Extensions: []string{".go"}, Server: "fake"}},
		[]lang.Server{{Name: "fake", Command: fakelsp}})
	require.NoError(t, err)
	state := t.TempDir()
	cfg := config.Config{Dir: root, LogFile: filepath.Join(state, "seam.log"), Checkouts: filepath.Join(state, "worktrees"), Verbose: true}
	logger, err := observability.NewLogger(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, logger.Close()) })
	app := fx.New(append(options(cfg, logger, root, "", configfile.File{Languages: table}), fx.Populate(targets...))...)
	require.NoError(t, app.Err())
	require.NoError(t, app.Start(t.Context()))
	var once sync.Once
	var stopErr error
	stop = func() error {
		once.Do(func() { stopErr = stopGraph(app) })
		return stopErr
	}
	t.Cleanup(func() { _ = stop() })
	return stop, cfg.LogFile
}

func fileAt(t *testing.T, cs change.Changeset, path string) change.ChangedFile {
	t.Helper()
	i := slices.IndexFunc(cs.Files, func(f change.ChangedFile) bool { return f.Path() == path })
	require.GreaterOrEqual(t, i, 0, "%s is not in the changeset", path)
	return cs.Files[i]
}

// buildFakelsp builds the adapter's test server for this test.
func buildFakelsp(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fakelsp")
	out, err := exec.CommandContext(t.Context(), "go", "build", "-o", path, "github.com/dturcinovic/seam/internal/adapter/driven/lsp/cmd/fakelsp").CombinedOutput()
	require.NoError(t, err, string(out))
	return path
}
