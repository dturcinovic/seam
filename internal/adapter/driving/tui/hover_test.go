package tui

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"

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

// codeLine is the right side's only line in hoverModel. After the tab,
// fmt takes columns 4 to 6 (bytes 1 to 3), the dot 7 and Println 8 to 14
// (bytes 5 to 11).
const codeLine = "\tfmt.Println(x)"

type hoverFakes struct {
	hover *infakes.FakeHoverer
	lsp   *infakes.FakeLSPUpdates
}

// hoverModel shows codeLine as the first line of the worktree's text, 40
// more after it, served by gopls, on a screen of 80 by 24; in the hunk
// view the fifth line is a fold.
func hoverModel(t *testing.T) (model, hoverFakes) {
	t.Helper()
	m, fl, _ := startModel(t, "a.go")
	m.p.Docs.(*infakes.FakeDocumentTracker).TrackReturns(gopls(), true)
	f := hoverFakes{m.p.Hover.(*infakes.FakeHoverer), m.p.LSP.(*infakes.FakeLSPUpdates)}
	f.hover.HoverReturns(hover.Sent)
	d := diff.Compute(t.Context(), diff.Split("x\n"+numbered(40)), diff.Split(codeLine+"\n"+numbered(40)))
	fl.DrainReturns([]filediff.Update{{Gen: fl.LoadArgsForCall(0).Gen, Snapshot: filediff.Snapshot{
		File: fl.LoadArgsForCall(0).File, Diff: d,
		RightDoc: hover.Document{Path: "a.go", Lang: lang.Go, Version: 1, Text: codeLine + "\n"},
	}}})
	m, _ = send(m, fileDiffsReady{})
	return m, f
}

// at is the screen cell of column col of codeLine's text.
func at(m model, col int) (x, y int) {
	from, _ := m.s.Layout.text(filediff.NewSide)
	return from + col, bodyTop
}

func move(m model, col int) (model, tea.Cmd) {
	x, y := at(m, col)
	return send(m, tea.MouseMotionMsg{X: x, Y: y})
}

// rest moves the mouse to col and lets it rest there: the tick comes, and
// the model asks. The tick is sent rather than waited for, which
// TestTheMouseAsksOnceItRests does.
func rest(t *testing.T, m model, col int) model {
	t.Helper()
	m, cmd := move(m, col)
	require.NotNil(t, cmd)
	m, _ = send(m, hoverDue{Seq: m.s.Hover.Seq})
	return m
}

func answer(m model, us ...hover.Update) model {
	m.p.LSP.(*infakes.FakeLSPUpdates).DrainReturns(us)
	m, _ = send(m, lspUpdated{})
	return m
}

func text(req hover.ReqID, s string) hover.Update {
	return hover.Update{Kind: hover.HoverDone, Req: req, Version: 1, Outcome: hover.Text, Content: code(s)}
}

func TestTheMouseAsksOnceItRests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, f := hoverModel(t)
		m, cmd := move(m, 5)
		require.NotNil(t, cmd)
		begin := time.Now()

		m, _ = send(m, cmd())

		assert.Equal(t, 300*time.Millisecond, time.Since(begin))
		require.Equal(t, 1, f.hover.HoverCallCount())
		assert.Equal(t, hover.Position{Req: 1, Path: "a.go", Version: 1, Line: 0, Byte: 2}, f.hover.HoverArgsForCall(0))
		assert.Equal(t, hover.ReqID(1), m.s.Hover.Inflight)
	})
}

func TestTheMouseRestsForTheConfiguredDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, f := hoverModel(t)
		m.hoverDelay = time.Second
		_, cmd := move(m, 5)
		begin := time.Now()

		cmd()

		assert.Equal(t, time.Second, time.Since(begin))
		assert.Zero(t, f.hover.HoverCallCount(), "asked only once the message arrives")
	})
}

func TestMovingWithinAnIdentifierAsksNothingNew(t *testing.T) {
	m, f := hoverModel(t)
	m = rest(t, m, 4)

	m, cmd := move(m, 6)

	assert.Nil(t, cmd, "still fmt")
	assert.Zero(t, f.hover.CancelHoverCallCount())
	_, cmd = move(m, 7)
	assert.NotNil(t, cmd, "the dot is another target")
}

