package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"seam/internal/app/port/in"
	"seam/internal/supervise"
)

// bridge wakes the program when a port has updates: one goroutine per
// signal sends a message for each. Only these goroutines wait on the
// program, never a port; and a signal set while one waits covers any number
// of updates, as Update drains them all.
type bridge struct {
	group *supervise.Group
}

// newBridge returns a bridge whose goroutines report panics to panics. A
// panic there would leave the screen without updates, but the program and
// its keys keep working, so there is no owner to tell beyond the log.
func newBridge(panics supervise.PanicFunc) *bridge {
	return &bridge{group: supervise.NewGroup("bridge", panics, nil)}
}

// start forwards the signals of the ports to send, which is the program's
// Send: it blocks until the program takes the message, and returns at once
// once the program has ended.
func (b *bridge) start(send func(tea.Msg), files in.FileDiffLoader, reload in.ChangesetReloader) error {
	if err := b.watch("file diffs", files.Signal(), send, fileDiffsReady{}); err != nil {
		return err
	}
	return b.watch("changeset", reload.Signal(), send, changesetReady{})
}

// stop ends the goroutines; after the program has ended none is held up in
// send.
func (b *bridge) stop(ctx context.Context) error {
	return b.group.Stop(ctx)
}

func (b *bridge) watch(task string, signal <-chan struct{}, send func(tea.Msg), msg tea.Msg) error {
	return b.group.Go(task, func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-signal:
				send(msg)
			}
		}
	})
}
