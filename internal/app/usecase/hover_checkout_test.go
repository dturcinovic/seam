package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/testsupport"
)

func base(t *testing.T) change.ObjectID { return mustID(t, strings.Repeat("b", 40)) }

func baseDoc(t *testing.T) hover.Document {
	d := goDoc("a.go", 1)
	d.Commit = base(t)
	return d
}

func basePos(t *testing.T, req hover.ReqID) hover.Position {
	return hover.Position{Req: req, Commit: base(t), Path: "a.go", Version: 1}
}

func checkoutUpdate(t *testing.T, s hover.ServerStatus) hover.Update {
	return hover.Update{Kind: hover.CheckoutChanged, Commit: base(t), Status: s}
}

// gated has the checkout wait for release and come out in dir.
func (f hoverFakes) gated(dir string) (release chan struct{}) {
	release = make(chan struct{})
	f.checkouts.CheckoutCalls(func(ctx context.Context, _ change.ObjectID) (string, error) {
		select {
		case <-release:
			return dir, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	return release
}

func TestACommitsDocumentWaitsForItsCheckout(t *testing.T) {
	h, f := newHovers(t)

	_, ok := h.Track(baseDoc(t))

	assert.False(t, ok, "no server before the checkout")
	assert.Zero(t, f.docs.SyncCallCount())
	assert.Zero(t, f.checkouts.CheckoutCallCount(), "showing a commit's file checks out nothing")
}

func TestTheFirstHoverInACommitChecksItOut(t *testing.T) {
	h, f := newHovers(t)
	release := f.gated("/cache/base")
	h.Track(baseDoc(t))

	assert.Equal(t, hover.NotReady, h.Hover(basePos(t, 1)))
	assert.Equal(t, hover.NotReady, h.Hover(basePos(t, 2)))
	assert.Equal(t, []hover.Update{checkoutUpdate(t, hover.Starting)}, testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) > 0 }))
	assert.Zero(t, f.requester.RequestCallCount())
	close(release)

	assert.Equal(t, []hover.Update{checkoutUpdate(t, hover.Ready)}, testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) > 0 }))
	require.Equal(t, 1, f.checkouts.CheckoutCallCount(), "once per commit")
	_, id := f.checkouts.CheckoutArgsForCall(0)
	assert.Equal(t, base(t), id)
	require.Equal(t, 1, f.roots.MountCallCount())
	id, dir := f.roots.MountArgsForCall(0)
	assert.Equal(t, base(t), id)
	assert.Equal(t, "/cache/base", dir)
	require.Equal(t, 1, f.docs.SyncCallCount(), "the document waiting goes to its server")
	assert.Equal(t, baseDoc(t), f.docs.SyncArgsForCall(0))

	assert.Equal(t, hover.Sent, h.Hover(basePos(t, 3)))
	assert.Equal(t, basePos(t, 3), f.requester.RequestArgsForCall(0))
	inst, ok := h.Track(baseDoc(t))
	require.True(t, ok)
	assert.Equal(t, gopls(), inst, "once checked out, Track names the server's instance")
}

func TestAHoverInADocumentNotShownChecksOutNothing(t *testing.T) {
	h, f := newHovers(t)

	assert.Equal(t, hover.Unsupported, h.Hover(basePos(t, 1)))
	assert.Zero(t, f.checkouts.CheckoutCallCount())
}

func TestACommitsDocumentWithoutServerIsNotShown(t *testing.T) {
	h, f := newHovers(t)
	d := baseDoc(t)
	d.Lang = "brainfuck"

	_, ok := h.Track(d)

	assert.False(t, ok)
	assert.Equal(t, hover.Unsupported, h.Hover(basePos(t, 1)))
	assert.Zero(t, f.checkouts.CheckoutCallCount())
}

func TestAFailedCheckoutStaysFailed(t *testing.T) {
	h, f := newHovers(t)
	f.checkouts.CheckoutReturns("", errors.New("disk full"))
	h.Track(baseDoc(t))

	assert.Equal(t, hover.NotReady, h.Hover(basePos(t, 1)))

	got := testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) >= 2 })
	assert.Equal(t, []hover.Update{checkoutUpdate(t, hover.Starting), checkoutUpdate(t, hover.ServerFailed)}, got)
	assert.Equal(t, hover.Unsupported, h.Hover(basePos(t, 2)))
	assert.Equal(t, 1, f.checkouts.CheckoutCallCount())
	assert.Zero(t, f.roots.MountCallCount())
}

func TestStopCancelsACheckout(t *testing.T) {
	h, f := newHovers(t)
	f.gated("/never")
	h.Track(baseDoc(t))
	h.Hover(basePos(t, 1))
	testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) > 0 })

	require.NoError(t, h.Stop(t.Context()))

	assert.Zero(t, f.roots.MountCallCount())
}

func TestDocumentsWaitingForACheckoutTakeNoRoomFromTheOpenOnes(t *testing.T) {
	h, f := newHovers(t)
	for i := range 32 {
		h.Track(goDoc(fmt.Sprintf("%d.go", i), 1))
	}

	h.Track(baseDoc(t))

	assert.Zero(t, f.docs.CloseCallCount())
}
