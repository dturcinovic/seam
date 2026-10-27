package usecase

import (
	"context"
	"sync"

	"github.com/dturcinovic/seam/internal/app/port/in"
	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/domain/note"
	"github.com/dturcinovic/seam/internal/supervise"
)

// Batches implements in.BatchFeed for an agent session. One goroutine
// reads the book after each change the watcher reports and hands on the
// batches sent to the listener's session. Each batch goes out once per
// process: a later change of the book, or the notes of a batch given back,
// tell nothing again; only a new batch does.
type Batches struct {
	group   *supervise.Group
	store   out.NoteStore
	watcher out.NoteWatcher
	file    string

	wake chan struct{}

	mu       sync.Mutex
	listener in.BatchListener

	// told are the batches handed on, the worker's alone.
	told map[note.ID]bool
}

// NewBatches returns the use case over its ports. Its worker and watcher
// run between Start and Stop.
func NewBatches(set NotesSettings, s out.NoteStore, w out.NoteWatcher, panics supervise.PanicFunc) *Batches {
	b := &Batches{store: s, watcher: w, file: set.File, wake: make(chan struct{}, 1), told: make(map[note.ID]bool)}
	b.group = supervise.NewGroup("batches", panics, nil)
	return b
}

// Start starts the worker and the watcher; outside a repository neither.
func (b *Batches) Start() error {
	if b.file == "" {
		return nil
	}
	if err := b.group.Go("worker", b.loop); err != nil {
		return err
	}
	return b.group.Go("watch", b.watch)
}

// Stop ends both and waits for them until ctx ends.
func (b *Batches) Stop(ctx context.Context) error {
	return b.group.Stop(ctx)
}

// Follow implements in.BatchFeed.
func (b *Batches) Follow(l in.BatchListener) {
	b.mu.Lock()
	b.listener = l
	b.mu.Unlock()
	notify(b.wake)
}

func (b *Batches) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.wake:
			b.tell(ctx)
		}
	}
}

// tell hands the listener the batches of its session not handed on yet.
func (b *Batches) tell(ctx context.Context) {
	b.mu.Lock()
	l := b.listener
	b.mu.Unlock()
	if l == nil {
		return
	}
	session, ok := l.Session()
	if !ok {
		return
	}
	book, err := b.store.Load(ctx)
	if err != nil {
		if ctx.Err() == nil {
			l.Unread(err)
		}
		return
	}
	for _, s := range book.Sendings(session) {
		if b.told[s.Batch.ID] {
			continue
		}
		b.told[s.Batch.ID] = true
		l.Sent(ctx, s)
	}
}

// watch runs the watcher. Once it stops, batches go out no more; the
// watcher logged why, and the notes still wait to be taken by hand.
func (b *Batches) watch(ctx context.Context) {
	_ = b.watcher.WatchNotes(ctx, b.file, batchWaker{b}) // logged by the watcher
}

// batchWaker hears the watcher: the book may hold a new batch.
type batchWaker struct{ b *Batches }

func (w batchWaker) Changed() { notify(w.b.wake) }
