package tui

import (
	"sync"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/in/infakes"
	"seam/internal/domain/change"
)

// program stands in for Program.Send: it records messages and can be held
// busy, as the event loop is while it renders.
type program struct {
	mu   sync.Mutex
	msgs []tea.Msg
	busy chan struct{}
}

func (p *program) send(msg tea.Msg) {
	<-p.busy
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = append(p.msgs, msg)
}

func (p *program) received() []tea.Msg {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.msgs
}

// signalling are port fakes with real signals of capacity 1.
func signalling() (*infakes.FakeFileDiffLoader, *infakes.FakeChangesetReloader, chan struct{}, chan struct{}) {
	fl, rl := &infakes.FakeFileDiffLoader{}, &infakes.FakeChangesetReloader{}
	files, reload := make(chan struct{}, 1), make(chan struct{}, 1)
	fl.SignalReturns(files)
	rl.SignalReturns(reload)
	return fl, rl, files, reload
}

func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func TestTheBridgeWakesTheProgramWithoutInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fl, rl, files, reload := signalling()
		p := &program{busy: make(chan struct{})}
		close(p.busy)
		b := newBridge(nil)
		require.NoError(t, b.start(p.send, fl, rl))

		notify(files)
		synctest.Wait()
		notify(reload)
		synctest.Wait()

		assert.Equal(t, []tea.Msg{fileDiffsReady{}, changesetReady{}}, p.received())
		require.NoError(t, b.stop(t.Context()))
	})
}

func TestManyUpdatesFallTogetherAndNoneIsLost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fl, rl, files, _ := signalling()
		p := &program{busy: make(chan struct{})}
		b := newBridge(nil)
		require.NoError(t, b.start(p.send, fl, rl))

		// The loop is busy: the first signal waits in send, the rest fall
		// into the one still set.
		for range 10 {
			notify(files)
			synctest.Wait()
		}
		close(p.busy)
		synctest.Wait()

		msgs := p.received()
		assert.Len(t, msgs, 2, "one in flight, one for everything after")
		m := newModel(change.Changeset{}, fl, rl)
		m.Update(msgs[0])
		assert.Equal(t, 1, fl.DrainCallCount(), "one message drains every update")
		require.NoError(t, b.stop(t.Context()))
	})
}
