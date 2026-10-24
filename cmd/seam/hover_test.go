package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/adapter/driving/tui"
	"seam/internal/app/port/in"
	"seam/internal/domain/change"
	"seam/internal/testsupport"
)

// screen collects what the TUI writes.
type screen struct {
	mu  sync.Mutex
	out strings.Builder
}

func (s *screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.out.Write(p)
}

func (s *screen) shows(text string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Contains(s.out.String(), text)
}

// TestTheMouseRestingOnCodeShowsItsHover runs seam's graph with fakelsp
// against a repository whose one change is a.go, rests the mouse on its
// first word and waits, typing nothing: the answer appears in a popup,
// at once or after the server's delay.
func TestTheMouseRestingOnCodeShowsItsHover(t *testing.T) {
	fakelsp := buildFakelsp(t)
	for _, delay := range []string{"", "hover-delay 800ms\n"} {
		t.Run(strings.TrimSpace("answering "+delay), func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			repo := testsupport.NewGitRepo(t)
			repo.Write("a.go", "package a\n")
			repo.Git("add", ".")
			repo.Git("commit", "-q", "-m", "first")
			repo.Write("a.go", "package a\n\nfunc f() {}\n")
			require.NoError(t, os.WriteFile(filepath.Join(repo.Dir, "fakelsp.script"), []byte(delay+"hover func probe()\n"), 0o600))
			var (
				ports      tui.Ports
				changesets in.ChangesetLoader
			)
			stop, _ := fakeGraph(t, repo.Dir, fakelsp, &changesets, &ports.Files, &ports.Reload, &ports.Docs, &ports.LSP, &ports.Hover)
			cs, err := changesets.LoadChangeset(t.Context(), change.Comparison{Left: change.Commit("HEAD"), Right: change.Worktree()})
			require.NoError(t, err)
			out := &screen{}
			typeKeys, wait := runTUI(t, ports, cs, out)
			require.Eventually(t, func() bool { return out.shows("fake") }, testsupport.Guard, 10*time.Millisecond, "the server of a.go")

			// Any motion, no button, at the p of package: the right pane's
			// text starts at column 46, one-based, below the header.
			typeKeys("\x1b[<35;47;2M")

			assert.Eventually(t, func() bool { return out.shows("func probe()") }, testsupport.Guard, 10*time.Millisecond)
			typeKeys("q")
			assert.NoError(t, wait())
			assert.NoError(t, stop())
		})
	}
}
