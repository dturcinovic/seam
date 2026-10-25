package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"seam/internal/supervise"
)

// bridgeStop bounds stopping the bridge, counted from the call: once the
// program has ended its goroutines are not held up, so this is a safeguard.
const bridgeStop = time.Second

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
func (b *bridge) start(send func(tea.Msg), p Ports) error {
	if err := b.watch("file diffs", p.Files.Signal(), send, fileDiffsReady{}); err != nil {
		return err
	}
	if err := b.watch("stream", p.Stream.Signal(), send, streamReady{}); err != nil {
		return err
	}
	if err := b.watch("changeset", p.Reload.Signal(), send, changesetReady{}); err != nil {
		return err
	}
	if err := b.watch("lsp", p.LSP.Signal(), send, lspUpdated{}); err != nil {
		return err
	}
	return b.watch("watch", p.Watch.Signal(), send, watchUpdated{})
}

// stop ends the goroutines; after the program has ended none is held up in
// send. It sets its own deadline, so no caller can hand it one that started
// with the program and ran out since.
func (b *bridge) stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), bridgeStop)
	defer cancel()
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
