package usecase

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/app/port/out/outfakes"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/limits"
	"github.com/dturcinovic/seam/internal/domain/syntax"
)

// These tests call load directly, without the worker, to look at the cache
// it fills.

// internalFileDiffs reads old as every blob and new as every worktree file,
// and makes every line a comment.
func internalFileDiffs(old, new string) *FileDiffs {
	blobs, worktree, highlighter := &outfakes.FakeBlobReader{}, &outfakes.FakeWorktreeReader{}, &outfakes.FakeHighlighter{}
	blobs.ReadBlobReturns([]byte(old), nil)
	worktree.ReadWorktreeReturns([]byte(new), change.Stamp{}, nil)
	highlighter.HighlightCalls(func(_ context.Context, _ lang.Lang, text string) (syntax.Tokens, bool) {
		var b syntax.Builder
		for _, line := range diff.Split(text).Lines {
			_ = b.Add(len(line), syntax.Comment) // Valid by construction.
			b.EndLine(len(diff.Content(line)))
		}
		return b.Tokens(), true
	})
	return NewFileDiffs(blobs, worktree, &outfakes.FakeSubmoduleReader{}, &outfakes.FakeWorktreeStatter{}, PathPorts{}, highlighter, lang.Defaults(), limits.DefaultFileSize, nil)
}

func request(t *testing.T, path string) filediff.Request {
	t.Helper()
	c, err := change.NewComparison(change.Commit("a"), change.Worktree())
	require.NoError(t, err)
	return filediff.Request{Comparison: c, File: change.ChangedFile{Status: change.Added, NewPath: path, NewMode: 0o100644}}
}

func TestTheCacheKeepsItsBounds(t *testing.T) {
	tests := []struct {
		name     string
		lines    int
		maxBytes int64
	}{
		{"by count", 10, cacheBytes},
		{"by bytes", 1000, 200 << 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := internalFileDiffs("", strings.Repeat("some line of code\n", tt.lines))
			f.cache = newCache(cacheEntries, tt.maxBytes)

			for i := range 100 {
				u := f.load(t.Context(), request(t, fmt.Sprintf("f%d.go", i)))
				require.NoError(t, u.Err)
				assert.LessOrEqual(t, f.cache.entries.order.Len(), cacheEntries)
				assert.LessOrEqual(t, f.cache.entries.bytes, tt.maxBytes)
			}
			assert.Positive(t, f.cache.entries.order.Len())
			var sum int64
			for e := f.cache.entries.order.Front(); e != nil; e = e.Next() {
				s := e.Value.(*lruEntry[change.ChangedFile, *cached]).value.snapshot
				sum += s.Diff.ApproxBytes() + s.Columns.ApproxBytes() + s.Highlights.ApproxBytes()
			}
			assert.Equal(t, sum, f.cache.entries.bytes, "the count matches ApproxBytes")
			assert.Positive(t, f.cache.entries.order.Front().Value.(*lruEntry[change.ChangedFile, *cached]).value.snapshot.Highlights.ApproxBytes(), "tokens among them")
		})
	}
}

func TestAnEntryOverTheBudgetIsNotKept(t *testing.T) {
	f := internalFileDiffs("", "line\n")
	f.cache = newCache(cacheEntries, 10)

	u := f.load(t.Context(), request(t, "a.go"))

	require.NoError(t, u.Err)
	assert.NotEmpty(t, u.Snapshot.Diff.Rows, "it is still delivered")
	assert.Zero(t, f.cache.entries.order.Len())
}

func TestARanOutDiffSaysSo(t *testing.T) {
	f := internalFileDiffs("a\nx\nc\nd\n", "a\nb\nc\n")
	f.budget = 0
	r := request(t, "a.go")
	r.File.OldMode, r.File.OldPath = 0o100644, "a.go"

	u := f.load(t.Context(), r)

	require.NoError(t, u.Err)
	assert.Contains(t, u.Snapshot.Notices, filediff.Notice{Kind: filediff.Simplified})
}

func TestAStaleEntryIsDropped(t *testing.T) {
	f := internalFileDiffs("", "a\n")
	r := request(t, "a.go")
	require.NoError(t, f.load(t.Context(), r).Err)
	f.stats.(*outfakes.FakeWorktreeStatter).StatWorktreeReturns(change.Stamp{Size: 1}, nil)
	f.worktree.(*outfakes.FakeWorktreeReader).ReadWorktreeReturns(nil, change.Stamp{}, change.ErrNotRegular)

	require.ErrorIs(t, f.load(t.Context(), r).Err, change.ErrNotRegular)

	assert.Zero(t, f.cache.entries.order.Len(), "not kept as the most recently used")
}
