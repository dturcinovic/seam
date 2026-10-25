package lsp

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/testsupport"
)

func TestTheServersRequestsGetThePlansAnswers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		answers := map[string]string{}
		asks := map[int]string{
			1: `{"id":1,"method":"workspace/configuration","params":{"items":[{"section":"python.analysis"},{"section":"nothing"},{}]}}`,
			2: `{"id":2,"method":"client/registerCapability","params":{"registrations":[]}}`,
			3: `{"id":"three","method":"no/such/method","params":{}}`,
			4: `{"id":4,"method":"workspace/applyEdit","params":{"edit":{}}}`,
			5: `{"id":5,"method":"window/workDoneProgress/create","params":{"token":"t"}}`,
			6: `{"id":6,"method":"workspace/workspaceFolders"}`,
		}
		pool := testPool(t, goTable(t, lang.Server{Settings: map[string]any{"python": map[string]any{"analysis": map[string]any{"strict": true}}}}),
			&pipes{serve: func(p *peer) {
				p.serveUntil(func(m message) bool {
					if m.Method == "initialized" {
						for i := 1; i <= len(asks); i++ {
							var a message
							require.NoError(t, json.Unmarshal([]byte(asks[i]), &a))
							p.send(a)
						}
						return true
					}
					if m.Method == "" {
						answers[string(m.ID)] = string(m.Result) + errorOf(m)
					}
					return true
				})
			}})
		ready(t, pool, doc("a.go", 1, "package a\n"))
		synctest.Wait()

		assert.Equal(t, `[{"strict":true},null,{"python":{"analysis":{"strict":true}}}]`, answers["1"], "one entry per item")
		assert.Equal(t, "error -32601", answers["2"])
		assert.Equal(t, "error -32601", answers[`"three"`])
		assert.Equal(t, `{"applied":false,"failureReason":"seam is read-only"}`, answers["4"])
		assert.Equal(t, "null", answers["5"])
		assert.Equal(t, `[{"uri":"file://`+filepath.ToSlash(worktree(pool))+`","name":"`+filepath.Base(worktree(pool))+`"}]`, answers["6"])
		stopPool(t, pool)
	})
}

func errorOf(m message) string {
	if m.Error == nil {
		return ""
	}
	return fmt.Sprintf("error %d", m.Error.Code)
}

func TestABrokenFrameFailsTheServer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: func(p *peer) {
			p.serveUntil(func(m message) bool {
				if m.Method == "textDocument/hover" {
					_, _ = fmt.Fprint(p.out, "Content-Type: x\r\n\r\n{}")
				}
				return true
			})
		}})
		ready(t, pool, doc("a.go", 1, "package a\n"))
		require.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 1, Path: "a.go", Version: 1}))

		all := testsupport.Collect(t, pool, hasStatus(hover.ServerFailed))
		synctest.Wait()
		all = append(all, pool.Drain()...)

		assert.Contains(t, results(all), hover.Update{Kind: hover.HoverDone, Req: 1, Version: 1, Outcome: hover.Error})
		assert.Equal(t, hover.Failed, pool.Request(hover.Position{Req: 2, Path: "a.go", Version: 1}))
		stopPool(t, pool)
	})
}

// stalled serves until initialized, then reads nothing until resume
// closes, so the pipe to it is full.
func stalled(resume chan struct{}, seen *[]string) func(p *peer) {
	return func(p *peer) {
		p.serveUntil(func(m message) bool {
			*seen = append(*seen, describe(m))
			if m.Method == "initialized" {
				select {
				case <-resume:
				case <-p.done:
				}
			}
			return true
		})
	}
}

func TestUnderOverloadNoCallBlocks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		resume := make(chan struct{})
		var seen []string
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: stalled(resume, &seen)})
		ready(t, pool, doc("a.go", 1, "a\n"))

		busy := false
		for req := hover.ReqID(1); req < 60 && !busy; req++ {
			busy = pool.Request(hover.Position{Req: req, Path: "a.go", Version: 1}) == hover.Busy
		}
		assert.True(t, busy, "a full request queue says Busy")
		// A file change is taken all the same.
		pool.Close(hover.DocKey{Path: "a.go"})
		_, ok := pool.Sync(doc("b.go", 1, "b\n"))
		require.True(t, ok)
		synctest.Wait()

		close(resume)
		synctest.Wait()
		require.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 100, Path: "b.go", Version: 1}))
		synctest.Wait()

		docs := filter(seen, "did")
		assert.Equal(t, []string{"didOpen a.go v1 \"a\\n\"", "didClose a.go", "didOpen b.go v1 \"b\\n\""}, docs)
		assert.Equal(t, "hover b.go 0:0", filter(seen, "hover")[len(filter(seen, "hover"))-1])
		stopPool(t, pool)
	})
}

