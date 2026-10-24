package usecase

import (
	"context"
	"time"

	"go.uber.org/fx"

	"seam/internal/app/port/in"
	"seam/internal/app/port/out"
	"seam/internal/domain/lang"
	"seam/internal/domain/limits"
	"seam/internal/supervise"
)

// Module provides the use cases as their incoming ports and runs their
// workers between start and stop.
var Module = fx.Module("usecase",
	fx.Provide(
		fx.Annotate(newChangesets, fx.As(new(in.ChangesetLoader)), fx.As(new(in.ChangesetReloader))),
		fx.Annotate(newFileDiffs, fx.As(new(in.FileDiffLoader))),
		fx.Annotate(newHovers, fx.As(new(in.Hoverer)), fx.As(new(in.DocumentTracker)), fx.As(new(in.LSPUpdates))),
		fx.Annotate(newWatches, fx.As(new(in.WatchStarter)), fx.As(new(in.WatchUpdates))),
	),
)

// stopReserve is left of the stop deadline for the hooks after the
// workers': git, whose processes the workers read through, stops last.
const stopReserve = 300 * time.Millisecond

func newChangesets(lc fx.Lifecycle, r out.RevisionResolver, c out.ChangeLister, u out.UntrackedLister, panics supervise.PanicFunc) *Changesets {
	s := NewChangesets(r, c, u, panics)
	runWorker(lc, s)
	return s
}

func newFileDiffs(lc fx.Lifecycle, b out.BlobReader, w out.WorktreeReader, st out.WorktreeStatter, h out.Highlighter, languages lang.Table, max limits.FileSize, panics supervise.PanicFunc) *FileDiffs {
	f := NewFileDiffs(b, w, st, h, languages, max, panics)
	runWorker(lc, f)
	return f
}

// runWorker runs a use case's worker from start to stop.
func runWorker(lc fx.Lifecycle, w interface {
	Start() error
	Stop(context.Context) error
},
) {
	lc.Append(fx.StartStopHook(w.Start, reserved(w.Stop)))
}

// reserved is stop with stopReserve of its deadline left to the hooks after
// it.
func reserved(stop func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		// With less left than the reserve the worker keeps what there is: a
		// deadline already past would report a cooperating worker as stuck.
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) > stopReserve {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, deadline.Add(-stopReserve))
			defer cancel()
		}
		return stop(ctx)
	}
}

// newWatches stops the watcher with the workers; it starts with Watch.
func newWatches(lc fx.Lifecycle, p out.WatchPathsReader, i out.IgnoreChecker, w out.ChangeWatcher, panics supervise.PanicFunc) *Watches {
	ws := NewWatches(p, i, w, panics)
	lc.Append(fx.StopHook(reserved(ws.Stop)))
	return ws
}

// hoverPorts gathers the ports of Hovers from the graph.
type hoverPorts struct {
	fx.In

	Docs        out.DocumentSync
	Requester   out.HoverRequester
	Events      out.ServerEvents
	Highlighter out.Highlighter
	Checkouts   out.CommitCheckouts
	Roots       out.CheckoutRoots
}

func newHovers(lc fx.Lifecycle, p hoverPorts, languages lang.Table, panics supervise.PanicFunc) *Hovers {
	hs := NewHovers(HoverPorts{
		Docs: p.Docs, Requester: p.Requester, Events: p.Events, Highlighter: p.Highlighter, Checkouts: p.Checkouts, Roots: p.Roots,
	}, languages, panics)
	runWorker(lc, hs)
	return hs
}
