package lsp

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
	"seam/internal/testsupport"
)

// realPool runs the default servers, with the files given written into a
// fresh repository root.
func realPool(t *testing.T, table lang.Table, files map[string]string) *Pool {
	t.Helper()
	root := t.TempDir()
	for name, text := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(text), 0o600))
	}
	return newPool(Config{Root: root, LogDir: t.TempDir()}, table, newProcesses(t.TempDir()), zap.NewNop(), nil)
}

// hoverUntil asks at pos until an answer has text: a server answers
// empty while it still loads the workspace.
func hoverUntil(t *testing.T, pool *Pool, pos hover.Position, within time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		pos.Req++
		if s := pool.Request(pos); s != hover.Sent {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		for _, u := range results(testsupport.Collect(t, pool, hasResult)) {
			if u.Outcome == hover.Text {
				return shownContent(u.Content)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("no hover text within %v", within)
	return ""
}

func TestGopls(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls is not installed")
	}
	text := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n"
	pool := realPool(t, lang.Defaults(), map[string]string{"go.mod": "module m\n\ngo 1.27\n", "main.go": text})
	_, ok := pool.Sync(hover.Document{Path: "main.go", Lang: lang.Go, Version: 1, Text: text})
	require.True(t, ok)
	testsupport.Collect(t, pool, hasStatus(hover.Ready))

	line := strings.Split(text, "\n")[5]
	got := hoverUntil(t, pool, hover.Position{Path: "main.go", Version: 1, Line: 5, Byte: strings.Index(line, "Println")}, 30*time.Second)

	assert.Contains(t, got, "func fmt.Println(a ...any)", "gopls names the package")
	stopPool(t, pool)
}

func TestAServerThatIsNotInstalledIsMissing(t *testing.T) {
	d := lang.Defaults()
	servers := d.Servers()
	for i := range servers {
		servers[i].Command = "/no/such/" + servers[i].Command
	}
	table, err := lang.NewTable(d.Languages(), servers)
	require.NoError(t, err)
	pool := realPool(t, table, nil)

	_, ok := pool.Sync(hover.Document{Path: "main.go", Lang: lang.Go, Version: 1, Text: "package main\n"})
	require.True(t, ok)

	testsupport.Collect(t, pool, hasStatus(hover.Missing))
	assert.Equal(t, hover.Unsupported, pool.Request(hover.Position{Req: 1, Path: "main.go", Version: 1}))
	stopPool(t, pool)
}