func filter(seen []string, prefix string) []string {
	var out []string
	for _, s := range seen {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

func TestAServerRequestWithTheMandatoryQueueFullFailsTheServer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: func(p *peer) {
			p.serveUntil(func(m message) bool {
				if m.Method != "initialized" {
					return true
				}
				// Ask more than the queue holds and read none of the answers.
				for id := range mandatoryCap + 2 {
					p.send(message{ID: jsontext.Value(fmt.Sprint(id)), Method: "workspace/configuration", Params: jsontext.Value(`{"items":[{}]}`)})
				}
				<-p.done
				return false
			})
		}})
		_, ok := pool.Sync(doc("a.go", 1, "a\n"))
		require.True(t, ok)

		all := testsupport.Collect(t, pool, hasStatus(hover.ServerFailed))

		assert.Equal(t, hover.ServerFailed, lastStatus(all), "failed rather than lose an answer")
		stopPool(t, pool)
	})
}

// rooted is a table whose server is rooted at directories with a marker.
func rooted(t *testing.T, pool *Pool, roots ...string) {
	t.Helper()
	for _, r := range roots {
		require.NoError(t, os.MkdirAll(filepath.Join(worktree(pool), r), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(worktree(pool), r, "go.mod"), nil, 0o600))
	}
}

func TestAtMostFourInstancesRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &pipes{serve: served}
		pool := testPool(t, goTable(t, lang.Server{RootMarkers: []string{"go.mod"}}), f)
		roots := []string{"p1", "p2", "p3", "p4", "p5", "p6"}
		rooted(t, pool, roots...)

		for _, r := range roots {
			ready(t, pool, doc(r+"/a.go", 1, "package a\n"))
		}
		synctest.Wait()
		live, maxLive, started := f.counts()
		assert.Equal(t, 4, live)
		assert.LessOrEqual(t, maxLive, 4+1, "one stops while the next starts")
		assert.Equal(t, 6, started)

		// p1 was evicted first; a hover on it starts it again with its document.
		assert.Equal(t, hover.NotReady, pool.Request(hover.Position{Req: 1, Path: "p1/a.go", Version: 1}))
		testsupport.Collect(t, pool, hasStatus(hover.Ready))
		synctest.Wait()
		assert.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 2, Path: "p1/a.go", Version: 1}))
		live, _, started = f.counts()
		assert.Equal(t, 4, live)
		assert.Equal(t, 7, started)
		stopPool(t, pool)
	})
}

func TestTheLimitOfInstancesIsConfigured(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &pipes{serve: served}
		pool := newPool(Config{Root: t.TempDir(), LogDir: t.TempDir(), MaxInstances: 2},
			goTable(t, lang.Server{RootMarkers: []string{"go.mod"}}), f, zap.NewNop(), nil)
		roots := []string{"p1", "p2", "p3"}
		rooted(t, pool, roots...)

		for _, r := range roots {
			ready(t, pool, doc(r+"/a.go", 1, "package a\n"))
		}
		synctest.Wait()
		live, _, started := f.counts()
		assert.Equal(t, 2, live)
		assert.Equal(t, 3, started)
		stopPool(t, pool)
	})
}

func TestAnInstanceWithoutDocumentsEndsAfterItsIdleTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &pipes{serve: served}
		pool := testPool(t, goTable(t, lang.Server{}), f)
		ready(t, pool, doc("a.go", 1, "package a\n"))
		pool.Close(hover.DocKey{Path: "a.go"})

		time.Sleep(idleTimeout - time.Second)
		synctest.Wait()
		live, _, _ := f.counts()
		assert.Equal(t, 1, live)

		time.Sleep(time.Second)
		synctest.Wait()
		live, _, _ = f.counts()
		assert.Equal(t, 0, live)
		assert.Equal(t, hover.Stopped, lastStatus(pool.Drain()))
		stopPool(t, pool)
	})
}

func TestANewDocumentKeepsAnIdleInstance(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &pipes{serve: served}
		pool := testPool(t, goTable(t, lang.Server{}), f)
		ready(t, pool, doc("a.go", 1, "package a\n"))
		pool.Close(hover.DocKey{Path: "a.go"})
		time.Sleep(idleTimeout / 2)
		_, _ = pool.Sync(doc("b.go", 1, "package b\n"))

		time.Sleep(idleTimeout)
		synctest.Wait()
		live, _, started := f.counts()
		assert.Equal(t, 1, live)
		assert.Equal(t, 1, started)
		stopPool(t, pool)
	})
}

func TestAMissingServer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := testPool(t, goTable(t, lang.Server{Command: "missing"}), &pipes{serve: served})
		_, ok := pool.Sync(doc("a.go", 1, "package a\n"))
		require.True(t, ok)

		all := testsupport.Collect(t, pool, hasStatus(hover.Missing))
		_, _ = pool.Sync(doc("b.go", 1, "package a\n"))
		synctest.Wait()
		all = append(all, pool.Drain()...)

		assert.Equal(t, []hover.Update{{Kind: hover.StatusChanged, Instance: lang.Instance{Server: "fake", Root: "."}, Status: hover.Missing}}, all,
			"said once, and never Starting")
		assert.Equal(t, hover.Unsupported, pool.Request(hover.Position{Req: 1, Path: "a.go", Version: 1}))
		stopPool(t, pool)
	})
}

