package usecase

import (
	"context"
	"fmt"
	"hash/maphash"
	"time"

	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/diff"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/limits"
	"github.com/dturcinovic/seam/internal/supervise"
)

// commitVersion is the version of a document from a commit, whose text
// never changes.
const commitVersion hover.DocVersion = 1

// diffBudget bounds the search of one diff; past it the diff comes out
// coarser rather than late. highlightBudget bounds finding the tokens of one
// side; past it the rest of the side has no colours.
const (
	diffBudget      = time.Second
	highlightBudget = 500 * time.Millisecond
)

// FileDiffs implements in.FileDiffLoader.
type FileDiffs struct {
	*worker[filediff.Request, filediff.Update]

	blobs       out.BlobReader
	worktree    out.WorktreeReader
	stats       out.WorktreeStatter
	highlighter out.Highlighter
	languages   lang.Table
	max         limits.FileSize
	budget      time.Duration

	cache    *cache
	seed     maphash.Seed
	versions *versions
}

// NewFileDiffs returns the use case over its ports. Its worker runs between
// Start and Stop.
func NewFileDiffs(b out.BlobReader, w out.WorktreeReader, s out.WorktreeStatter, h out.Highlighter, languages lang.Table, max limits.FileSize, panics supervise.PanicFunc) *FileDiffs {
	f := &FileDiffs{
		blobs: b, worktree: w, stats: s, highlighter: h, languages: languages, max: max, budget: diffBudget,
		cache:    newCache(cacheEntries, cacheBytes),
		seed:     maphash.MakeSeed(),
		versions: newVersions(),
	}
	f.worker = newWorker("filediff", filediff.ErrLoaderFailed, panics, f.load, supersedes, failedLoad)
	return f
}

func failedLoad(r filediff.Request, err error) filediff.Update {
	return filediff.Update{Gen: r.Gen, File: r.File, Err: err}
}

// supersedes cancels the running load for another file, or for a fresh
// read of the same: either way its result would be replaced at once.
func supersedes(running, next filediff.Request) bool {
	return running.File != next.File || next.Fresh
}

// Load queues r; it does no I/O, not even for a cached file.
func (f *FileDiffs) Load(r filediff.Request) { f.submit(r) }

func (f *FileDiffs) load(ctx context.Context, r filediff.Request) filediff.Update {
	if !r.Fresh {
		if s, ok := f.fromCache(ctx, r); ok {
			return filediff.Update{Gen: r.Gen, File: r.File, Snapshot: s}
		}
	}
	e, err := f.compute(ctx, r)
	if err != nil {
		return filediff.Update{Gen: r.Gen, File: r.File, Err: err}
	}
	// Colours cut short by time are not kept: the next visit may have it.
	if !e.late {
		f.cache.put(e)
	}
	return filediff.Update{Gen: r.Gen, File: r.File, Snapshot: e.snapshot}
}

// fromCache returns the cached snapshot unless the worktree file has a new
// stamp. A failed stat counts as changed: the load that follows reports it.
func (f *FileDiffs) fromCache(ctx context.Context, r filediff.Request) (filediff.Snapshot, bool) {
	e, ok := f.cache.get(r.File)
	if !ok {
		return filediff.Snapshot{}, false
	}
	if r.ReadsWorktree() {
		stamp, err := f.stats.StatWorktree(ctx, r.File.NewPath)
		if err != nil || !stamp.Equal(e.stamp) {
			// Stale: dropped rather than kept as the most recently used.
			f.cache.remove(r.File)
			return filediff.Snapshot{}, false
		}
	}
	s := e.snapshot
	// The path may have had other text since, under another key: its
	// version must not go back.
	if r.ReadsWorktree() && s.RightDoc.Path != "" {
		s.RightDoc.Version = f.versions.of(r.File.NewPath, e.sum)
	}
	return s, true
}

