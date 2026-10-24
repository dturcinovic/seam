package tui

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/app/port/in/infakes"
	"seam/internal/domain/change"
	"seam/internal/domain/ui"
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

// signals are the signals of the ports' fakes, real ones of capacity 1.
type signals struct {
	files, reload, lsp chan struct{}
}

func signalling() (Ports, signals) {
	p := fakePorts()
	sig := signals{make(chan struct{}, 1), make(chan struct{}, 1), make(chan struct{}, 1)}
	p.Files.(*infakes.FakeFileDiffLoader).SignalReturns(sig.files)
	p.Reload.(*infakes.FakeChangesetReloader).SignalReturns(sig.reload)
	p.LSP.(*infakes.FakeLSPUpdates).SignalReturns(sig.lsp)
	return p, sig
}

func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func TestTheBridgeWakesTheProgramWithoutInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ports, sig := signalling()
		p := &program{busy: make(chan struct{})}
		close(p.busy)
		b := newBridge(nil)
		require.NoError(t, b.start(p.send, ports))

		for _, ch := range []chan struct{}{sig.files, sig.reload, sig.lsp} {
			notify(ch)
			synctest.Wait()
		}

		assert.Equal(t, []tea.Msg{fileDiffsReady{}, changesetReady{}, lspUpdated{}}, p.received())
		require.NoError(t, b.stop())
	})
}

func TestManyUpdatesFallTogetherAndNoneIsLost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ports, sig := signalling()
		p := &program{busy: make(chan struct{})}
		b := newBridge(nil)
		require.NoError(t, b.start(p.send, ports))

		// The loop is busy: the first signal waits in send, the rest fall
		// into the one still set.
		for range 10 {
			notify(sig.files)
			synctest.Wait()
		}
		close(p.busy)
		synctest.Wait()

		msgs := p.received()
		assert.Len(t, msgs, 2, "one in flight, one for everything after")
		m := newModel(change.Changeset{}, ports, ui.DefaultSettings())
		m.Update(msgs[0])
		assert.Equal(t, 1, ports.Files.(*infakes.FakeFileDiffLoader).DrainCallCount(), "one message drains every update")
		require.NoError(t, b.stop())
	})
}

// TestTheBridgeStopsLongAfterItStarted: its stop deadline counts from the
// stop, not from a start long past.
func TestTheBridgeStopsLongAfterItStarted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ports, _ := signalling()
		p := &program{busy: make(chan struct{})}
		b := newBridge(nil)
		require.NoError(t, b.start(p.send, ports))

		time.Sleep(time.Hour)

		require.NoError(t, b.stop())
	})
}