func TestATickForATargetLeftAsksNothing(t *testing.T) {
	m, f := hoverModel(t)
	m, _ = move(m, 4)
	first := m.s.Hover.Seq
	m, _ = move(m, 9)

	m, _ = send(m, hoverDue{Seq: first})
	assert.Zero(t, f.hover.HoverCallCount())
	send(m, hoverDue{Seq: m.s.Hover.Seq})

	require.Equal(t, 1, f.hover.HoverCallCount())
	assert.Equal(t, 6, f.hover.HoverArgsForCall(0).Byte)
}

func TestMovingOnCancelsTheRequest(t *testing.T) {
	m, f := hoverModel(t)
	m = rest(t, m, 4)

	m, _ = move(m, 9)

	require.Equal(t, 1, f.hover.CancelHoverCallCount())
	assert.Equal(t, hover.ReqID(1), f.hover.CancelHoverArgsForCall(0))
	_, _ = move(m, 16)
	assert.Equal(t, 1, f.hover.CancelHoverCallCount(), "nothing is in flight any more")
}

func TestOnlyTheAnswerWaitedForOpensThePopup(t *testing.T) {
	tests := []struct {
		name string
		u    hover.Update
		open bool
	}{
		{"its answer", text(1, "func fmt.Println"), true},
		{"another request's", text(2, "func fmt.Println"), false},
		{"for another version", hover.Update{Kind: hover.HoverDone, Req: 1, Version: 2, Outcome: hover.Text, Content: code("x")}, false},
		{"empty", hover.Update{Kind: hover.HoverDone, Req: 1, Version: 1, Outcome: hover.Empty}, false},
		{"timed out", hover.Update{Kind: hover.HoverDone, Req: 1, Version: 1, Outcome: hover.TimedOut}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := hoverModel(t)
			m = rest(t, m, 9)

			m = answer(m, tt.u)

			assert.Equal(t, tt.open, m.s.Hover.Popup.Open)
		})
	}
}

func TestAServerNotReadyIsAskedAgainOnceReady(t *testing.T) {
	m, f := hoverModel(t)
	f.hover.HoverReturnsOnCall(0, hover.NotReady)
	m = rest(t, m, 9)
	require.Zero(t, m.s.Hover.Inflight)

	m = answer(m, status(lang.Instance{Server: "gopls", Root: "other"}, hover.Ready))
	require.Equal(t, 1, f.hover.HoverCallCount(), "not the server of the file shown")
	m = answer(m, status(gopls(), hover.Ready))

	require.Equal(t, 2, f.hover.HoverCallCount())
	assert.Equal(t, hover.ReqID(2), f.hover.HoverArgsForCall(1).Req)
	m = answer(m, text(2, "func fmt.Println"))
	assert.True(t, m.s.Hover.Popup.Open)
}

func TestAnEmptyAnswerDuringTheServersWorkIsAskedAgainAfterIt(t *testing.T) {
	m, f := hoverModel(t)
	m = answer(m, progress(gopls(), "indexing", 40))
	m = rest(t, m, 9)

	m = answer(m, hover.Update{Kind: hover.HoverDone, Req: 1, Version: 1, Outcome: hover.Empty})
	m = answer(m, hover.Update{Kind: hover.ProgressChanged, Instance: lang.Instance{Server: "gopls", Root: "other"}})
	require.Equal(t, 1, f.hover.HoverCallCount(), "not the server of the file shown")
	m = answer(m, hover.Update{Kind: hover.ProgressChanged, Instance: gopls()})

	require.Equal(t, 2, f.hover.HoverCallCount())
	m = answer(m, text(2, "func fmt.Println"))
	assert.True(t, m.s.Hover.Popup.Open)
}

// TestAnEmptyAnswerIsAskedAgainAfterEachWork: intelephense answers a
// hover right after it starts, before it reports its indexing, and
// rust-analyzer runs one piece of work after another.
func TestAnEmptyAnswerIsAskedAgainAfterEachWork(t *testing.T) {
	m, f := hoverModel(t)
	m = rest(t, m, 9)
	m = answer(m, hover.Update{Kind: hover.HoverDone, Req: 1, Version: 1, Outcome: hover.Empty})

	m = answer(m, progress(gopls(), "fetching", -1), hover.Update{Kind: hover.ProgressChanged, Instance: gopls()})
	require.Equal(t, 2, f.hover.HoverCallCount())
	m = answer(m, hover.Update{Kind: hover.HoverDone, Req: 2, Version: 1, Outcome: hover.Empty})
	m = answer(m, progress(gopls(), "indexing", 0), hover.Update{Kind: hover.ProgressChanged, Instance: gopls()})
	require.Equal(t, 3, f.hover.HoverCallCount())

	m = answer(m, text(3, "func fmt.Println"))
	answer(m, progress(gopls(), "cargo check", -1), hover.Update{Kind: hover.ProgressChanged, Instance: gopls()})
	assert.Equal(t, 3, f.hover.HoverCallCount(), "an answer with text waits for nothing")
}

