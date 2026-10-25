package main

import (
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

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

// use is the part of seam's graph a viewer drives while it switches files
// and hovers, without the TUI.
type use struct {
	t     *testing.T
	cs    change.Changeset
	ports tui.Ports
	texts map[string]string
	gen   filediff.Gen
	req   hover.ReqID
}

// switchTo loads f, shows its new side to the server and hovers twice on
// it, as a pointer passing over two words does; it waits for the second
// answer, the only one handed on.
func (u *use) switchTo(f change.ChangedFile) {
	u.t.Helper()
	u.gen++
	load(u.t, u.ports.Files, filediff.Request{Gen: u.gen, Comparison: u.cs.Comparison, File: f})

	version := hover.DocVersion(u.gen)
	server, ok := u.ports.Docs.Track(hover.Document{Path: f.Path(), Lang: lang.Go, Version: version, Text: u.texts[f.Path()]})
	require.True(u.t, ok)
	if u.gen == 1 {
		testsupport.Collect(u.t, u.ports.LSP, statusOf(server, hover.Ready))
	}
	for line := range 2 {
		u.req++
		require.Equal(u.t, hover.Sent, u.ports.Hover.Hover(hover.Position{Req: u.req, Path: f.Path(), Version: version, Line: line}))
	}
	testsupport.Collect(u.t, u.ports.LSP, func(us []hover.Update) bool {
		return slices.ContainsFunc(us, func(h hover.Update) bool { return h.Kind == hover.HoverDone && h.Req == u.req })
	})
}

// liveHeap is the heap the program holds once garbage is collected. It
// reads HeapAlloc, not HeapInuse, and collects twice: HeapInuse counts whole
// spans, and after a single collection objects freed by finalizers or swept
// late still count, both of which scatter the reading by a few hundred KiB,
// more than a leak of a few hundred bytes per request adds over 400 switches.
func liveHeap() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// goFile is a Go file of n functions, its own per seed.
func goFile(seed, n int) string {
	var b strings.Builder
	b.WriteString("package p\n\n")
	for i := range n {
		fmt.Fprintf(&b, "// F%d_%d returns its argument and %d.\nfunc F%d_%d(x int) int {\n\treturn x + %d\n}\n\n", seed, i, i, seed, i, i)
	}
	return b.String()
}

// TestTheHeapDoesNotGrowWithUse switches 500 times between 100 changed
// files and hovers 1000 times against fakelsp, through seam's whole graph:
// the caches, the documents and the requests are bounded, so the heap after
// 500 switches holds what it held after 100.
func TestTheHeapDoesNotGrowWithUse(t *testing.T) {
	fakelsp := buildFakelsp(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := testsupport.NewGitRepo(t)
	repo.Write("fakelsp.script", "hover "+strconv.Quote("```go\nfunc F(x int) int\n```")+"\n")
	texts := map[string]string{}
	for i := range 100 {
		texts[fmt.Sprintf("f%03d.go", i)] = goFile(i, 2)
	}
	for p := range texts {
		repo.Write(p, goFile(-1, 2))
	}
	repo.Git("add", ".")
	repo.Git("commit", "-q", "-m", "first")
	for p, text := range texts {
		repo.Write(p, text)
	}
	u := &use{t: t, texts: texts}
	var changesets in.ChangesetLoader
	stop, _ := fakeGraph(t, repo.Dir, fakelsp, &changesets, &u.ports.Files, &u.ports.Docs, &u.ports.LSP, &u.ports.Hover)
	cs, err := changesets.LoadChangeset(t.Context(), change.Comparison{Left: change.Commit("HEAD"), Right: change.Worktree()})
	require.NoError(t, err)
	require.Len(t, cs.Files, 100)
	u.cs = cs

	for i := range 100 {
		u.switchTo(cs.Files[i%len(cs.Files)])
	}
	after100, goroutines := liveHeap(), runtime.NumGoroutine()
	for i := range 400 {
		u.switchTo(cs.Files[(i*37)%len(cs.Files)])
	}
	after500 := liveHeap()

	t.Logf("live heap: %d KiB after 100 switches, %d KiB after 500", after100>>10, after500>>10)
	assert.Less(t, after100, uint64(32<<20), "a bound, whatever the use")
	// The runtime keeps the goroutine structures it made at the peak of
	// concurrency for reuse; they add up to some 50 KiB here.
	assert.Less(t, int64(after500)-int64(after100), int64(192<<10), "no growth with the switches")
	// The superseded first hover of the last switch may still be winding
	// down when its successor's answer arrives.
	assert.Eventually(t, func() bool { return runtime.NumGoroutine() <= goroutines }, testsupport.Guard, 10*time.Millisecond, "no goroutine left behind")
	assert.NoError(t, stop())
}
