package main

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"seam/internal/adapter/driving/tui"
	"seam/internal/app/port/in"
	"seam/internal/app/port/out"
	"seam/internal/app/port/out/outfakes"
	"seam/internal/app/usecase"
	"seam/internal/domain/change"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
	"seam/internal/domain/limits"
	"seam/internal/domain/syntax"
	"seam/internal/domain/ui"
	"seam/internal/supervise"
	"seam/internal/testsupport"
)

// The tests here run the TUI on the real graph of the use cases, with fakes
// behind the outgoing ports, keys from a pipe and the screen discarded.

type outFakes struct {
	revisions *outfakes.FakeRevisionResolver
	changes   *outfakes.FakeChangeLister
	untracked *outfakes.FakeUntrackedLister
	blobs     *outfakes.FakeBlobReader
	worktree  *outfakes.FakeWorktreeReader
	stats     *outfakes.FakeWorktreeStatter
	highlight *outfakes.FakeHighlighter
	docs      *outfakes.FakeDocumentSync
	requester *outfakes.FakeHoverRequester
	events    *outfakes.FakeServerEvents
	checkouts *outfakes.FakeCommitCheckouts
	roots     *outfakes.FakeCheckoutRoots
	paths     *outfakes.FakeWatchPathsReader
	ignores   *outfakes.FakeIgnoreChecker
	watcher   *outfakes.FakeChangeWatcher
}

func newOutFakes(t *testing.T) outFakes {
	t.Helper()
	f := outFakes{
		&outfakes.FakeRevisionResolver{}, &outfakes.FakeChangeLister{}, &outfakes.FakeUntrackedLister{},
		&outfakes.FakeBlobReader{}, &outfakes.FakeWorktreeReader{}, &outfakes.FakeWorktreeStatter{},
		&outfakes.FakeHighlighter{}, &outfakes.FakeDocumentSync{}, &outfakes.FakeHoverRequester{}, &outfakes.FakeServerEvents{},
		&outfakes.FakeCommitCheckouts{}, &outfakes.FakeCheckoutRoots{},
		&outfakes.FakeWatchPathsReader{}, &outfakes.FakeIgnoreChecker{}, &outfakes.FakeChangeWatcher{},
	}
	f.events.SignalReturns(make(chan struct{}))
	head, err := change.ParseObjectID("0123456789abcdef0123456789abcdef01234567")
	require.NoError(t, err)
	f.revisions.ResolveCommitReturns(head, nil)
	return f
}

// startGraph starts the use cases over f, loads the changeset as session
// does and returns what the TUI calls.
func startGraph(t *testing.T, f outFakes) (*fx.App, tui.Ports) {
	t.Helper()
	var (
		ports  tui.Ports
		loader in.ChangesetLoader
	)
	app := fx.New(
		fx.Provide(
			func() out.RevisionResolver { return f.revisions },
			func() out.ChangeLister { return f.changes },
			func() out.UntrackedLister { return f.untracked },
			func() out.BlobReader { return f.blobs },
			func() out.WorktreeReader { return f.worktree },
			func() out.WorktreeStatter { return f.stats },
			func() out.Highlighter { return f.highlight },
			func() out.DocumentSync { return f.docs },
			func() out.HoverRequester { return f.requester },
			func() out.ServerEvents { return f.events },
			func() out.CommitCheckouts { return f.checkouts },
			func() out.CheckoutRoots { return f.roots },
			func() out.WatchPathsReader { return f.paths },
			func() out.IgnoreChecker { return f.ignores },
			func() out.ChangeWatcher { return f.watcher },
			func() supervise.PanicFunc { return nil },
		),
		fx.Supply(limits.DefaultFileSize, lang.Defaults()),
		usecase.Module,
		fx.NopLogger,
		fx.Populate(&ports.Files, &ports.Stream, &ports.Reload, &ports.Docs, &ports.LSP, &ports.Hover, &ports.Watch, &loader),
	)
	require.NoError(t, app.Err())
	require.NoError(t, app.Start(t.Context()))
	_, err := loader.LoadChangeset(t.Context(), changeset().Comparison)
	require.NoError(t, err)
	return app, ports
}

func changeset() change.Changeset {
	files := []change.ChangedFile{
		{Status: change.Modified, OldPath: "a.go", NewPath: "a.go", OldMode: 0o100644, NewMode: 0o100644},
		{Status: change.Modified, OldPath: "b.go", NewPath: "b.go", OldMode: 0o100644, NewMode: 0o100644},
	}
	return change.Changeset{Comparison: change.Comparison{Left: change.Commit("HEAD"), Right: change.Worktree()}, Files: files}
}

// runTUI starts the TUI on cs, side by side on 80 columns, writing to out,
// and returns a function that types keys and one that waits for it to end,
// failing after testsupport.Guard.
func runTUI(t *testing.T, ports tui.Ports, cs change.Changeset, out io.Writer) (typeKeys func(string), wait func() error) {
	t.Helper()
	keys, w := io.Pipe()
	t.Cleanup(func() { _ = w.Close() }) // Closing a pipe end does not fail.
	set := ui.DefaultSettings()
	set.Layout = ui.SplitLayout
	done := make(chan error, 1)
	go func() {
		done <- tui.Run(context.Background(), cs, set, ports, nil, tui.IO{In: keys, Out: out, Width: 80, Height: 24})
	}()
	typeKeys = func(s string) {
		_, err := io.WriteString(w, s)
		require.NoError(t, err)
	}
	wait = func() error {
		select {
		case err := <-done:
			return err
		case <-time.After(testsupport.Guard):
			t.Fatalf("the TUI did not end within %v", testsupport.Guard)
			return nil
		}
	}
	return typeKeys, wait
}