func TestLeavingTheTargetStopsWaitingForTheWork(t *testing.T) {
	m, f := hoverModel(t)
	m = answer(m, progress(gopls(), "indexing", 40))
	m = rest(t, m, 9)
	m = answer(m, hover.Update{Kind: hover.HoverDone, Req: 1, Version: 1, Outcome: hover.Empty})

	m, _ = send(m, tea.MouseMotionMsg{X: 0, Y: 0})
	answer(m, hover.Update{Kind: hover.ProgressChanged, Instance: gopls()})

	assert.Equal(t, 1, f.hover.HoverCallCount())
}

func TestRefusedRequestsWaitForNothing(t *testing.T) {
	for _, sub := range []hover.Submit{hover.Unsupported, hover.Stale, hover.Busy, hover.Failed} {
		m, f := hoverModel(t)
		f.hover.HoverReturns(sub)
		m = rest(t, m, 9)

		assert.Zero(t, m.s.Hover.Inflight, sub)
		assert.False(t, m.s.Hover.Waiting, sub)
	}
}

// TestAnAnswerBeforeHoverReturnsIsTheSame: the answer may be waiting
// before Hover has returned, or come after; the popup is the same.
func TestAnAnswerBeforeHoverReturnsIsTheSame(t *testing.T) {
	early, f := hoverModel(t)
	f.hover.HoverCalls(func(p hover.Position) hover.Submit {
		f.lsp.DrainReturns([]hover.Update{text(p.Req, "func fmt.Println")})
		return hover.Sent
	})
	early = rest(t, early, 9)
	early, _ = send(early, lspUpdated{})

	late, _ := hoverModel(t)
	late = rest(t, late, 9)
	late = answer(late, text(1, "func fmt.Println"))

	require.True(t, early.s.Hover.Popup.Open)
	assert.Equal(t, late.s.Hover.Popup, early.s.Hover.Popup)
}

func TestThePopupStaysWhileTheMouseIsOverIt(t *testing.T) {
	m, f := hoverModel(t)
	m = answer(rest(t, m, 9), text(1, "func fmt.Println(a ...any)"))
	p := m.s.Hover.Popup
	require.True(t, p.Open)

	m, cmd := send(m, tea.MouseMotionMsg{X: p.X + 1, Y: p.Y + 1})

	assert.Nil(t, cmd)
	assert.True(t, m.s.Hover.Popup.Open)
	assert.Zero(t, f.hover.CancelHoverCallCount())
}

func TestEscClosesThePopupBeforeItQuits(t *testing.T) {
	m, _ := hoverModel(t)
	m = answer(rest(t, m, 9), text(1, "func fmt.Println"))

	m, cmd := key(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Nil(t, cmd)
	assert.False(t, m.s.Hover.Popup.Open)

	_, cmd = key(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	assert.Equal(t, tea.Quit(), cmd())
}

func TestMovingTheTextClosesThePopup(t *testing.T) {
	full := tea.KeyPressMsg{Code: 'f', Text: "f"}
	tests := []struct {
		name  string
		first []tea.Msg
		msg   tea.Msg
	}{
		{"a key scrolling", []tea.Msg{full}, tea.KeyPressMsg{Code: 'j', Text: "j"}},
		{"the wheel over the panes", []tea.Msg{full}, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 1, Y: bodyTop}},
		{"another view", nil, full},
		{"a fold unfolded", nil, tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: bodyTop + 4}},
		{"a resize", nil, tea.WindowSizeMsg{Width: 100, Height: 30}},
	}
	for _, tt := range tests {
		m, f := hoverModel(t)
		for _, msg := range tt.first {
			m, _ = send(m, msg)
		}
		m = answer(rest(t, m, 9), text(1, "func fmt.Println"))
		require.True(t, m.s.Hover.Popup.Open, tt.name)

		m, _ = send(m, tt.msg)

		assert.False(t, m.s.Hover.Popup.Open, tt.name)
		assert.Zero(t, f.hover.CancelHoverCallCount(), "%s: the answer came, nothing to cancel", tt.name)
	}
}

