package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/syntax"
	"github.com/dturcinovic/seam/internal/testsupport"
)

// withCode is the answer to req with a Go block and a paragraph.
func withCode(req hover.ReqID) hover.Update {
	u := done(req)
	u.Content.Blocks = append([]hover.Block{{Kind: hover.Code, Lang: lang.Go, Code: "func a()"}}, u.Content.Blocks...)
	return u
}

func keyword() syntax.Tokens {
	var b syntax.Builder
	_ = b.Add(len("func"), syntax.Keyword) // A kind and a run that are valid.
	b.EndLine(len("func a()"))
	return b.Tokens()
}

func ready() hover.Update {
	return hover.Update{Kind: hover.StatusChanged, Instance: gopls(), Status: hover.Ready}
}

func TestTheCodeOfAnAnswerArrivesColoured(t *testing.T) {
	h, f := newHovers(t)
	f.highlighter.HighlightReturns(keyword(), true)
	h.Hover(hover.Position{Req: 1, Path: "a.go", Version: 1})

	f.report(withCode(1))

	got := testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) == 1 })
	want := withCode(1)
	want.Content.Blocks[0].Tokens = keyword()
	assert.Equal(t, []hover.Update{want}, got)
	_, l, text := f.highlighter.HighlightArgsForCall(0)
	assert.Equal(t, lang.Go, l)
	assert.Equal(t, "func a()", text)
}

func TestAnAnswerWithoutCodeIsNotColoured(t *testing.T) {
	h, f := newHovers(t)
	h.Hover(hover.Position{Req: 1, Path: "a.go", Version: 1})
	plain := withCode(1)
	plain.Content.Blocks[0].Lang = ""

	f.report(done(1), plain)

	assert.Equal(t, []hover.Update{done(1), plain}, testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) == 2 }))
	assert.Zero(t, f.highlighter.HighlightCallCount())
}

// TestAHangingHighlighterHoldsUpNoStatus: while the colourer waits for a
// highlighter, the servers' status changes still arrive, and a new request
// gives the answer up, which the highlighter notices by its context.
func TestAHangingHighlighterHoldsUpNoStatus(t *testing.T) {
	h, f := newHovers(t)
	started, gaveUp := make(chan struct{}), make(chan struct{})
	f.highlighter.HighlightCalls(func(ctx context.Context, _ lang.Lang, _ string) (syntax.Tokens, bool) {
		close(started)
		<-ctx.Done()
		close(gaveUp)
		return syntax.Tokens{}, true
	})
	h.Hover(hover.Position{Req: 1, Path: "a.go", Version: 1})
	f.report(withCode(1))
	<-started

	f.report(ready())
	assert.Equal(t, []hover.Update{ready()}, testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) == 1 }))

	h.Hover(hover.Position{Req: 2, Path: "a.go", Version: 1})
	testsupport.Until(t, gaveUp)
	f.report(done(2))
	assert.Equal(t, []hover.Update{done(2)}, testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) == 1 }),
		"nothing of the answer given up")
}

func TestACancelledHoverIsNotColouredOn(t *testing.T) {
	h, f := newHovers(t)
	gaveUp := make(chan struct{})
	f.highlighter.HighlightCalls(func(ctx context.Context, _ lang.Lang, _ string) (syntax.Tokens, bool) {
		<-ctx.Done()
		close(gaveUp)
		return syntax.Tokens{}, true
	})
	h.Hover(hover.Position{Req: 1, Path: "a.go", Version: 1})
	f.report(withCode(1))
	require.Eventually(t, func() bool { return f.highlighter.HighlightCallCount() == 1 }, testsupport.Guard, time.Millisecond)

	h.CancelHover(1)

	testsupport.Until(t, gaveUp)
	f.report(ready())
	assert.Equal(t, []hover.Update{ready()}, testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) == 1 }))
}

func TestAnAnswerNoLongerWaitedForIsNotColoured(t *testing.T) {
	h, f := newHovers(t)
	h.Hover(hover.Position{Req: 1, Path: "a.go", Version: 1})
	h.Hover(hover.Position{Req: 2, Path: "a.go", Version: 1})

	f.report(withCode(1), done(2))

	assert.Equal(t, []hover.Update{done(2)}, testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) == 1 }))
	assert.Zero(t, f.highlighter.HighlightCallCount())
}

// TestAFailedColourerAnswersUncoloured: after a panic while colouring,
// answers with code come without colours, and the rest as before.
func TestAFailedColourerAnswersUncoloured(t *testing.T) {
	h, f := newHovers(t)
	f.highlighter.HighlightCalls(func(context.Context, lang.Lang, string) (syntax.Tokens, bool) { panic("lexer") })
	h.Hover(hover.Position{Req: 1, Path: "a.go", Version: 1})

	f.report(withCode(1))
	assert.Equal(t, []hover.Update{withCode(1)}, testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) == 1 }))

	h.Hover(hover.Position{Req: 2, Path: "a.go", Version: 1})
	f.report(withCode(2), ready())
	assert.ElementsMatch(t, []hover.Update{withCode(2), ready()},
		testsupport.Collect(t, h, func(u []hover.Update) bool { return len(u) == 2 }))
	assert.Equal(t, 1, f.highlighter.HighlightCallCount(), "not coloured again")
}

// TestAFailedForwarderLeavesNoHoverWaiting: once taking the servers'
// updates has failed, no answer comes, so a hover is refused at once.
func TestAFailedForwarderLeavesNoHoverWaiting(t *testing.T) {
	h, f := newHovers(t)
	f.events.DrainCalls(func() []hover.Update { panic("drain") })
	f.signal <- struct{}{}

	require.Eventually(t, func() bool { return h.Hover(hover.Position{Req: 1, Path: "a.go", Version: 1}) == hover.Failed },
		testsupport.Guard, time.Millisecond)
}
