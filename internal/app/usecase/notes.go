package usecase

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/supervise"
)

// NotesSettings names the book's file; empty outside a repository, where
// there are no notes.
type NotesSettings struct {
	File string
}

// Notes implements in.NoteWriter, in.NoteReader and in.NoteUpdates. One
// goroutine reads, places and changes the book, in the order asked; a
// change from another process, which the watcher reports, reloads it
// before the next job.
type Notes struct {
	group   *supervise.Group
	store   out.NoteStore
	watcher out.NoteWatcher
	file    string

	wake   chan struct{}
	signal chan struct{}

	mu      sync.Mutex
	jobs    []noteJob
	stale   bool
	live    bool
	lastOp  note.Op
	failed  error
	updates []note.Update

	// The book as last read or written, the worker's alone.
	book    note.Book
	bookErr error
}

// NewNotes returns the use case over its ports. Its worker and watcher run
// between Start and Stop.
func NewNotes(set NotesSettings, s out.NoteStore, w out.NoteWatcher, panics supervise.PanicFunc) *Notes {
	n := &Notes{store: s, watcher: w, file: set.File, wake: make(chan struct{}, 1), signal: make(chan struct{}, 1), stale: true}
	n.group = supervise.NewGroup("notes", panics, n.fail)
	return n
}

// Start starts the worker and the watcher; outside a repository neither.
func (n *Notes) Start() error {
	if n.file == "" {
		return nil
	}
	if err := n.group.Go("worker", n.loop); err != nil {
		return err
	}
	return n.group.Go("watch", n.watch)
}

// Stop ends both and waits for them until ctx ends.
func (n *Notes) Stop(ctx context.Context) error {
	return n.group.Stop(ctx)
}

// Signal is set after each new update.
func (n *Notes) Signal() <-chan struct{} { return n.signal }

// Drain hands over the updates since the last call, oldest first.
func (n *Notes) Drain() []note.Update {
	n.mu.Lock()
	defer n.mu.Unlock()
	u := n.updates
	n.updates = nil
	return u
}

// Add implements in.NoteWriter.
func (n *Notes) Add(d note.Draft) note.Op {
	return n.change(func(b *note.Book) error {
		return b.Add(d, note.ID(rand.Text()), time.Now())
	})
}

// Resolve implements in.NoteWriter.
func (n *Notes) Resolve(id note.ID) note.Op {
	return n.change(func(b *note.Book) error { return b.Resolve(id) })
}

// Place implements in.NoteReader.
func (n *Notes) Place(k note.SnapshotKey, lines []string) {
	n.submit(noteJob{key: k, lines: lines})
}

// noteJob is a change of the book, named op, or else the placement of the
// notes of a side.
type noteJob struct {
	op     note.Op
	change func(*note.Book) error

	key   note.SnapshotKey
	lines []string
}

// answer is the job's update without the book: a change failed with err,
// a placement found nothing.
func (j noteJob) answer(err error) note.Update {
	if j.change != nil {
		return note.Update{Kind: note.Done, Op: j.op, Err: err}
	}
	return note.Update{Kind: note.Placed, Key: j.key}
}

func (n *Notes) change(do func(*note.Book) error) note.Op {
	n.mu.Lock()
	n.lastOp++
	op := n.lastOp
	n.mu.Unlock()
	n.submit(noteJob{op: op, change: do})
	return op
}

// submit queues a job; once the worker failed, or outside a repository,
// it answers at once.
func (n *Notes) submit(j noteJob) {
	n.mu.Lock()
	defer n.mu.Unlock()
	switch {
	case n.failed != nil:
		n.publishLocked(j.answer(n.failed))
	case n.file == "":
		n.publishLocked(j.answer(note.ErrNoBook))
	default:
		n.jobs = append(n.jobs, j)
		notify(n.wake)
	}
}

func (n *Notes) run(ctx context.Context, j noteJob) {
	if j.change == nil {
		n.publish(note.Update{Kind: note.Placed, Key: j.key, Spots: n.spots(j.key, j.lines)})
		return
	}
	var after note.Book
	err := n.store.Change(ctx, func(b *note.Book) error {
		if err := j.change(b); err != nil {
			return err
		}
		after = *b
		return nil
	})
	n.publish(j.answer(err))
	if err != nil {
		// The file may be what failed; the next job reads it again.
		n.markStale()
		return
	}
	n.book, n.bookErr = after, nil
	n.publishStatus()
}

// spots are the notes of side k not resolved, with where they stand in
// lines.
func (n *Notes) spots(k note.SnapshotKey, lines []string) []note.Spot {
	var spots []note.Spot
	for _, nt := range n.book.Notes {
		a := nt.Anchor
		if nt.State == note.Resolved || a.Path != k.Path || a.Origin != k.Origin {
			continue
		}
		spots = append(spots, note.Spot{Note: nt, Place: note.Locate(a, k.Blob, lines)})
	}
	return spots
}

func (n *Notes) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-n.wake:
		}
		for n.runNext(ctx) {
		}
	}
}

// runNext reloads the book if it is stale, then runs the next job, if
// any.
func (n *Notes) runNext(ctx context.Context) bool {
	n.mu.Lock()
	stale := n.stale
	n.stale = false
	job, ok := noteJob{}, len(n.jobs) > 0
	if ok {
		job, n.jobs = n.jobs[0], n.jobs[1:]
	}
	n.mu.Unlock()
	if stale {
		n.book, n.bookErr = n.store.Load(ctx)
		if ctx.Err() != nil {
			return false
		}
		n.publishStatus()
	}
	if !ok {
		return stale
	}
	n.run(ctx, job)
	return true
}

// watch runs the watcher. Once it stops, changes from elsewhere show only
// after the next change here; the status says so, and the watcher logged
// why.
func (n *Notes) watch(ctx context.Context) {
	_ = n.watcher.WatchNotes(ctx, n.file, noteListener{n}) // logged by the watcher
	if ctx.Err() != nil {
		return
	}
	n.mu.Lock()
	n.live = false
	n.mu.Unlock()
	n.markStale()
}

// noteListener hears the watcher: the book may have changed, and the
// watcher is live.
type noteListener struct{ n *Notes }

func (l noteListener) Changed() {
	l.n.mu.Lock()
	l.n.live = true
	l.n.mu.Unlock()
	l.n.markStale()
}

func (n *Notes) markStale() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stale = true
	notify(n.wake)
}

// publishStatus hands on the book in brief, as last read.
func (n *Notes) publishStatus() {
	heads := make([]note.Head, 0, len(n.book.Notes))
	for _, nt := range n.book.Notes {
		heads = append(heads, note.Head{ID: nt.ID, Path: nt.Anchor.Path, Origin: nt.Anchor.Origin, Kind: nt.Kind, State: nt.State})
	}
	n.mu.Lock()
	live := n.live
	n.mu.Unlock()
	n.publish(note.Update{Kind: note.StoreStatus, Heads: heads, Err: n.bookErr, Live: live})
}

func (n *Notes) publish(u note.Update) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.publishLocked(u)
}

func (n *Notes) publishLocked(u note.Update) {
	n.updates = append(n.updates, u)
	notify(n.signal)
}

// fail answers the jobs waiting and every later one, as the group runs
// no more.
func (n *Notes) fail(f supervise.Failure) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failed = fmt.Errorf("%w: %v", note.ErrNotesFailed, f.Value)
	n.publishLocked(note.Update{Kind: note.StoreStatus, Err: n.failed})
	for _, j := range n.jobs {
		n.publishLocked(j.answer(n.failed))
	}
	n.jobs = nil
}
