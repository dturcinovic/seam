package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/testsupport"
)

func defined(req hover.ReqID) hover.Update {
	return hover.Update{Kind: hover.DefinitionDone, Req: req, Version: 1, Outcome: hover.Text, Target: hover.Location{Path: "b.go", Line: 4}, Targets: 1}
}

func TestDefineAndCancelGoToTheServer(t *testing.T) {
	h, f := newHovers(t)
	f.definitions.RequestDefinitionReturns(hover.Busy)
	p := hover.Position{Req: 7, Path: "a.go", Version: 1, Line: 2, Byte: 3}

	assert.Equal(t, hover.Busy, h.Define(p))
	h.CancelDefinition(7)

	assert.Equal(t, p, f.definitions.RequestDefinitionArgsForCall(0))
	assert.Equal(t, hover.ReqID(7), f.definitions.CancelDefinitionArgsForCall(0))
	assert.Zero(t, f.requester.RequestCallCount(), "no hover asked")
}

func TestOnlyTheAnswerToTheLastDefinitionIsHandedOn(t *testing.T) {
	h, f := newHovers(t)
	h.Define(hover.Position{Req: 1, Path: "a.go", Version: 1})
	h.Define(hover.Position{Req: 2, Path: "a.go", Version: 1})
	// A hover asked meanwhile does not replace the definition waited for.
	h.Hover(hover.Position{Req: 3, Path: "a.go", Version: 1})

	f.report(defined(1), defined(2), done(3))

	got := testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) >= 2 })
	assert.Equal(t, []hover.Update{defined(2), done(3)}, got)
}

func TestTheFirstDefinitionInACommitChecksItOut(t *testing.T) {
	h, f := newHovers(t)
	release := f.gated("/cache/base")
	h.Track(baseDoc(t))

	require.Equal(t, hover.NotReady, h.Define(basePos(t, 1)))
	assert.Equal(t, []hover.Update{checkoutUpdate(t, hover.Starting)}, testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) > 0 }))
	close(release)
	assert.Equal(t, []hover.Update{checkoutUpdate(t, hover.Ready)}, testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) > 0 }))

	assert.Equal(t, hover.Sent, h.Define(basePos(t, 2)))
	assert.Equal(t, 1, f.definitions.RequestDefinitionCallCount())
}
