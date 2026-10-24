package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/in/infakes"
	"seam/internal/domain/change"
	"seam/internal/domain/filediff"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
)

func gopls() lang.Instance { return lang.Instance{Server: "gopls"} }

// showDoc has m show the answer to its last request, with a right side
// for the server, and returns the tracker's fake.
func showDoc(t *testing.T, m model, fl *infakes.FakeFileDiffLoader, path string) (model, *infakes.FakeDocumentTracker) {
	t.Helper()
	u := snapshotFor(t, fl.LoadArgsForCall(fl.LoadCallCount()-1))
	u.Snapshot.RightDoc = hover.Document{Path: path, Lang: lang.Go, Version: 1, Text: "b\n"}
	fl.DrainReturns([]filediff.Update{u})
	m, _ = send(m, fileDiffsReady{})
	return m, m.p.Docs.(*infakes.FakeDocumentTracker)
}

func lspSays(m model, us ...hover.Update) model {
	m.p.LSP.(*infakes.FakeLSPUpdates).DrainReturns(us)
	m, _ = send(m, lspUpdated{})
	return m
}

func status(i lang.Instance, s hover.ServerStatus) hover.Update {
	return hover.Update{Kind: hover.StatusChanged, Instance: i, Status: s}
}

func footerLine(m model) string {
	lines := strings.Split(render(&m.s, m.theme), "\n")
	return strings.TrimRight(ansi.Strip(lines[len(lines)-1]), " ")
}

func TestTheFileShownIsTracked(t *testing.T) {
	m, fl, _ := startModel(t, "a.go", "b.go")
	m, _ = key(m, tea.KeyPressMsg{Code: tea.KeyTab})
	// The answer for a.go comes late: it is not shown, so not tracked.
	fl.DrainReturns([]filediff.Update{snapshotFor(t, fl.LoadArgsForCall(0))})
	m, _ = send(m, fileDiffsReady{})
	docs := m.p.Docs.(*infakes.FakeDocumentTracker)
	require.Zero(t, docs.TrackCallCount())

	_, docs = showDoc(t, m, fl, "b.go")

	require.Equal(t, 1, docs.TrackCallCount())
	assert.Equal(t, "b.go", docs.TrackArgsForCall(0).Path)
}

func TestTheFooterTellsOfTheServerOfTheFileShown(t *testing.T) {
	tests := []struct {
		status hover.ServerStatus
		want   string
	}{
		{hover.Missing, "no LSP for .go"},
		{hover.Starting, "gopls starting…"},
		{hover.Ready, "gopls"},
		{hover.ServerFailed, "gopls failed"},
		{hover.Stopped, "gopls stopped"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			m, fl, _ := startModel(t, "a.go")
			m, docs := showDoc(t, m, fl, "a.go")
			docs.TrackReturns(gopls(), true)
			m, _ = showDoc(t, m, fl, "a.go")

			m = lspSays(m, status(gopls(), tt.status))

			assert.True(t, strings.HasSuffix(footerLine(m), "… "+tt.want), footerLine(m))
		})
	}
}

func TestTheFooterTellsOnlyOfTheServerServingTheFile(t *testing.T) {
	m, fl, _ := startModel(t, "a.go")
	m, docs := showDoc(t, m, fl, "a.go")
	keysOnly := footerLine(m)

	m = lspSays(m, status(gopls(), hover.Ready))
	assert.Equal(t, keysOnly, footerLine(m), "a.go has no server")

	docs.TrackReturns(lang.Instance{Server: "gopls", Root: "sub"}, true)
	m, _ = showDoc(t, m, fl, "a.go")
	assert.Equal(t, keysOnly, footerLine(m), "no status yet for this instance")

	docs.TrackReturns(gopls(), true)
	m, _ = showDoc(t, m, fl, "a.go")
	assert.True(t, strings.HasSuffix(footerLine(m), "… gopls"), "the status of an instance is kept")
}

func TestKindOfAFileWithoutExtensionIsItsLanguage(t *testing.T) {
	assert.Equal(t, "go", kindOf(hover.Document{Path: "cmd/tool", Lang: lang.Go}))
}

func TestTheFooterForgetsTheServerWhenTheFileGoes(t *testing.T) {
	m, fl, _ := startModel(t, "a.go", "b.go")
	m, docs := showDoc(t, m, fl, "a.go")
	docs.TrackReturns(gopls(), true)
	m, _ = showDoc(t, m, fl, "a.go")
	m = lspSays(m, status(gopls(), hover.Missing))
	require.True(t, strings.HasSuffix(footerLine(m), "… no LSP for .go"))

	m, _ = key(m, tea.KeyPressMsg{Code: tea.KeyTab})

	assert.NotContains(t, footerLine(m), "LSP", "b.go is still loading")
}

func TestAFileShownWithoutTextIsUntracked(t *testing.T) {
	m, fl, _ := startModel(t, "a.go")
	fl.DrainReturns([]filediff.Update{snapshotFor(t, fl.LoadArgsForCall(0))})

	m, _ = send(m, fileDiffsReady{})

	docs := m.p.Docs.(*infakes.FakeDocumentTracker)
	assert.Zero(t, docs.TrackCallCount())
	require.Equal(t, 1, docs.UntrackCallCount())
	assert.Equal(t, "a.go", docs.UntrackArgsForCall(0))
}

func TestAFileGoneFromTheChangesetIsUntracked(t *testing.T) {
	m, _, rl := startModel(t, "a.go", "b.go")
	m, _ = key(m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	rl.DrainReturns([]change.Update{{Gen: rl.ReloadArgsForCall(0), Changeset: change.Changeset{Files: files("b.go")}}})

	m, _ = send(m, changesetReady{})

	docs := m.p.Docs.(*infakes.FakeDocumentTracker)
	require.Equal(t, 1, docs.UntrackCallCount())
	assert.Equal(t, "a.go", docs.UntrackArgsForCall(0))
}
