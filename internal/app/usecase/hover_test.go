package usecase_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/out/outfakes"
	"seam/internal/app/usecase"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
	"seam/internal/testsupport"
)

type hoverFakes struct {
	docs        *outfakes.FakeDocumentSync
	requester   *outfakes.FakeHoverRequester
	events      *outfakes.FakeServerEvents
	highlighter *outfakes.FakeHighlighter
	checkouts   *outfakes.FakeCommitCheckouts
	roots       *outfakes.FakeCheckoutRoots
	// signal is the events' signal, which a test sets after it gave Drain
	// something to return.
	signal chan struct{}
}

// newHovers starts the use case over fakes; it stops when the test ends.
// panics records the panics of its goroutines.
func newHovers(t *testing.T) (*usecase.Hovers, hoverFakes) {
	t.Helper()
	f := hoverFakes{
		&outfakes.FakeDocumentSync{}, &outfakes.FakeHoverRequester{}, &outfakes.FakeServerEvents{}, &outfakes.FakeHighlighter{},
		&outfakes.FakeCommitCheckouts{}, &outfakes.FakeCheckoutRoots{}, make(chan struct{}, 1),
	}
	f.events.SignalReturns(f.signal)
	f.docs.SyncReturns(gopls(), true)
	f.requester.RequestReturns(hover.Sent)
	h := usecase.NewHovers(usecase.HoverPorts{
		Docs: f.docs, Requester: f.requester, Events: f.events, Highlighter: f.highlighter, Checkouts: f.checkouts, Roots: f.roots,
	}, lang.Defaults(), nil)
	require.NoError(t, h.Start())
	t.Cleanup(func() { assert.NoError(t, h.Stop(context.Background())) })
	return h, f
}

// report has the servers report us.
func (f hoverFakes) report(us ...hover.Update) {
	f.events.DrainReturnsOnCall(f.events.DrainCallCount(), us)
	f.signal <- struct{}{}
}

// gopls is the instance of the fakes' documents.
func gopls() lang.Instance { return lang.Instance{Server: "gopls", Root: "."} }

func goDoc(path string, v hover.DocVersion) hover.Document {
	return hover.Document{Path: path, Lang: lang.Go, Version: v, Text: "package a\n"}
}

func done(req hover.ReqID) hover.Update {
	text := []hover.Span{{Text: "func a()"}}
	return hover.Update{Kind: hover.HoverDone, Req: req, Version: 1, Outcome: hover.Text, Content: hover.Content{Blocks: []hover.Block{{Kind: hover.Paragraph, Spans: text}}}}
}

func TestTrackOpensTheDocumentWithItsServer(t *testing.T) {
	h, f := newHovers(t)

	inst, ok := h.Track(goDoc("a.go", 1))

	require.True(t, ok)
	assert.Equal(t, gopls(), inst)
	require.Equal(t, 1, f.docs.SyncCallCount())
	assert.Equal(t, goDoc("a.go", 1), f.docs.SyncArgsForCall(0))
}

func TestADocumentWithoutPathGetsNoServer(t *testing.T) {
	h, f := newHovers(t)

	_, ok := h.Track(hover.Document{})

	assert.False(t, ok)
	assert.Zero(t, f.docs.SyncCallCount())
}

func TestTheDocumentsShownLastStayOpen(t *testing.T) {
	h, f := newHovers(t)
	for i := range 32 {
		h.Track(goDoc(fmt.Sprintf("%d.go", i), 1))
	}
	h.Track(goDoc("0.go", 2))
	assert.Zero(t, f.docs.CloseCallCount(), "thirty-two stay open")

	h.Track(goDoc("32.go", 1))

	require.Equal(t, 1, f.docs.CloseCallCount())
	assert.Equal(t, hover.DocKey{Path: "1.go"}, f.docs.CloseArgsForCall(0), "0.go was shown again since")
}

func TestUntrackClosesTheDocument(t *testing.T) {
	h, f := newHovers(t)
	h.Track(goDoc("a.go", 1))

	h.Untrack(hover.DocKey{Path: "a.go"})
	for i := range 32 {
		h.Track(goDoc(fmt.Sprintf("%d.go", i), 1))
	}

	require.Equal(t, 1, f.docs.CloseCallCount(), "a.go took no place since")
	assert.Equal(t, hover.DocKey{Path: "a.go"}, f.docs.CloseArgsForCall(0))
}

func TestADocumentWithoutServerIsNotKept(t *testing.T) {
	h, f := newHovers(t)
	f.docs.SyncReturns(lang.Instance{}, false)
	_, ok := h.Track(goDoc("a.go", 1))
	require.False(t, ok)
	f.docs.SyncReturns(gopls(), true)

	for i := range 32 {
		h.Track(goDoc(fmt.Sprintf("%d.go", i), 1))
	}

	assert.Zero(t, f.docs.CloseCallCount(), "a.go took no place")
}

func TestHoverAndCancelGoToTheServer(t *testing.T) {
	h, f := newHovers(t)
	f.requester.RequestReturns(hover.Busy)
	p := hover.Position{Req: 7, Path: "a.go", Version: 1, Line: 2, Byte: 3}

	assert.Equal(t, hover.Busy, h.Hover(p))
	h.CancelHover(7)

	assert.Equal(t, p, f.requester.RequestArgsForCall(0))
	assert.Equal(t, hover.ReqID(7), f.requester.CancelArgsForCall(0))
}

func TestOnlyTheAnswerToTheLastRequestIsHandedOn(t *testing.T) {
	h, f := newHovers(t)
	h.Hover(hover.Position{Req: 1, Path: "a.go", Version: 1})
	h.Hover(hover.Position{Req: 2, Path: "a.go", Version: 1})
	status := hover.Update{Kind: hover.StatusChanged, Instance: gopls(), Status: hover.Ready}

	f.report(done(1), status, done(2))

	got := testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) >= 2 })
	assert.Equal(t, []hover.Update{status, done(2)}, got)
}

// TestAnAnswerBeforeHoverReturnsIsKept: the server may answer, and the
// viewer drain it, while Request has not yet returned.
func TestAnAnswerBeforeHoverReturnsIsKept(t *testing.T) {
	h, f := newHovers(t)
	f.requester.RequestCalls(func(p hover.Position) hover.Submit {
		f.report(done(p.Req))
		testsupport.Until(t, h.Signal())
		return hover.Sent
	})

	require.Equal(t, hover.Sent, h.Hover(hover.Position{Req: 1, Path: "a.go", Version: 1}))

	assert.Equal(t, []hover.Update{done(1)}, h.Drain())
}
