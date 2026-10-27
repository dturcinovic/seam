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
	submodules  out.SubmoduleReader
	stats       out.WorktreeStatter
	paths       out.PathReader
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
func NewFileDiffs(b out.BlobReader, w out.WorktreeReader, sub out.SubmoduleReader, s out.WorktreeStatter, paths out.PathReader, h out.Highlighter, languages lang.Table, max limits.FileSize, panics supervise.PanicFunc) *FileDiffs {
	f := &FileDiffs{
		blobs: b, worktree: w, submodules: sub, stats: s, paths: paths, highlighter: h, languages: languages, max: max, budget: diffBudget,
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
	// Nor is a submodule of the worktree: a commit in it leaves the stamp
	// of its directory alone.
	worktreeSubmodule := r.ReadsWorktree() && r.File.NewMode == change.Gitlink
	if !e.late && !worktreeSubmodule {
		f.cache.put(e)
	}
	return filediff.Update{Gen: r.Gen, File: r.File, Snapshot: e.snapshot}
}

// fromCache returns the cached snapshot unless the right side has a new
// stamp. A failed stat counts as changed: the load that follows reports it.
func (f *FileDiffs) fromCache(ctx context.Context, r filediff.Request) (filediff.Snapshot, bool) {
	e, ok := f.cache.get(r.File)
	if !ok {
		return filediff.Snapshot{}, false
	}
	stamp, stamped, err := f.statNew(ctx, r)
	if stamped && (err != nil || !stamp.Equal(e.stamp)) {
		// Stale: dropped rather than kept as the most recently used.
		f.cache.remove(r.File)
		return filediff.Snapshot{}, false
	}
	s := e.snapshot
	// The path may have had other text since, under another key: its
	// version must not go back.
	if stamped && s.RightDoc.Path != "" {
		s.RightDoc.Version = f.versions.of(s.RightDoc.Path, e.sum)
	}
	return s, true
}

// statNew returns the stamp of a right side that can change while seam
// runs: a worktree file, or a file on disk, which git difftool -d links to
// the worktree. Its left side on disk is a copy git made and is not stamped.
func (f *FileDiffs) statNew(ctx context.Context, r filediff.Request) (change.Stamp, bool, error) {
	switch {
	case r.ReadsWorktree():
		stamp, err := f.stats.StatWorktree(ctx, r.File.NewPath)
		return stamp, true, err
	case r.Comparison.Right.Kind() == change.KindPath && r.File.NewPath != "":
		stamp, err := f.paths.StatPath(ctx, r.Comparison.Right.At(r.File.NewPath))
		return stamp, true, err
	}
	return change.Stamp{}, false, nil
}

// compute reads both sides and diffs them. A side too large or binary
// leaves the diff empty and says so in a notice.
func (f *FileDiffs) compute(ctx context.Context, r filediff.Request) (*cached, error) {
	old, oldNote, err := f.readOld(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("read old side of %s: %w", r.File.Path(), err)
	}
	new, stamp, newNote, err := f.readNew(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("read new side of %s: %w", r.File.Path(), err)
	}
	if newNote != nil {
		// A file too large to read has no stamp from the reader; without one
		// every visit would read the other side again for the same notice.
		// A failed stat leaves it empty, which only costs that rereading.
		stamp, _, _ = f.statNew(ctx, r)
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
	oldLang, newLang := f.detect(s.File.OldMode, s.File.OldPath, old), f.detect(s.File.NewMode, s.File.NewPath, new)
	var oldLate, newLate bool
	if s.Highlights.Old, oldLate, err = f.highlightSide(ctx, s, filediff.OldSide, oldLang, old, len(d.Old.Lines)); err != nil {
		return err
	}
	if s.Highlights.New, newLate, err = f.highlightSide(ctx, s, filediff.NewSide, newLang, new, len(d.New.Lines)); err != nil {
		return err
	}
	e.late = oldLate || newLate
	s.Diff = d
	// A submodule's text is no code a server could work on, nor is a file
	// on disk without a commit.
	if s.File.OldPath != "" && s.File.OldMode != change.Gitlink && r.Comparison.Left.Kind() == change.KindCommit {
		s.LeftDoc = hover.Document{Commit: r.Comparison.Left.ID(), Path: s.File.OldPath, Lang: oldLang, Version: commitVersion, Text: old}
	}
	switch {
	case s.File.NewMode == change.Gitlink:
	case r.ReadsWorktree():
		f.worktreeDoc(s, e, s.File.NewPath, newLang, new)
	case r.Comparison.Right.Kind() == change.KindCommit && s.File.NewPath != "":
		s.RightDoc = hover.Document{Commit: r.Comparison.Right.ID(), Path: s.File.NewPath, Lang: newLang, Version: commitVersion, Text: new}
	case r.Comparison.Right.Kind() == change.KindPath && s.File.WorktreePath != "":
		f.worktreeDoc(s, e, s.File.WorktreePath, newLang, new)
	}
	return nil
}

// worktreeDoc makes the worktree file path the right document.
func (f *FileDiffs) worktreeDoc(s *filediff.Snapshot, e *cached, path string, l lang.Lang, text string) {
	e.sum = maphash.String(f.seed, text)
	s.RightDoc = hover.Document{Path: path, Lang: l, Version: f.versions.of(path, e.sum), Text: text}
}

// detect is the language of one side; a submodule's has none.
func (f *FileDiffs) detect(mode change.Mode, path, text string) lang.Lang {
	if mode == change.Gitlink {
		return ""
	}
	return f.languages.Detect(path, text)
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

func (f *FileDiffs) readOld(ctx context.Context, r filediff.Request) (string, *filediff.Notice, error) {
	file := r.File
	switch {
	case file.OldPath == "":
		return "", nil, nil
	case r.Comparison.Left.Kind() == change.KindPath:
		data, _, err := f.paths.ReadPath(ctx, r.Comparison.Left.At(file.OldPath), f.max)
		return asText(filediff.OldSide, data, err)
	case file.OldMode == change.Gitlink:
		return change.SubprojectText(file.OldID, false), nil, nil
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
	switch {
	case r.Comparison.Right.Kind() == change.KindPath:
		data, stamp, err = f.paths.ReadPath(ctx, r.Comparison.Right.At(r.File.NewPath), f.max)
	case r.File.NewMode == change.Gitlink && r.ReadsWorktree():
		return f.readSubmodule(ctx, r.File.NewPath)
	case r.File.NewMode == change.Gitlink:
		return change.SubprojectText(r.File.NewID, false), change.Stamp{}, nil, nil
	case r.ReadsWorktree():
		data, stamp, err = f.worktree.ReadWorktree(ctx, r.File.NewPath, f.max)
	default:
		data, err = f.blobs.ReadBlob(ctx, r.File.NewID, f.max)
	}
	text, n, err := asText(filediff.NewSide, data, err)
	return text, stamp, n, err
}

func (f *FileDiffs) readSubmodule(ctx context.Context, path string) (string, change.Stamp, *filediff.Notice, error) {
	commit, dirty, err := f.submodules.ReadSubmodule(ctx, path)
	if err != nil {
		return "", change.Stamp{}, nil, err
	}
	return change.SubprojectText(commit, dirty), change.Stamp{}, nil, nil
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
