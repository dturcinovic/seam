package lsp

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/change"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
	"seam/internal/testsupport"
)

func TestACommitsDocumentsAreServedFromItsCheckout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var mu sync.Mutex
		var opened []string
		f := &pipes{serve: func(p *peer) {
			p.serveUntil(func(m message) bool {
				if m.Method == "textDocument/didOpen" {
					var params struct {
						TextDocument struct {
							URI string `json:"uri"`
						} `json:"textDocument"`
					}
					require.NoError(t, json.Unmarshal(m.Params, &params))
					mu.Lock()
					opened = append(opened, params.TextDocument.URI)
					mu.Unlock()
				}
				return true
			})
		}}
		pool := testPool(t, goTable(t, lang.Server{RootMarkers: []string{"go.mod"}}), f)
		id, err := change.ParseObjectID(strings.Repeat("c", 40))
		require.NoError(t, err)
		// The commit has a module in sub, which the worktree has not.
		checkout := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(checkout, "sub"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(checkout, "sub", "go.mod"), nil, 0o600))
		old := hover.Document{Commit: id, Path: "sub/a.go", Lang: lang.Go, Version: 1, Text: "package old\n"}

		_, ok := pool.Sync(old)
		assert.False(t, ok, "no instance before the checkout is mounted")
		assert.Equal(t, hover.Unsupported, pool.Request(hover.Position{Req: 1, Commit: id, Path: "sub/a.go", Version: 1}))

		pool.Mount(id, checkout)
		inst, ok := pool.Sync(old)
		require.True(t, ok)
		assert.Equal(t, lang.Instance{Server: "fake", Commit: id, Root: "sub"}, inst)
		now, ok := pool.Sync(doc("sub/a.go", 1, "package now\n"))
		require.True(t, ok)
		assert.Equal(t, lang.Instance{Server: "fake", Root: "."}, now, "the worktree's is another instance")
		testsupport.Collect(t, pool, func(all []hover.Update) bool { return countStatus(all, hover.Ready) == 2 })
		synctest.Wait()

		assert.ElementsMatch(t, []string{filepath.Join(checkout, "sub"), worktree(pool)}, f.dirs)
		mu.Lock()
		assert.ElementsMatch(t, []string{fileURI(filepath.Join(checkout, "sub", "a.go")), fileURI(filepath.Join(worktree(pool), "sub", "a.go"))}, opened)
		mu.Unlock()
		assert.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 2, Commit: id, Path: "sub/a.go", Version: 1}))

		pool.Close(old.Key())
		assert.Equal(t, hover.Unsupported, pool.Request(hover.Position{Req: 3, Commit: id, Path: "sub/a.go", Version: 1}))
		assert.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 4, Path: "sub/a.go", Version: 1}), "closing the commit's leaves the worktree's")
		stopPool(t, pool)
	})
}
