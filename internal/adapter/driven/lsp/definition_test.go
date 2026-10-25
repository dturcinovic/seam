package lsp

import (
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/testsupport"
)

// definitions are the DefinitionDone updates among all.
func definitions(all []hover.Update) []hover.Update {
	var r []hover.Update
	for _, u := range all {
		if u.Kind == hover.DefinitionDone {
			r = append(r, u)
		}
	}
	return r
}

// defining answers each definition request with result, in which ROOT
// stands for the file URI of the root.
func defining(result string, root func() string) func(p *peer) {
	return func(p *peer) {
		p.serveUntil(func(m message) bool {
			if m.Method == "textDocument/definition" {
				p.reply(m.ID, strings.ReplaceAll(result, "ROOT", fileURI(root())))
			}
			return true
		})
	}
}

func TestDefinitionsInEveryForm(t *testing.T) {
	found := func(path string, line, targets int) hover.Update {
		return hover.Update{Kind: hover.DefinitionDone, Req: 1, Version: 1, Outcome: hover.Text,
			Target: hover.Location{Path: path, Line: line}, Targets: targets}
	}
	tests := map[string]struct {
		result string
		want   hover.Update
	}{
		"a location": {`{"uri":"ROOT/b.go","range":{"start":{"line":4,"character":5},"end":{"line":4,"character":6}}}`, found("b.go", 4, 1)},
		"locations, the first of them": {
			`[{"uri":"ROOT/sub/c.go","range":{"start":{"line":9,"character":0}}},{"uri":"ROOT/b.go","range":{"start":{"line":1,"character":0}}}]`,
			found("sub/c.go", 9, 2),
		},
		"a link at its name": {
			`[{"targetUri":"ROOT/b.go","targetRange":{"start":{"line":2}},"targetSelectionRange":{"start":{"line":3}}}]`,
			found("b.go", 3, 1),
		},
		"none":        {`null`, hover.Update{Kind: hover.DefinitionDone, Req: 1, Version: 1, Outcome: hover.Empty}},
		"none listed": {`[]`, hover.Update{Kind: hover.DefinitionDone, Req: 1, Version: 1, Outcome: hover.Empty}},
		"outside the tree": {
			`{"uri":"file:///usr/lib/go/src/fmt/print.go","range":{"start":{"line":41}}}`,
			hover.Update{Kind: hover.DefinitionDone, Req: 1, Version: 1, Outcome: hover.Text, Targets: 1,
				Target: hover.Location{Path: filepath.FromSlash("/usr/lib/go/src/fmt/print.go"), Outside: true, Line: 41}},
		},
		"the first that names a file": {
			`[{"uri":"untitled:x","range":{"start":{"line":1}}},{"uri":"ROOT/b.go","range":{"start":{"line":2}}}]`,
			found("b.go", 2, 2),
		},
		"no file":     {`{"uri":"untitled:x","range":{"start":{"line":1}}}`, hover.Update{Kind: hover.DefinitionDone, Req: 1, Version: 1, Outcome: hover.Error}},
		"not a range": {`{"uri":3}`, hover.Update{Kind: hover.DefinitionDone, Req: 1, Version: 1, Outcome: hover.Error}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var pool *Pool
				pool = testPool(t, goTable(t, lang.Server{}), &pipes{serve: defining(tt.result, func() string { return worktree(pool) })})
				ready(t, pool, doc("a.go", 1, "package a\n"))

				require.Equal(t, hover.Sent, pool.RequestDefinition(hover.Position{Req: 1, Path: "a.go", Version: 1}))
				all := testsupport.Collect(t, pool, func(all []hover.Update) bool { return len(definitions(all)) > 0 })

				assert.Equal(t, []hover.Update{tt.want}, definitions(all))
				assert.Empty(t, results(all), "no hover answered")
				stopPool(t, pool)
			})
		})
	}
}

func TestAnUnansweredDefinitionTimesOut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: served})
		ready(t, pool, doc("a.go", 1, "package a\n"))

		require.Equal(t, hover.Sent, pool.RequestDefinition(hover.Position{Req: 1, Path: "a.go", Version: 1}))
		time.Sleep(hoverTimeout)
		synctest.Wait()

		assert.Equal(t, []hover.Update{{Kind: hover.DefinitionDone, Req: 1, Version: 1, Outcome: hover.TimedOut}}, definitions(pool.Drain()))
		stopPool(t, pool)
	})
}

func TestACancelledDefinitionIsNotAnswered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: served})
		ready(t, pool, doc("a.go", 1, "package a\n"))

		require.Equal(t, hover.Sent, pool.RequestDefinition(hover.Position{Req: 1, Path: "a.go", Version: 1}))
		pool.CancelDefinition(1)
		time.Sleep(hoverTimeout)
		synctest.Wait()

		assert.Empty(t, definitions(pool.Drain()))
		stopPool(t, pool)
	})
}

func TestADefinitionWithoutAServer(t *testing.T) {
	pool := testPool(t, goTable(t, lang.Server{}), &pipes{serve: served})

	assert.Equal(t, hover.Unsupported, pool.RequestDefinition(hover.Position{Req: 1, Path: "a.go", Version: 1}))
	stopPool(t, pool)
}

func TestADefinitionInACheckoutIsInItsCommit(t *testing.T) {
	id, err := change.ParseObjectID(strings.Repeat("c", 40))
	require.NoError(t, err)
	root := filepath.FromSlash("/cache/checkout")
	c := &client{repoRoot: root, inst: lang.Instance{Commit: id}}

	got, ok := c.located(location{URI: fileURI(filepath.Join(root, "sub", "a.go"))})

	require.True(t, ok)
	assert.Equal(t, hover.Location{Commit: id, Path: "sub/a.go"}, got)
	got, _ = c.located(location{URI: fileURI(filepath.FromSlash("/cache/checkout-other/a.go"))})
	assert.True(t, got.Outside, "a sibling that shares the prefix is outside")
}