func TestNoServerForTheLanguage(t *testing.T) {
	pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: served})
	_, ok := pool.Sync(hover.Document{Path: "a.py", Lang: lang.Python, Version: 1})
	assert.False(t, ok)
	assert.Equal(t, hover.Unsupported, pool.Request(hover.Position{Req: 1, Path: "a.py", Version: 1}))
	stopPool(t, pool)
}

func TestAPanicFailsTheServer(t *testing.T) {
	tests := map[string]func(c *client){
		"in a goroutine": func(c *client) { _ = c.group.Go("boom", func(context.Context) { panic("boom") }) },
		"in a timer": func(c *client) {
			_, _ = c.group.AfterFunc(time.Millisecond, "boom", func() { panic("boom") })
		},
	}
	for name, boom := range tests {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := &pipes{serve: func(p *peer) { p.serveUntil(func(message) bool { return true }) }}
				pool := testPool(t, goTable(t, lang.Server{}), f)
				ready(t, pool, doc("a.go", 1, "package a\n"))
				require.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 1, Path: "a.go", Version: 1}))

				boom(pool.instances[lang.Instance{Server: "fake", Root: "."}].client)
				all := testsupport.Collect(t, pool, hasStatus(hover.ServerFailed))
				synctest.Wait()
				all = append(all, pool.Drain()...)

				assert.Contains(t, results(all), hover.Update{Kind: hover.HoverDone, Req: 1, Version: 1, Outcome: hover.Error})
				live, _, _ := f.counts()
				assert.Equal(t, 0, live, "the server is gone")
				stopPool(t, pool)
			})
		})
	}
}

func TestALateStoppedOfAnEvictedServerDoesNotHideItsSuccessor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := testPool(t, goTable(t, lang.Server{RootMarkers: []string{"go.mod"}}), &pipes{serve: served})
		roots := []string{"p1", "p2", "p3", "p4", "p5"}
		rooted(t, pool, roots...)
		for _, r := range roots[:4] {
			ready(t, pool, doc(r+"/a.go", 1, "package a\n"))
		}
		_ = pool.Drain()

		// p5 evicts p1, and p1 is wanted back while its old server still stops.
		_, _ = pool.Sync(doc("p5/a.go", 1, "package a\n"))
		assert.Equal(t, hover.NotReady, pool.Request(hover.Position{Req: 1, Path: "p1/a.go", Version: 1}))
		synctest.Wait()
		time.Sleep(5 * time.Second)
		synctest.Wait()

		p1 := lang.Instance{Server: "fake", Root: "p1"}
		var last hover.ServerStatus
		for _, u := range pool.Drain() {
			if u.Kind == hover.StatusChanged && u.Instance == p1 {
				last = u.Status
			}
		}
		assert.Equal(t, hover.Ready, last)
		assert.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 2, Path: "p1/a.go", Version: 1}))
		stopPool(t, pool)
	})
}

func TestAStopWhileTheServerStartsEndsIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &pipes{serve: served, gate: make(chan struct{})}
		pool := testPool(t, goTable(t, lang.Server{}), f)
		_, _ = pool.Sync(doc("a.go", 1, "package a\n"))
		synctest.Wait()

		stopped := make(chan struct{})
		go func() {
			stopPool(t, pool)
			close(stopped)
		}()
		synctest.Wait()
		close(f.gate)
		<-stopped
		synctest.Wait() // the peer counts itself out after it has ended

		live, _, started := f.counts()
		assert.Equal(t, 1, started)
		assert.Equal(t, 0, live, "ended once it had started")
	})
}

func TestAFailedServerStaysFailed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &pipes{serve: func(*peer) {}}
		pool := testPool(t, goTable(t, lang.Server{}), f)
		_, _ = pool.Sync(doc("a.go", 1, "package a\n"))
		testsupport.Collect(t, pool, hasStatus(hover.ServerFailed))

		pool.Close(hover.DocKey{Path: "a.go"})
		_, ok := pool.Sync(doc("a.go", 2, "package a\n"))
		synctest.Wait()

		require.True(t, ok)
		_, _, started := f.counts()
		assert.Equal(t, 1, started, "not started again")
		assert.Equal(t, hover.Failed, pool.Request(hover.Position{Req: 1, Path: "a.go", Version: 2}))
		stopPool(t, pool)
	})
}

func TestTextThatIsNoUTF8GoesWithReplacementCharacters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var seen []string
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: func(p *peer) {
			p.serveUntil(func(m message) bool {
				seen = append(seen, describe(m))
				return true
			})
		}})
		ready(t, pool, doc("a.go", 1, "caf\xe9\n"))
		synctest.Wait()

		assert.Contains(t, seen, "didOpen a.go v1 \"caf\uFFFD\\n\"")
		stopPool(t, pool)
	})
}
