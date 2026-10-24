package tui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/in/infakes"
	"seam/internal/domain/change"
)

func watchNews(m model, us ...change.WatchUpdate) model {
	m.p.Watch.(*infakes.FakeWatchUpdates).DrainReturns(us)
	m, _ = send(m, watchUpdated{})
	return m
}

func TestTheHeaderSaysWhileTheWatcherRuns(t *testing.T) {
	m, _, _ := startModel(t, "a.go")
	assert.NotContains(t, header(&m.s), "watch")

	m = watchNews(m, change.WatchUpdate{Kind: change.Watching})
	assert.Contains(t, header(&m.s), "  watch")

	m = watchNews(m, change.WatchUpdate{Kind: change.WatchFailed, Err: errors.New("watch: inotify")})
	text, _ := footer(&m.s)
	assert.Equal(t, "error: watch: inotify", text)
	m.s.Err = nil // as the next file loaded does
	assert.Contains(t, header(&m.s), "  watch failed", "the header keeps saying so")
}

func TestAChangeReloadsAsRDoes(t *testing.T) {
	m, fl, rl := startModel(t, "a.go", "b.go")

	m = watchNews(m, change.WatchUpdate{Kind: change.Changed})

	require.Equal(t, 1, rl.ReloadCallCount())
	rl.DrainReturns([]change.Update{{Gen: rl.ReloadArgsForCall(0), Changeset: change.Changeset{Files: files("a.go", "b.go")}}})
	send(m, changesetReady{})
	require.Equal(t, 2, fl.LoadCallCount())
	r := fl.LoadArgsForCall(1)
	assert.Equal(t, "a.go", r.File.Path())
	assert.True(t, r.Fresh, "the same file, read afresh")
}

func TestAChangeWaitsForTheRunningReload(t *testing.T) {
	m, _, rl := startModel(t, "a.go")
	m = watchNews(m, change.WatchUpdate{Kind: change.Changed})

	m = watchNews(m, change.WatchUpdate{Kind: change.Changed}, change.WatchUpdate{Kind: change.Changed})
	assert.Equal(t, 1, rl.ReloadCallCount(), "no cancelling the running reload")

	rl.DrainReturns([]change.Update{{Gen: rl.ReloadArgsForCall(0), Changeset: change.Changeset{Files: files("a.go")}}})
	m, _ = send(m, changesetReady{})
	require.Equal(t, 2, rl.ReloadCallCount(), "one reload for the changes meanwhile")

	rl.DrainReturns([]change.Update{{Gen: rl.ReloadArgsForCall(1), Changeset: change.Changeset{Files: files("a.go")}}})
	send(m, changesetReady{})
	assert.Equal(t, 2, rl.ReloadCallCount(), "nothing waits any more")
}
