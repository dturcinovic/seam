package main

import (
	"fmt"
	"runtime"
	"runtime/metrics"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/in"
	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/limits"
	"seam/internal/testsupport"
)

// samplePeak samples the heap in use every 2 ms until the returned
// function is called, which returns by how much the highest sample exceeds
// the heap in use at the start, after a collection; the test's cleanup
// stops it too, should the test end first. It reads runtime/metrics
// rather than MemStats, which would stop the world at each sample and slow
// the load it measures.
func samplePeak(t *testing.T) (stop func() uint64) {
	runtime.GC()
	start := heapInUse()
	peak := start
	var wg sync.WaitGroup
	done := make(chan struct{})
	wg.Go(func() {
		tick := time.NewTicker(2 * time.Millisecond)
		defer tick.Stop()
		for {
			peak = max(peak, heapInUse())
			select {
			case <-done:
				return
			case <-tick.C:
			}
		}
	})
	var once sync.Once
	stop = func() uint64 {
		once.Do(func() { close(done); wg.Wait() })
		return peak - start
	}
	t.Cleanup(func() { stop() })
	return stop
}

// heapInUse is what MemStats calls HeapInuse: the spans holding objects,
// live or not yet swept, and their free slots.
func heapInUse() uint64 {
	s := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}, {Name: "/memory/classes/heap/unused:bytes"}}
	metrics.Read(s)
	return s[0].Value.Uint64() + s[1].Value.Uint64()
}

// goLines is exactly size bytes of Go-like lines of 60 bytes; every
// hundredth line holds seed, so two seeds differ throughout the file. The
// cut at size may fall inside a line: only chroma reads it, which needs no
// valid Go.
func goLines(size, seed int) string {
	var b strings.Builder
	b.Grow(size + 64)
	b.WriteString("package p\n\nvar x = []int{\n")
	for i := 0; b.Len() < size; i++ {
		v := i
		if i%100 == 0 {
			v = seed
		}
		fmt.Fprintf(&b, "\t%-50d, // pad\n", v)
	}
	return b.String()[:size-2] + "}\n"
}

// TestThePeakOfALoadIsBounded loads, through seam's graph, a file just
// under the size limit on both sides, one just over it and one whose new
// side is a single line of 5 MiB, sampling the heap as it goes.
func TestThePeakOfALoadIsBounded(t *testing.T) {
	const limit = int(limits.DefaultFileSize)
	repo := testsupport.NewGitRepo(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo.Write("under.go", goLines(limit-1024, 1))
	repo.Write("over.go", goLines(limit+1, 1))
	repo.Write("line.go", "package p\n")
	repo.Git("add", ".")
	repo.Git("commit", "-q", "-m", "first")
	repo.Write("under.go", goLines(limit-1024, 2))
	repo.Write("over.go", goLines(limit+1, 2))
	repo.Write("line.go", "package p\n\nvar s = \""+strings.Repeat("x", 5<<20)+"\"\n")
	var (
		changesets in.ChangesetLoader
		files      in.FileDiffLoader
	)
	// No document reaches a server: the loads alone are measured.
	stop, _ := fakeGraph(t, repo.Dir, "fakelsp-not-started", &changesets, &files)
	cs, err := changesets.LoadChangeset(t.Context(), change.Comparison{Left: change.Commit("HEAD"), Right: change.Worktree()})
	require.NoError(t, err)

	tests := []struct {
		path     string
		bound    uint64
		tooLarge bool
	}{
		// chroma's matching leaves some 8 times the text as garbage, which
		// the collector lets pile up to about twice what is live.
		{"under.go", 192 << 20, false},
		{"over.go", 1 << 20, true},
		{"line.go", 64 << 20, false},
	}
	for i, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			peak := samplePeak(t)
			r := filediff.Request{Gen: filediff.Gen(i + 1), Comparison: cs.Comparison, File: fileAt(t, cs, tt.path)}
			u := load(t, files, r)
			grown := peak()

			t.Logf("%s: the heap grew by up to %d MiB while loading", tt.path, grown>>20)
			assert.Equal(t, tt.tooLarge, slices.ContainsFunc(u.Snapshot.Notices, func(n filediff.Notice) bool { return n.Kind == filediff.TooLarge }))
			assert.Equal(t, tt.tooLarge, len(u.Snapshot.Diff.Rows) == 0, "rows only from what was read")
			assert.Less(t, grown, tt.bound)
		})
	}
	assert.NoError(t, stop())
}
