package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/in/infakes"
	"seam/internal/domain/change"
	"seam/internal/domain/diff"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
)

func baseCommit(t *testing.T) change.ObjectID {
	t.Helper()
	id, err := change.ParseObjectID(strings.Repeat("b", 40))
	require.NoError(t, err)
	return id
}

// leftModel shows HEAD's codeLine on the left of the worktree's "y", 40
// lines after both, on a screen of 80 by 24. Track names left as the
// instance serving the left side, none while it is zero; the right one is
// gopls's.
func leftModel(t *testing.T, left *lang.Instance) (model, hoverFakes) {
	t.Helper()
	head, err := change.Commit("HEAD").Resolved(baseCommit(t))
	require.NoError(t, err)
	c, err := change.NewComparison(head, change.Worktree())
	require.NoError(t, err)
	p := fakePorts()
	m := newModel(change.Changeset{Comparison: c, Files: files("a.go")}, p, splitSettings())
	m, _ = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	p.Docs.(*infakes.FakeDocumentTracker).TrackCalls(func(d hover.Document) (lang.Instance, bool) {
		if d.Commit.IsZero() {
			return gopls(), true
		}
		return *left, *left != lang.Instance{}
	})
	f := hoverFakes{p.Hover.(*infakes.FakeHoverer), p.LSP.(*infakes.FakeLSPUpdates)}
	f.hover.HoverReturns(hover.Sent)
	fl := p.Files.(*infakes.FakeFileDiffLoader)
	old, new := codeLine+"\n"+numbered(40), "y\n"+numbered(40)
	fl.DrainReturns([]filediff.Update{{Gen: fl.LoadArgsForCall(0).Gen, Snapshot: filediff.Snapshot{
		File: fl.LoadArgsForCall(0).File, Diff: diff.Compute(t.Context(), diff.Split(old), diff.Split(new)),
		LeftDoc:  hover.Document{Commit: baseCommit(t), Path: "a.go", Lang: lang.Go, Version: 1, Text: old},
		RightDoc: hover.Document{Path: "a.go", Lang: lang.Go, Version: 1, Text: new},
	}}})
	m, _ = send(m, fileDiffsReady{})
	return m, f
}

// restLeft lets the mouse rest on column col of codeLine on the left.
func restLeft(t *testing.T, m model, col int) model {
	t.Helper()
	from, _ := m.s.Layout.text(filediff.OldSide)
	m, cmd := send(m, tea.MouseMotionMsg{X: from + col, Y: bodyTop})
	require.NotNil(t, cmd)
	m, _ = send(m, hoverDue{Seq: m.s.Hover.Seq})
	return m
}

// checkout is the news of the left commit's checkout.
func checkout(t *testing.T, st hover.ServerStatus) hover.Update {
	return hover.Update{Kind: hover.CheckoutChanged, Commit: baseCommit(t), Status: st}
}

func TestTheMouseAsksOnTheLeftSide(t *testing.T) {
	var left lang.Instance
	m, f := leftModel(t, &left)
	require.Zero(t, m.s.LSP.Left, "no server before the checkout")
	require.True(t, m.s.hoverable(), "but the mouse may ask")

	m = restLeft(t, m, 9)

	require.Equal(t, 1, f.hover.HoverCallCount())
	assert.Equal(t, hover.Position{Req: 1, Commit: baseCommit(t), Path: "a.go", Version: 1, Line: 0, Byte: 6}, f.hover.HoverArgsForCall(0))
	m = answer(m, text(1, "func Println()"))
	assert.True(t, m.s.Hover.Popup.Open, "an answer about the left side's version opens the popup")
}

func TestALeftHoverWaitsForTheCheckoutAndItsServer(t *testing.T) {
	var left lang.Instance
	m, f := leftModel(t, &left)
	f.hover.HoverReturns(hover.NotReady)
	m = restLeft(t, m, 9)
	m = answer(m, checkout(t, hover.Starting))
	assert.True(t, strings.HasSuffix(footerLine(m), "HEAD: checkout…"), footerLine(m))

	left = lang.Instance{Server: "gopls", Commit: baseCommit(t), Root: "."}
	m = answer(m, checkout(t, hover.Ready), status(left, hover.Starting))

	assert.Equal(t, left, m.s.LSP.Left, "Track names the server once checked out")
	assert.Equal(t, 2, f.hover.HoverCallCount(), "asked again once checked out")
	assert.True(t, strings.HasSuffix(footerLine(m), "HEAD: gopls starting…"), footerLine(m))
	f.hover.HoverReturns(hover.Sent)
	m = answer(m, status(left, hover.Ready))
	assert.Equal(t, 3, f.hover.HoverCallCount(), "asked again once its server is ready")
	assert.True(t, strings.HasSuffix(footerLine(m), "HEAD: gopls"), footerLine(m))
	m = restLeft(t, m, 4)
	m = answer(m, status(gopls(), hover.Ready))
	assert.True(t, strings.HasSuffix(footerLine(m), "HEAD: gopls"), "the footer stays with the side asked about: %s", footerLine(m))
}

func TestAFailedCheckoutShowsInTheFooter(t *testing.T) {
	var left lang.Instance
	m, f := leftModel(t, &left)
	f.hover.HoverReturns(hover.NotReady)
	m = restLeft(t, m, 9)

	m = answer(m, checkout(t, hover.ServerFailed))

	assert.True(t, strings.HasSuffix(footerLine(m), "HEAD: checkout failed"), footerLine(m))
	assert.False(t, m.s.Hover.Waiting, "nothing to wait for")
	assert.False(t, m.s.sideHoverable(0, filediff.OldSide))
}

func TestTheCaretGoesToTheOtherSide(t *testing.T) {
	var left lang.Instance
	m, f := leftModel(t, &left)
	m, _ = typed(m, "i")
	require.Equal(t, caret{On: true, Side: filediff.NewSide, Row: 0, Byte: 0}, m.s.Caret, "on y")

	m, _ = typed(m, "o")
	assert.Equal(t, caret{On: true, Side: filediff.OldSide, Row: 0, Byte: 0}, m.s.Caret, "on the tab, at the same column")
	m, _ = typed(m, "w", "K")

	require.Equal(t, 1, f.hover.HoverCallCount())
	assert.Equal(t, hover.Position{Req: 1, Commit: baseCommit(t), Path: "a.go", Version: 1, Line: 0, Byte: 1}, f.hover.HoverArgsForCall(0))
	assert.True(t, m.s.caretOn(0, 0, filediff.OldSide).On, "drawn on the left")
	assert.False(t, m.s.caretOn(0, 0, filediff.NewSide).On, "not on the right")
	m, _ = typed(m, "o")
	assert.Equal(t, filediff.NewSide, m.s.Caret.Side)
}

func TestTheCaretStaysWithoutTextOnTheOtherSide(t *testing.T) {
	d := diff.Compute(t.Context(), diff.Split(""), diff.Split("a\n"))
	s := shown(t, files("a.go"), 80, 10+chrome, worktreeDoc(filediff.Snapshot{Diff: d, LeftDoc: hover.Document{Path: "a.go", Commit: baseCommit(t)}}))
	s.startCaret()
	require.True(t, s.Caret.On)

	s.otherSide()

	assert.Equal(t, filediff.NewSide, s.Caret.Side)
}