func TestWhatMovesNothingKeepsThePopup(t *testing.T) {
	m, _ := hoverModel(t)
	m = answer(rest(t, m, 9), text(1, "func fmt.Println"))

	m, _ = key(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 1, Y: bodyTop})

	assert.True(t, m.s.Hover.Popup.Open, "an unknown key, and the wheel at the top")
}

func TestTheWheelOverThePopupScrollsIt(t *testing.T) {
	m, _ := hoverModel(t)
	m = answer(rest(t, m, 9), text(1, strings.Repeat("line\n", 30)))
	p := m.s.Hover.Popup
	require.Less(t, p.shown(), len(p.Lines))

	m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: p.X + 1, Y: p.Y + 1})
	assert.Equal(t, wheelLines, m.s.Hover.Popup.Scroll)
	for range 20 {
		m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: p.X + 1, Y: p.Y + 1})
	}

	assert.Equal(t, len(p.Lines)-p.shown(), m.s.Hover.Popup.Scroll, "no further than its last line")
}

func TestTheMouseStaysCaughtWhileAPopupIsOpen(t *testing.T) {
	m, _ := hoverModel(t)
	m = answer(rest(t, m, 9), text(1, "func fmt.Println"))

	m = answer(m, status(gopls(), hover.ServerFailed))

	assert.Equal(t, tea.MouseModeAllMotion, m.View().MouseMode, "moving away must close it")
	m, _ = move(m, 30)
	assert.False(t, m.s.Hover.Popup.Open)
	assert.Equal(t, tea.MouseModeCellMotion, m.View().MouseMode)
}

func TestAClickOnThePopupStaysThere(t *testing.T) {
	m, _ := hoverModel(t)
	m = answer(rest(t, m, 9), text(1, "a\nb\nc\nd"))
	p := m.s.Hover.Popup
	require.True(t, p.contains(p.X+1, bodyTop+4), "over the fold")

	m, _ = send(m, tea.MouseClickMsg{Button: tea.MouseLeft, X: p.X + 1, Y: bodyTop + 4})

	assert.True(t, m.s.Hover.Popup.Open)
	assert.True(t, m.s.Lines[4].Fold, "not unfolded")
}

func TestAPopupWithNothingToScrollLetsTheWheelThrough(t *testing.T) {
	m, _ := hoverModel(t)
	m, _ = send(m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = answer(rest(t, m, 9), text(1, "var x int"))
	p := m.s.Hover.Popup

	m, _ = send(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: p.X + 1, Y: p.Y + 1})

	assert.Positive(t, m.s.Pos.Scroll, "the panes scrolled")
	assert.False(t, m.s.Hover.Popup.Open)
}

func TestABusyServerIsAskedAgainOnTheNextRest(t *testing.T) {
	for _, sub := range []hover.Submit{hover.Busy, hover.Stale} {
		m, f := hoverModel(t)
		f.hover.HoverReturnsOnCall(0, sub)
		m = rest(t, m, 9)

		rest(t, m, 10)

		assert.Equal(t, 2, f.hover.HoverCallCount(), "%v: within the same identifier", sub)
	}
}

func TestThePopupOpensWhereTheMouseRests(t *testing.T) {
	m, _ := hoverModel(t)
	m, _ = move(m, 14)
	m, _ = move(m, 8)
	m, _ = send(m, hoverDue{Seq: m.s.Hover.Seq})

	m = answer(m, text(1, "func fmt.Println"))

	x, _ := at(m, 8)
	assert.Equal(t, x, m.s.Hover.Popup.X)
}

func TestANewLoadOfTheFileClosesThePopup(t *testing.T) {
	m, _ := hoverModel(t)
	m = answer(rest(t, m, 9), text(1, "func fmt.Println"))

	m, _ = key(m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	rl := m.p.Reload.(*infakes.FakeChangesetReloader)
	rl.DrainReturns([]change.Update{{Gen: rl.ReloadArgsForCall(0), Changeset: change.Changeset{Files: files("a.go")}}})
	m, _ = send(m, changesetReady{})

	assert.False(t, m.s.Hover.Popup.Open, "the other side may differ though the text is the same")
}
