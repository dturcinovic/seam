package lsp

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/testsupport"
)

func stopPool(t *testing.T, p *Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, p.Stop(ctx))
}

func lastStatus(all []hover.Update) hover.ServerStatus {
	var s hover.ServerStatus
	for _, u := range all {
		if u.Kind == hover.StatusChanged {
			s = u.Status
		}
	}
	return s
}

func TestASilentServerFailsAfter20Seconds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: func(p *peer) {
			for _, ok := p.next(); ok; _, ok = p.next() {
			}
		}})
		_, ok := pool.Sync(doc("a.go", 1, "package a\n"))
		require.True(t, ok)

		time.Sleep(initTimeout - time.Second)
		synctest.Wait()
		assert.Equal(t, hover.Starting, lastStatus(pool.Drain()))

		time.Sleep(time.Second)
		synctest.Wait()
		select {
		case <-pool.Signal():
		default:
			t.Fatal("no signal")
		}
		assert.Equal(t, hover.ServerFailed, lastStatus(pool.Drain()))
		stopPool(t, pool)
	})
}

// ready syncs a document and waits until its server is Ready.
func ready(t *testing.T, pool *Pool, d hover.Document) {
	t.Helper()
	_, ok := pool.Sync(d)
	require.True(t, ok)
	testsupport.Collect(t, pool, hasStatus(hover.Ready))
	synctest.Wait()
}

func TestAnUnansweredHoverTimesOutAfter5Seconds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: served})
		ready(t, pool, doc("a.go", 1, "package a\n"))

		require.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 7, Path: "a.go", Version: 1}))
		time.Sleep(hoverTimeout - time.Millisecond)
		synctest.Wait()
		assert.Empty(t, results(pool.Drain()))

		time.Sleep(time.Millisecond)
		synctest.Wait()
		<-pool.Signal()
		assert.Equal(t, []hover.Update{{Kind: hover.HoverDone, Req: 7, Version: 1, Outcome: hover.TimedOut}}, results(pool.Drain()))
		stopPool(t, pool)
	})
}

// answering answers hovers after delay with text.
func answering(delay time.Duration, text string) func(p *peer) {
	return func(p *peer) {
		p.serveUntil(func(m message) bool {
			if m.Method == "textDocument/hover" {
				time.Sleep(delay)
				p.reply(m.ID, fmt.Sprintf(`{"contents":{"kind":"markdown","value":%q}}`, text))
			}
			return true
		})
	}
}

func TestALateAnswerSetsTheSignal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: answering(800*time.Millisecond, "func F()")})
		ready(t, pool, doc("a.go", 1, "package a\n"))

		require.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 1, Path: "a.go", Version: 1}))
		<-pool.Signal()

		assert.Equal(t, []hover.Update{{Kind: hover.HoverDone, Req: 1, Version: 1, Outcome: hover.Text, Content: paragraph("func F()")}}, results(pool.Drain()))
		stopPool(t, pool)
	})
}

func TestTheDocumentGoesOutBeforeTheHover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var seen []string
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: func(p *peer) {
			p.serveUntil(func(m message) bool {
				seen = append(seen, describe(m))
				if m.Method == "textDocument/hover" {
					time.Sleep(time.Second)
					p.reply(m.ID, `{"contents":"x"}`)
				}
				return true
			})
		}})
		ready(t, pool, doc("a.go", 1, "v1\n"))
		_, _ = pool.Sync(doc("a.go", 2, "v2\n"))
		require.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 1, Path: "a.go", Version: 2}))
		// v3 arrives while the server answers the hover against v2.
		synctest.Wait()
		_, _ = pool.Sync(doc("a.go", 3, "v3\n"))

		got := results(testsupport.Collect(t, pool, hasResult))
		synctest.Wait()

		assert.Equal(t, []string{"initialized", "didOpen a.go v1 \"v1\\n\"", "didChange a.go v2 \"v2\\n\"", "hover a.go 0:0", "didChange a.go v3 \"v3\\n\""}, seen)
		assert.Equal(t, hover.DocVersion(2), got[0].Version, "the answer names the version it was asked against")
		assert.Equal(t, hover.Stale, pool.Request(hover.Position{Req: 2, Path: "a.go", Version: 2}))
		stopPool(t, pool)
	})
}

