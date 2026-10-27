package main

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/adapter/driving/tui"
	"github.com/dturcinovic/seam/internal/app/port/in"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/syntax"
	"github.com/dturcinovic/seam/internal/testsupport"
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
	return strings.Contains(s.text(), text)
}

func (s *screen) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.out.String()
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
			stop, _ := fakeGraph(t, repo.Dir, fakelsp, &changesets, &ports.Files, &ports.Stream, &ports.Reload, &ports.Docs, &ports.LSP, &ports.Hover, &ports.Define, &ports.Watch,
				&ports.NoteWriter, &ports.NoteReader, &ports.NoteUpdates)
			cs, err := changesets.LoadChangeset(t.Context(), change.Comparison{Left: change.Commit("HEAD"), Right: change.Worktree()})
			require.NoError(t, err)
			out := &screen{}
			typeKeys, wait := runTUI(t, ports, cs, out)
			// A motion over the view before a.go is laid out hovers nothing;
			// one before its server is ready is asked again once it is. The
			// server's name is no sign: the renderer writes only the cells
			// that change, so "fake" may never appear in one piece.
			require.Eventually(t, func() bool { return out.shows("func f()") }, testsupport.Guard, 10*time.Millisecond, "a.go laid out")

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

// TestAHoverShowsItsMarkdown: the answer shows with its rule across and
// without its marks. A pipe gets no colours; TestTheCodeOfAHoverArrivesColoured
// checks those.
func TestAHoverShowsItsMarkdown(t *testing.T) {
	fakelsp := buildFakelsp(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := testsupport.NewGitRepo(t)
	repo.Write("a.go", "package a\n")
	repo.Git("add", ".")
	repo.Git("commit", "-q", "-m", "first")
	repo.Write("a.go", "package a\n\nfunc f() {}\n")
	answer := "```go\nfunc probe()\n```\n\n---\n\nProbe **probes** `x`."
	require.NoError(t, os.WriteFile(filepath.Join(repo.Dir, "fakelsp.script"), []byte("hover "+strconv.Quote(answer)+"\n"), 0o600))
	var (
		ports      tui.Ports
		changesets in.ChangesetLoader
	)
	stop, _ := fakeGraph(t, repo.Dir, fakelsp, &changesets, &ports.Files, &ports.Stream, &ports.Reload, &ports.Docs, &ports.LSP, &ports.Hover, &ports.Define, &ports.Watch,
		&ports.NoteWriter, &ports.NoteReader, &ports.NoteUpdates)
	cs, err := changesets.LoadChangeset(t.Context(), change.Comparison{Left: change.Commit("HEAD"), Right: change.Worktree()})
	require.NoError(t, err)
	out := &screen{}
	typeKeys, wait := runTUI(t, ports, cs, out)
	// Why a.go and not the server's name: see TestTheMouseRestingOnCodeShowsItsHover.
	require.Eventually(t, func() bool { return out.shows("func f()") }, testsupport.Guard, 10*time.Millisecond, "a.go laid out")

	typeKeys("\x1b[<35;47;2M")

	require.Eventually(t, func() bool { return out.shows("probes") }, testsupport.Guard, 10*time.Millisecond)
	typeKeys("q")
	assert.NoError(t, wait())
	assert.NoError(t, stop())
	assert.True(t, out.shows("─────"), "the rule")
	assert.False(t, out.shows("**probes**"), "no marks")
	assert.False(t, out.shows("```"), "no fence")
}

// TestTheCodeOfAHoverArrivesColoured asks seam's graph, with fakelsp and
// chroma, through the ports the TUI calls: the answer's Go code comes with
// its tokens.
func TestTheCodeOfAHoverArrivesColoured(t *testing.T) {
	fakelsp := buildFakelsp(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := testsupport.NewGitRepo(t)
	repo.Write("a.go", "package a\n")
	repo.Git("add", ".")
	repo.Git("commit", "-q", "-m", "first")
	answer := "```go\nfunc probe()\n```"
	require.NoError(t, os.WriteFile(filepath.Join(repo.Dir, "fakelsp.script"), []byte("hover "+strconv.Quote(answer)+"\n"), 0o600))
	var ports tui.Ports
	stop, _ := fakeGraph(t, repo.Dir, fakelsp, &ports.Docs, &ports.LSP, &ports.Hover)
	defer func() { assert.NoError(t, stop()) }()
	_, ok := ports.Docs.Track(hover.Document{Path: "a.go", Lang: lang.Go, Version: 1, Text: "package a\n"})
	require.True(t, ok)
	testsupport.Collect(t, ports.LSP, func(us []hover.Update) bool {
		return slices.ContainsFunc(us, func(u hover.Update) bool { return u.Status == hover.Ready })
	})

	require.Equal(t, hover.Sent, ports.Hover.Hover(hover.Position{Req: 1, Path: "a.go", Version: 1}))

	us := testsupport.Collect(t, ports.LSP, func(us []hover.Update) bool {
		return slices.ContainsFunc(us, func(u hover.Update) bool { return u.Kind == hover.HoverDone })
	})
	u := us[slices.IndexFunc(us, func(u hover.Update) bool { return u.Kind == hover.HoverDone })]
	require.Equal(t, hover.Text, u.Outcome)
	code := u.Content.Blocks[0]
	assert.Equal(t, lang.Go, code.Lang)
	require.Equal(t, 1, code.Tokens.Lines())
	assert.Equal(t, syntax.Run{End: int32(len("func")), Kind: syntax.Keyword}, code.Tokens.Line(0)[0])
}
