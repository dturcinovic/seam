package usecase_test

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/out/outfakes"
	"seam/internal/domain/change"
	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/lang"
	"seam/internal/domain/syntax"
)

// coveringHighlighter marks the first byte of every line a keyword, so a
// test sees which text got tokens.
func coveringHighlighter() *outfakes.FakeHighlighter {
	h := &outfakes.FakeHighlighter{}
	h.HighlightCalls(func(_ context.Context, _ lang.Lang, text string) (syntax.Tokens, bool) {
		return firstBytes(len(diff.Split(text).Lines)), true
	})
	return h
}

func firstBytes(lines int) syntax.Tokens {
	var b syntax.Builder
	for range lines {
		_ = b.Add(1, syntax.Keyword) // Valid by construction.
		b.EndLine(0)
	}
	return b.Tokens()
}

func TestTheLanguagesComeFromTheTable(t *testing.T) {
	table, err := lang.NewTable([]lang.Language{{Lang: lang.Go, Extensions: []string{".tmpl"}}}, nil)
	require.NoError(t, err)
	uc, f := newFileDiffsIn(t, table)
	start(t, uc)
	file := f.modified(t, "a.tmpl", "package a\n", "package b\n")

	u := loadOne(t, uc, worktreeComparison(t), 1, file, false)

	require.NoError(t, u.Err)
	require.Equal(t, 2, f.highlighter.HighlightCallCount())
	_, l, _ := f.highlighter.HighlightArgsForCall(1)
	assert.Equal(t, lang.Go, l)
}

func TestBothSidesGetTokensInTheirLanguage(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	file := f.modified(t, "a.go", "package a\n", "package a\n\nfunc F() {}\n")
	file.OldPath = "a.py" // a rename across languages

	u := loadOne(t, uc, worktreeComparison(t), 1, file, false)

	require.NoError(t, u.Err)
	assert.Equal(t, 1, u.Snapshot.Highlights.Old.Lines())
	assert.Equal(t, 3, u.Snapshot.Highlights.New.Lines())
	require.Equal(t, 2, f.highlighter.HighlightCallCount())
	_, l, text := f.highlighter.HighlightArgsForCall(0)
	assert.Equal(t, lang.Python, l)
	assert.Equal(t, "package a\n", text)
	_, l, text = f.highlighter.HighlightArgsForCall(1)
	assert.Equal(t, lang.Go, l)
	assert.Equal(t, "package a\n\nfunc F() {}\n", text)
	assert.Empty(t, u.Snapshot.Notices)
}

// TestTheBudgetIsADeadlinePerSide: a highlighter that runs until its
// context ends gets 500 ms for each side, and what it covered by then stays,
// but is not cached, so the next visit tries again.
func TestTheBudgetIsADeadlinePerSide(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		uc, f := newFileDiffs(t)
		require.NoError(t, uc.Start())
		var spent []time.Duration
		f.highlighter.HighlightCalls(func(ctx context.Context, _ lang.Lang, _ string) (syntax.Tokens, bool) {
			begin := time.Now()
			<-ctx.Done()
			spent = append(spent, time.Since(begin))
			return firstBytes(1), true
		})
		file := f.modified(t, "a.go", "a\nb\n", "a\nc\n")
		c := worktreeComparison(t)

		for gen := range filediff.Gen(2) {
			uc.Load(filediff.Request{Gen: gen + 1, Comparison: c, File: file})
			synctest.Wait()
			<-uc.Signal()
			u := uc.Drain()
			require.Len(t, u, 1)
			require.NoError(t, u[0].Err)
			assert.Equal(t, []filediff.Notice{
				{Kind: filediff.HighlightStopped, Side: filediff.OldSide, Line: 2},
				{Kind: filediff.HighlightStopped, Side: filediff.NewSide, Line: 2},
			}, u[0].Snapshot.Notices)
		}

		budget := 500 * time.Millisecond
		assert.Equal(t, []time.Duration{budget, budget, budget, budget}, spent,
			"each side its budget, and the second visit lexes again")
		require.NoError(t, uc.Stop(t.Context()))
	})
}

