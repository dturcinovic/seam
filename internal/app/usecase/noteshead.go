package usecase

import (
	"context"
	"fmt"
	"slices"

	"github.com/dturcinovic/seam/internal/domain/note"
)

// What HEAD stands on decides which notes belong here (note.Away). The
// worker of Notes keeps it as cur and reads it again before a job once the
// watcher on HEAD's file saw a checkout, so that a new branch at the same
// commit, which changes no side the TUI would place again, is seen too.
// A HEAD kept in a reftable leaves that file alone; for it only the
// changes that keep HEAD read HEAD afresh, Add and Send, so that no note is
// stored under the branch left. While HEAD cannot be read it is read before
// every job, the TUI shows every note, and Add and Send refuse: a note
// without its branch would show on every branch for good.

// changeAtHead queues a change that keeps HEAD, read just before it.
func (n *Notes) changeAtHead(do func(*note.Book) error) note.Op {
	return n.submitChange(noteJob{change: do, atHead: true})
}

// readHead reads what HEAD stands on, when it may have changed or force
// asks for it. When it did change, other notes are away, and the status
// goes out anew; before the book was first read the load does that.
func (n *Notes) readHead(ctx context.Context, force bool) {
	n.mu.Lock()
	due, loading := force || n.headStale || !n.headLive || n.headErr != nil, n.stale
	n.headStale = false
	n.mu.Unlock()
	if !due {
		return
	}
	cur, err := n.head.ReadHead(ctx)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		err = fmt.Errorf("read HEAD: %w", err)
	}
	changed := cur != n.cur || (err == nil) != (n.headErr == nil)
	n.cur, n.headErr = cur, err
	if changed && !loading {
		n.publishStatus()
	}
}

// watchHead runs the watcher on HEAD's file. Once it stops, every job
// reads HEAD; the watcher logged why.
func (n *Notes) watchHead(ctx context.Context) {
	_ = n.headWatcher.WatchHead(ctx, n.headFile, headListener{n}) // logged by the watcher
	n.mu.Lock()
	n.headLive = false
	n.mu.Unlock()
}

// headListener hears the watcher on HEAD: a checkout may have moved it.
type headListener struct{ n *Notes }

func (l headListener) Changed() {
	l.n.mu.Lock()
	l.n.headLive, l.n.headStale = true, true
	l.n.mu.Unlock()
	notify(l.n.wake)
}

// gone are the branches notes belong to that no longer exist; none when
// the branches cannot be listed, as then no branch is known to be gone.
func (n *Notes) gone(ctx context.Context) []string {
	var noted []string
	for _, nt := range n.book.Notes {
		if b := nt.On.Branch; b != "" && !slices.Contains(noted, b) {
			noted = append(noted, b)
		}
	}
	if len(noted) == 0 {
		return nil
	}
	branches, err := n.head.ListBranches(ctx)
	if err != nil {
		return nil
	}
	return slices.DeleteFunc(noted, func(b string) bool { return slices.Contains(branches, b) })
}