// blockUntilCancelled makes a fake read wait for its context, as a
// cooperating reader does; started is closed when the first read begins.
func blockUntilCancelled(f outFakes) (started chan struct{}) {
	started = make(chan struct{})
	f.blobs.ReadBlobCalls(func(ctx context.Context, _ change.ObjectID, _ limits.FileSize) ([]byte, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	return started
}

func TestQuitWhileACooperatingLoadRuns(t *testing.T) {
	f := newOutFakes(t)
	started := blockUntilCancelled(f)
	app, ports := startGraph(t, f)
	typeKeys, wait := runTUI(t, ports, changeset(), io.Discard)
	<-started

	begin := time.Now()
	typeKeys("q")
	require.NoError(t, wait())
	assert.Less(t, time.Since(begin), time.Second, "the terminal is free at once")

	begin = time.Now()
	require.NoError(t, stopGraph(app))
	assert.Less(t, time.Since(begin), stopBudget)
}

func TestQuitWhileAStuckLoadRuns(t *testing.T) {
	f := newOutFakes(t)
	started, release := make(chan struct{}), make(chan struct{})
	// goleak in TestMain checks that the worker ends once released, also
	// when an assertion below ends the test early.
	defer close(release)
	f.blobs.ReadBlobCalls(func(context.Context, change.ObjectID, limits.FileSize) ([]byte, error) {
		close(started)
		<-release
		return nil, nil
	})
	app, ports := startGraph(t, f)
	typeKeys, wait := runTUI(t, ports, changeset(), io.Discard)
	<-started

	typeKeys("q")
	require.NoError(t, wait())
	begin := time.Now()
	err := stopGraph(app)

	assert.Less(t, time.Since(begin), stopBudget+500*time.Millisecond, "stop keeps its deadline")
	var timeout *supervise.StopTimeout
	require.ErrorAs(t, err, &timeout, "the stuck worker is reported")
	assert.Equal(t, []string{"worker"}, timeout.Tasks)
}

// TestASignalEndsTheTUIWithoutError: main turns SIGINT and SIGTERM into the
// end of ctx, and that is a normal end.
func TestASignalEndsTheTUIWithoutError(t *testing.T) {
	f := newOutFakes(t)
	app, ports := startGraph(t, f)
	defer func() { assert.NoError(t, stopGraph(app)) }()
	keys, w := io.Pipe()
	defer func() { _ = w.Close() }() // Closing a pipe end does not fail.
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- tui.Run(ctx, changeset(), ui.DefaultSettings(), ports, nil, tui.IO{In: keys, Out: io.Discard, Width: 80, Height: 24})
	}()

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(testsupport.Guard):
		t.Fatal("the TUI did not end")
	}
}

func TestBlockedPortsHoldUpNoKey(t *testing.T) {
	f := newOutFakes(t)
	release := make(chan struct{})
	defer close(release)
	f.blobs.ReadBlobCalls(func(context.Context, change.ObjectID, limits.FileSize) ([]byte, error) {
		<-release
		return nil, nil
	})
	f.stats.StatWorktreeCalls(func(context.Context, string) (change.Stamp, error) {
		<-release
		return change.Stamp{}, nil
	})
	f.changes.ListChangesCalls(func(context.Context, change.Comparison) ([]change.ChangedFile, error) {
		if f.changes.ListChangesCallCount() > 1 {
			<-release
		}
		return nil, nil
	})
	app, ports := startGraph(t, f)
	typeKeys, wait := runTUI(t, ports, changeset(), io.Discard)

	begin := time.Now()
	typeKeys("jj\t]r\x1b[Bfnpq")
	require.NoError(t, wait())

	// The reload reached git and hangs there, on the worker: the keys after
	// it, q among them, went through all the same.
	assert.Less(t, time.Since(begin), time.Second)
	assert.Eventually(t, func() bool { return f.changes.ListChangesCallCount() == 2 }, testsupport.Guard, time.Millisecond,
		"the first load and the reload")
	t.Cleanup(func() { _ = stopGraph(app) }) // The blocked fakes are released first.
}

// TestAStuckHighlighterHoldsUpNoKey: an answer's code waits for a
// highlighter that does not return; keys, a status change after it and q
// go through all the same.
func TestAStuckHighlighterHoldsUpNoKey(t *testing.T) {
	f := newOutFakes(t)
	started, release := make(chan struct{}), make(chan struct{})
	f.highlight.HighlightCalls(func(_ context.Context, _ lang.Lang, text string) (syntax.Tokens, bool) {
		// The files shown are highlighted through the same port.
		if text == "func probe()" {
			close(started)
			<-release
		}
		return syntax.Tokens{}, true
	})
	signal := make(chan struct{}, 1)
	f.events.SignalReturns(signal)
	code := hover.Content{Blocks: []hover.Block{{Kind: hover.Code, Lang: lang.Go, Code: "func probe()"}}}
	f.events.DrainReturnsOnCall(0, []hover.Update{{Kind: hover.HoverDone, Outcome: hover.Text, Content: code}})
	f.events.DrainReturnsOnCall(1, []hover.Update{{Kind: hover.StatusChanged, Instance: lang.Instance{Server: "gopls", Root: "."}, Status: hover.Ready}})
	app, ports := startGraph(t, f)
	signal <- struct{}{}
	<-started
	typeKeys, wait := runTUI(t, ports, changeset(), io.Discard)

	begin := time.Now()
	signal <- struct{}{}
	typeKeys("jj\t]\x1b[Bfnpq")
	require.NoError(t, wait())

	assert.Less(t, time.Since(begin), time.Second)
	assert.Eventually(t, func() bool { return f.events.DrainCallCount() >= 2 }, testsupport.Guard, time.Millisecond,
		"the status change was taken while the highlighter hangs")
	close(release)
	assert.NoError(t, stopGraph(app))
}
