package usecase

import (
	"context"
	"time"

	"go.uber.org/fx"

	"github.com/dturcinovic/seam/internal/app/port/in"
	"github.com/dturcinovic/seam/internal/app/port/out"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/limits"
	"github.com/dturcinovic/seam/internal/supervise"
)

// Module provides the use cases as their incoming ports and runs their
// workers between start and stop.
var Module = fx.Module("usecase",
	fx.Provide(
		fx.Annotate(newChangesets, fx.As(new(in.ChangesetLoader)), fx.As(new(in.ChangesetReloader))),
		fx.Annotate(newFileDiffs, fx.As(new(in.FileDiffLoader)), fx.As(fx.Self())),
		fx.Annotate(newStreams, fx.As(new(in.StreamLoader))),
		fx.Annotate(newHovers, fx.As(new(in.Hoverer)), fx.As(new(in.Definer)), fx.As(new(in.DocumentTracker)), fx.As(new(in.LSPUpdates))),
		fx.Annotate(newWatches, fx.As(new(in.WatchStarter)), fx.As(new(in.WatchUpdates))),
		fx.Annotate(newReleaseChecks, fx.As(new(in.ReleaseNotifier))),
		fx.Annotate(newNotes, fx.As(new(in.NoteWriter)), fx.As(new(in.NoteReader)), fx.As(new(in.NoteOverview)), fx.As(new(in.NoteReleaser)), fx.As(new(in.NoteDeleter)), fx.As(new(in.NoteEditor)), fx.As(new(in.NoteSender)), fx.As(new(in.NoteUpdates))),
		fx.Annotate(newBatches, fx.As(new(in.BatchFeed))),
		fx.Annotate(NewNoteAgents, fx.As(new(in.NoteSessions)), fx.As(new(in.NoteAnswerer)), fx.As(new(in.BatchTaker))),
	),
)

// stopReserve is left of the stop deadline for the hooks after the
// workers': git, whose processes the workers read through, stops last.
const stopReserve = 300 * time.Millisecond

func newChangesets(lc fx.Lifecycle, r out.RevisionResolver, c out.ChangeLister, u out.UntrackedLister, p out.PathLister, l out.WorktreeLocator, panics supervise.PanicFunc) *Changesets {
	s := NewChangesets(r, c, u, p, l, panics)
	runWorker(lc, s)
	return s
}

func newFileDiffs(lc fx.Lifecycle, b out.BlobReader, w out.WorktreeReader, sub out.SubmoduleReader, st out.WorktreeStatter, paths out.PathReader, formats out.ObjectFormatReader, h out.Highlighter, languages lang.Table, max limits.FileSize, panics supervise.PanicFunc) *FileDiffs {
	f := NewFileDiffs(b, w, sub, st, paths, formats, h, languages, max, panics)
	runWorker(lc, f)
	return f
}

func newStreams(lc fx.Lifecycle, f *FileDiffs, panics supervise.PanicFunc) *Streams {
	s := NewStreams(f, panics)
	runWorker(lc, s)
	return s
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

// newReleaseChecks cancels a check still running with the workers; it
// starts with Check.
func newReleaseChecks(lc fx.Lifecycle, settings ReleaseSettings, f out.ReleaseFetcher, s out.ReleaseStore, panics supervise.PanicFunc) *ReleaseChecks {
	r := NewReleaseChecks(settings, f, s, panics)
	lc.Append(fx.StopHook(reserved(r.Stop)))
	return r
}

func newNotes(lc fx.Lifecycle, set NotesSettings, s out.NoteStore, w out.NoteWatcher, p out.ProcessProber, panics supervise.PanicFunc) *Notes {
	n := NewNotes(set, s, w, p, panics)
	runWorker(lc, n)
	return n
}

func newBatches(lc fx.Lifecycle, set NotesSettings, s out.NoteStore, w out.NoteWatcher, panics supervise.PanicFunc) *Batches {
	b := NewBatches(set, s, w, panics)
	runWorker(lc, b)
	return b
}

// hoverPorts gathers the ports of Hovers from the graph.
type hoverPorts struct {
	fx.In

	Docs        out.DocumentSync
	Requester   out.HoverRequester
	Definitions out.DefinitionRequester
	Events      out.ServerEvents
	Highlighter out.Highlighter
	Checkouts   out.CommitCheckouts
	Roots       out.CheckoutRoots
}

func newHovers(lc fx.Lifecycle, p hoverPorts, languages lang.Table, panics supervise.PanicFunc) *Hovers {
	hs := NewHovers(HoverPorts{
		Docs: p.Docs, Requester: p.Requester, Definitions: p.Definitions, Events: p.Events, Highlighter: p.Highlighter, Checkouts: p.Checkouts, Roots: p.Roots,
	}, languages, panics)
	runWorker(lc, hs)
	return hs
}
