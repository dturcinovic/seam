package usecase

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/supervise"
)

// NotesSettings names the book's file, empty outside a repository, where
// there are no notes, and git's HEAD file of the worktree, to see a
// checkout; without it HEAD is read each time the worker wakes.
type NotesSettings struct {
	File string
	Head string
}

// Notes implements in.NoteWriter, in.NoteReader, in.NoteOverview,
// in.NoteReleaser, in.NoteDeleter, in.NoteEditor, in.NoteSender and
// in.NoteUpdates. One goroutine reads, places and changes the book, in the
// order asked; a change from another process, which the watcher reports,
// reloads it before the next job. What HEAD stands on decides which notes
// belong here; see noteshead.go.
type Notes struct {
	group       *supervise.Group
	store       out.NoteStore
	watcher     out.NoteWatcher
	headWatcher out.HeadWatcher
	procs       out.ProcessProber
	head        out.HeadReader
	file        string
	headFile    string

	wake   chan struct{}
	signal chan struct{}

	mu    sync.Mutex
	jobs  []noteJob
	stale bool
	live  bool
	// headStale asks for HEAD to be read before the next job; headLive
	// says the watcher on HEAD runs, else every job reads it.
	headStale bool
	headLive  bool
	lastOp    note.Op
	failed    error
	updates   []note.Update

	// The book as last read or written, and HEAD as last read, the
	// worker's alone. While HEAD cannot be read, cur is zero and no note
	// is away: rather all notes shown than some hidden for no reason.
	book    note.Book
	bookErr error
	cur     change.Head
	headErr error
}

// NewNotes returns the use case over its ports. Its worker and watcher run
// between Start and Stop.
func NewNotes(set NotesSettings, s out.NoteStore, w out.NoteWatcher, hw out.HeadWatcher, p out.ProcessProber, h out.HeadReader, panics supervise.PanicFunc) *Notes {
	n := &Notes{
		store: s, watcher: w, headWatcher: hw, procs: p, head: h, file: set.File, headFile: set.Head,
		wake: make(chan struct{}, 1), signal: make(chan struct{}, 1), stale: true, headStale: true,
	}
	n.group = supervise.NewGroup("notes", panics, n.fail)
	return n
}

// Start starts the worker and the watchers; outside a repository none.
func (n *Notes) Start() error {
	if n.file == "" {
		return nil
	}
	if err := n.group.Go("worker", n.loop); err != nil {
		return err
	}
	if err := n.group.Go("watch", n.watch); err != nil {
		return err
	}
	if n.headFile == "" {
		return nil
	}
	return n.group.Go("watch HEAD", n.watchHead)
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

// Add implements in.NoteWriter; the note belongs to what HEAD stands on.
func (n *Notes) Add(d note.Draft) note.Op {
	return n.changeAtHead(func(b *note.Book) error {
		if err := n.headErr; err != nil && d.Anchor.Origin.Kind != change.KindCommit {
			return err
		}
		d.On = n.cur
		return b.Add(d, newID(), time.Now())
	})
}

// Resolve implements in.NoteWriter.
func (n *Notes) Resolve(id note.ID) note.Op {
	return n.change(func(b *note.Book) error { return b.Resolve(id) })
}

// Release implements in.NoteReleaser.
func (n *Notes) Release(id note.ID) note.Op {
	return n.change(func(b *note.Book) error { return b.Release(id) })
}

// Delete implements in.NoteDeleter.
func (n *Notes) Delete(id note.ID) note.Op {
	return n.change(func(b *note.Book) error { return b.Delete(id) })
}

// Edit implements in.NoteEditor.
func (n *Notes) Edit(id note.ID, k note.Kind, text string) note.Op {
	return n.change(func(b *note.Book) error { return b.Edit(id, k, text) })
}

// Send implements in.NoteSender; it sends only the notes of what HEAD
// stands on.
func (n *Notes) Send(session note.ID) note.Op {
	return n.changeAtHead(func(b *note.Book) error {
		if n.headErr != nil {
			return n.headErr
		}
		_, err := b.Send(newID(), session, time.Now(), n.procs.Alive, n.cur)
		return err
	})
}

// Place implements in.NoteReader.
func (n *Notes) Place(k note.SnapshotKey, lines []string) {
	n.submit(noteJob{key: k, lines: lines})
}

// Overview implements in.NoteOverview.
func (n *Notes) Overview() {
	n.submit(noteJob{overview: true})
}

// noteJob is a change of the book, named op, the overview, or else the
// placement of the notes of a side.
type noteJob struct {
	op     note.Op
	change func(*note.Book) error
	// atHead reads HEAD before the change, which keeps it.
	atHead bool

	overview bool

	key   note.SnapshotKey
	lines []string
}

// answer is the job's update without the book: a change failed with err,
// an overview has none, a placement found nothing.
func (j noteJob) answer(err error) note.Update {
	switch {
	case j.change != nil:
		return note.Update{Kind: note.Done, Op: j.op, Err: err}
	case j.overview:
		return note.Update{Kind: note.Overview, Err: err}
	}
	return note.Update{Kind: note.Placed, Key: j.key}
}

func (n *Notes) change(do func(*note.Book) error) note.Op {
	return n.submitChange(noteJob{change: do})
}

func (n *Notes) submitChange(j noteJob) note.Op {
	n.mu.Lock()
	n.lastOp++
	j.op = n.lastOp
	n.mu.Unlock()
	n.submit(j)
	return j.op
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
	if j.overview {
		n.publish(note.Update{Kind: note.Overview, Notes: slices.Clone(n.book.Notes), Err: n.bookErr, Gone: n.gone(ctx)})
		return
	}
	if j.change == nil {
		n.publish(note.Update{Kind: note.Placed, Key: j.key, Spots: n.spots(j.key, j.lines)})
		return
	}
	if j.atHead {
		n.readHead(ctx, true)
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

// spots are the notes of side k not resolved and not away, with where they
// stand in lines.
func (n *Notes) spots(k note.SnapshotKey, lines []string) []note.Spot {
	var spots []note.Spot
	for _, nt := range n.book.Notes {
		a := nt.Anchor
		if nt.State == note.Resolved || a.Path != k.Path || a.Origin != k.Origin || note.Away(nt, n.cur) {
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

// runNext reads HEAD and reloads the book if either may have changed, then
// runs the next job, if any. HEAD is checked for each job, so that one
// seen to move while jobs wait reaches those after it.
func (n *Notes) runNext(ctx context.Context) bool {
	n.readHead(ctx, false)
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
		heads = append(heads, note.Head{
			ID: nt.ID, Path: nt.Anchor.Path, Origin: nt.Anchor.Origin, Kind: nt.Kind, State: nt.State, Away: note.Away(nt, n.cur),
		})
	}
	n.mu.Lock()
	live := n.live
	n.mu.Unlock()
	n.publish(note.Update{
		Kind: note.StoreStatus, Heads: heads, Sessions: slices.Clone(n.book.Sessions), Targets: n.book.Targets(n.procs.Alive),
		Err: n.bookErr, HeadErr: n.headErr, Live: live,
	})
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