func TestColoursThatStopShortSaySoPerSide(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	f.highlighter.HighlightCalls(func(_ context.Context, _ lang.Lang, text string) (syntax.Tokens, bool) {
		if strings.HasPrefix(text, "new") {
			return firstBytes(1), true
		}
		return firstBytes(3), true
	})
	file := f.modified(t, "a.go", "old\nb\nc\n", "new\nb\nc\n")

	u := loadOne(t, uc, worktreeComparison(t), 1, file, false)

	require.NoError(t, u.Err)
	assert.Equal(t, []filediff.Notice{{Kind: filediff.HighlightStopped, Side: filediff.NewSide, Line: 2}}, u.Snapshot.Notices)
	assert.Equal(t, 1, u.Snapshot.Highlights.New.Lines(), "what there is is kept")
}

func TestTextWithoutLanguageOrLinesIsNotHighlighted(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	file := f.modified(t, "notes.txt", "a\n", "b\n")
	added := f.modified(t, "new.go", "", "")
	added.Status, added.OldPath, added.OldMode = change.Added, "", 0

	for gen, file := range []change.ChangedFile{file, added} {
		u := loadOne(t, uc, worktreeComparison(t), filediff.Gen(gen+1), file, false)
		require.NoError(t, u.Err)
		assert.Empty(t, u.Snapshot.Notices)
	}

	assert.Zero(t, f.highlighter.HighlightCallCount())
}

// TestACancelledJobDropsItsTokens: the job is superseded while the
// highlighter runs; what it returns is not delivered.
func TestACancelledJobDropsItsTokens(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	started := make(chan struct{})
	f.highlighter.HighlightCalls(func(ctx context.Context, _ lang.Lang, text string) (syntax.Tokens, bool) {
		if strings.HasPrefix(text, "slow") {
			close(started)
			<-ctx.Done()
		}
		return firstBytes(len(diff.Split(text).Lines)), true
	})
	slow := f.modified(t, "slow.go", "slow\n", "slow\n!\n")
	fast := f.modified(t, "fast.go", "fast\n", "fast\n!\n")
	c := worktreeComparison(t)

	uc.Load(filediff.Request{Gen: 1, Comparison: c, File: slow})
	<-started
	u := loadOne(t, uc, c, 2, fast, false)

	require.NoError(t, u.Err)
	assert.Equal(t, fast, u.Snapshot.File)
	assert.Empty(t, uc.Drain(), "the slow one left nothing")
}

// TestALanguageWithoutLexerSaysNothing: no colours are expected there, so
// none missing is worth a notice.
func TestALanguageWithoutLexerSaysNothing(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	f.highlighter.HighlightReturns(syntax.Tokens{}, false)
	file := f.modified(t, "a.go", "a\n", "b\n")

	u := loadOne(t, uc, worktreeComparison(t), 1, file, false)

	require.NoError(t, u.Err)
	assert.Empty(t, u.Snapshot.Notices)
}

// TestTheLexerNeverSeesAVeryLongLine: the text ends before the first line
// over syntax.MaxLine, and the colours stop there with a notice.
func TestTheLexerNeverSeesAVeryLongLine(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	text := "a\nb\n" + strings.Repeat("x", syntax.MaxLine+1) + "\nc\n"
	file := f.modified(t, "a.go", text, text+"d\n")

	u := loadOne(t, uc, worktreeComparison(t), 1, file, false)

	require.NoError(t, u.Err)
	_, _, seen := f.highlighter.HighlightArgsForCall(1)
	assert.Equal(t, "a\nb\n", seen)
	assert.Contains(t, u.Snapshot.Notices, filediff.Notice{Kind: filediff.HighlightStopped, Side: filediff.NewSide, Line: 3})
}