// compute reads both sides and diffs them. A side too large or binary
// leaves the diff empty and says so in a notice.
func (f *FileDiffs) compute(ctx context.Context, r filediff.Request) (*cached, error) {
	old, oldNote, err := f.readOld(ctx, r.File)
	if err != nil {
		return nil, fmt.Errorf("read old side of %s: %w", r.File.Path(), err)
	}
	new, stamp, newNote, err := f.readNew(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("read new side of %s: %w", r.File.Path(), err)
	}
	if newNote != nil && r.ReadsWorktree() {
		// A file too large to read has no stamp from the reader; without one
		// every visit would read the other side again for the same notice.
		// A failed stat leaves it empty, which only costs that rereading.
		stamp, _ = f.stats.StatWorktree(ctx, r.File.NewPath)
	}
	s := filediff.Snapshot{File: r.File}
	for _, n := range []*filediff.Notice{oldNote, newNote} {
		if n != nil {
			s.Notices = append(s.Notices, *n)
		}
	}
	e := &cached{stamp: stamp}
	if len(s.Notices) == 0 {
		if err := f.showText(ctx, &s, e, r, old, new); err != nil {
			return nil, err
		}
	}
	e.snapshot, e.size = s, s.Diff.ApproxBytes()+s.Columns.ApproxBytes()+s.Highlights.ApproxBytes()
	return e, nil
}

// showText puts the diff of both sides' text into s with the column index of
// its long lines and their tokens, and the documents of the sides.
func (f *FileDiffs) showText(ctx context.Context, s *filediff.Snapshot, e *cached, r filediff.Request, old, new string) error {
	d, err := f.diff(ctx, old, new)
	if err != nil {
		return err
	}
	if d.Simplified {
		s.Notices = append(s.Notices, filediff.Notice{Kind: filediff.Simplified})
	}
	if s.Columns, err = indexColumns(ctx, d); err != nil {
		return err
	}
	oldLang, newLang := f.languages.Detect(s.File.OldPath, old), f.languages.Detect(s.File.NewPath, new)
	var oldLate, newLate bool
	if s.Highlights.Old, oldLate, err = f.highlightSide(ctx, s, filediff.OldSide, oldLang, old, len(d.Old.Lines)); err != nil {
		return err
	}
	if s.Highlights.New, newLate, err = f.highlightSide(ctx, s, filediff.NewSide, newLang, new, len(d.New.Lines)); err != nil {
		return err
	}
	e.late = oldLate || newLate
	s.Diff = d
	if s.File.OldPath != "" {
		s.LeftDoc = hover.Document{Commit: r.Comparison.Left.ID(), Path: s.File.OldPath, Lang: oldLang, Version: commitVersion, Text: old}
	}
	switch {
	case r.ReadsWorktree():
		e.sum = maphash.String(f.seed, new)
		s.RightDoc = hover.Document{Path: s.File.NewPath, Lang: newLang, Version: f.versions.of(s.File.NewPath, e.sum), Text: new}
	case r.Comparison.Right.Kind() == change.KindCommit && s.File.NewPath != "":
		s.RightDoc = hover.Document{Commit: r.Comparison.Right.ID(), Path: s.File.NewPath, Lang: newLang, Version: commitVersion, Text: new}
	}
	return nil
}

func (f *FileDiffs) diff(ctx context.Context, old, new string) (diff.FileDiff, error) {
	budget, cancel := context.WithTimeout(ctx, f.budget)
	defer cancel()
	d := diff.Compute(budget, diff.Split(old), diff.Split(new))
	// The budget ending gives a coarser diff; the job being cancelled gives
	// none, as nobody waits for it.
	if err := ctx.Err(); err != nil {
		return diff.FileDiff{}, err
	}
	return d, nil
}

func (f *FileDiffs) readOld(ctx context.Context, file change.ChangedFile) (string, *filediff.Notice, error) {
	if file.OldPath == "" {
		return "", nil, nil
	}
	data, err := f.blobs.ReadBlob(ctx, file.OldID, f.max)
	return asText(filediff.OldSide, data, err)
}

// readNew reads the right side, with its stamp if it is a worktree file.
func (f *FileDiffs) readNew(ctx context.Context, r filediff.Request) (string, change.Stamp, *filediff.Notice, error) {
	if r.File.NewPath == "" {
		return "", change.Stamp{}, nil, nil
	}
	var (
		data  []byte
		stamp change.Stamp
		err   error
	)
	if r.ReadsWorktree() {
		data, stamp, err = f.worktree.ReadWorktree(ctx, r.File.NewPath, f.max)
	} else {
		data, err = f.blobs.ReadBlob(ctx, r.File.NewID, f.max)
	}
	text, n, err := asText(filediff.NewSide, data, err)
	return text, stamp, n, err
}

// asText judges a side and turns it into text at once, so its bytes, up to
// the file size limit, are not held twice while the diff runs.
func asText(side filediff.Which, data []byte, err error) (string, *filediff.Notice, error) {
	n, err := filediff.SideNotice(side, data, err)
	if n != nil || err != nil {
		return "", n, err
	}
	return string(data), nil, nil
}