// describe names a message as the tests compare them.
func describe(m message) string {
	var p struct {
		TextDocument struct {
			URI     string `json:"uri"`
			Version int    `json:"version"`
			Text    string `json:"text"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}
	_ = json.Unmarshal(m.Params, &p) // a message without these fields is named by its method
	name := p.TextDocument.URI[strings.LastIndex(p.TextDocument.URI, "/")+1:]
	switch m.Method {
	case "textDocument/didOpen":
		return fmt.Sprintf("didOpen %s v%d %q", name, p.TextDocument.Version, p.TextDocument.Text)
	case "textDocument/didChange":
		if len(p.ContentChanges) != 1 {
			return fmt.Sprintf("didChange %s v%d with %d changes", name, p.TextDocument.Version, len(p.ContentChanges))
		}
		return fmt.Sprintf("didChange %s v%d %q", name, p.TextDocument.Version, p.ContentChanges[0].Text)
	case "textDocument/didClose":
		return "didClose " + name
	case "textDocument/hover":
		return fmt.Sprintf("hover %s %d:%d", name, p.Position.Line, p.Position.Character)
	case "$/cancelRequest":
		return "cancel " + string(m.Params)
	}
	return m.Method
}

// TestAHoverWaitsForItsTextWhenBothWait: the writer is held up while a new
// version and a hover on it queue; once free, both are ready at once, and
// the text still goes first.
func TestAHoverWaitsForItsTextWhenBothWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resume := make(chan struct{})
		var seen []string
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: func(p *peer) {
			p.serveUntil(func(m message) bool {
				seen = append(seen, describe(m))
				if m.Method == "textDocument/didOpen" {
					<-resume
				}
				return true
			})
		}})
		ready(t, pool, doc("a.go", 1, "v1\n"))
		require.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 1, Path: "a.go", Version: 1}))
		synctest.Wait()
		_, _ = pool.Sync(doc("a.go", 2, "v2\n"))
		require.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 2, Path: "a.go", Version: 2}))
		synctest.Wait()

		close(resume)
		synctest.Wait()

		assert.Equal(t, []string{"initialized", "didOpen a.go v1 \"v1\\n\"", "hover a.go 0:0", "didChange a.go v2 \"v2\\n\"", "hover a.go 0:0"}, seen)
		stopPool(t, pool)
	})
}

func TestOnlyTheNewestVersionGoesOutWhileThePipeIsFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resume := make(chan struct{})
		var seen []string
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: func(p *peer) {
			p.serveUntil(func(m message) bool {
				seen = append(seen, describe(m))
				if m.Method == "initialized" {
					<-resume
				}
				return true
			})
		}})
		ready(t, pool, doc("a.go", 1, "v1\n"))
		// The writer is held up on v1 by the pipe; v2 and v3 queue behind it.
		_, _ = pool.Sync(doc("a.go", 2, "v2\n"))
		_, _ = pool.Sync(doc("a.go", 3, "v3\n"))
		synctest.Wait()

		close(resume)
		synctest.Wait()

		assert.Equal(t, []string{"initialized", "didOpen a.go v1 \"v1\\n\"", "didChange a.go v3 \"v3\\n\""}, seen)
		stopPool(t, pool)
	})
}

func TestCancelTellsTheServerAndDropsTheAnswer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var seen []string
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: func(p *peer) {
			p.serveUntil(func(m message) bool {
				seen = append(seen, describe(m))
				if m.Method == "textDocument/hover" {
					time.Sleep(time.Second)
					p.reply(m.ID, `{"contents":"late"}`)
				}
				return true
			})
		}})
		ready(t, pool, doc("a.go", 1, "package a\n"))

		require.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 1, Path: "a.go", Version: 1}))
		synctest.Wait()
		pool.Cancel(1)
		time.Sleep(2 * time.Second)
		synctest.Wait()

		assert.Contains(t, seen, `cancel {"id":2}`)
		assert.Empty(t, results(pool.Drain()), "a cancelled hover reports nothing")
		c := pool.instances[lang.Instance{Server: "fake", Root: "."}].client
		assert.Empty(t, c.pending)
		stopPool(t, pool)
	})
}

func TestAtMostEightHoversAreOpen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: served})
		ready(t, pool, doc("a.go", 1, "package a\n"))

		for req := range hover.ReqID(10) {
			require.Equal(t, hover.Sent, pool.Request(hover.Position{Req: req + 1, Path: "a.go", Version: 1}))
		}
		synctest.Wait()

		c := pool.instances[lang.Instance{Server: "fake", Root: "."}].client
		var open []hover.ReqID
		for _, id := range c.hovers {
			open = append(open, c.pending[id].req)
		}
		assert.Equal(t, []hover.ReqID{3, 4, 5, 6, 7, 8, 9, 10}, open, "the oldest went")
		assert.Equal(t, []hover.Update{
			{Kind: hover.HoverDone, Req: 1, Version: 1, Outcome: hover.Error},
			{Kind: hover.HoverDone, Req: 2, Version: 1, Outcome: hover.Error},
		}, results(pool.Drain()), "and each of them was answered")
		stopPool(t, pool)
	})
}

func TestPositionsInTheServersEncoding(t *testing.T) {
	text := "package a\n\tx := \"é👨‍👩\" + y\n"
	byteOffset := strings.Index(text[10:], "y")
	tests := map[string]struct{ caps, want string }{
		"utf-16 by default": {`{"capabilities":{}}`, "hover a.go 1:17"},
		"utf-8 if chosen":   {`{"capabilities":{"positionEncoding":"utf-8"}}`, fmt.Sprintf("hover a.go 1:%d", byteOffset)},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var seen []string
				pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: func(p *peer) {
					for m, ok := p.next(); ok; m, ok = p.next() {
						switch m.Method {
						case "initialize":
							p.reply(m.ID, tt.caps)
						case "textDocument/hover":
							seen = append(seen, describe(m))
						case "exit":
							return
						}
					}
				}})
				ready(t, pool, doc("a.go", 1, text))
				require.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 1, Path: "a.go", Version: 1, Line: 1, Byte: byteOffset}))
				synctest.Wait()
				assert.Equal(t, []string{tt.want}, seen)
				stopPool(t, pool)
			})
		})
	}
}
