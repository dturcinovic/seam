package lsp

import (
	"bufio"
	"context"
	"encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/limits"
	"github.com/dturcinovic/seam/internal/testsupport"
)

// processPool runs fakelsp for .go files, rooted at go.mod; each root
// gets the script given for it.
func processPool(t *testing.T, scripts map[string]string) *Pool {
	t.Helper()
	root := t.TempDir()
	for dir, script := range scripts {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(root, dir, "go.mod"), nil, 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(root, dir, "fakelsp.script"), []byte(script), 0o600))
	}
	table := goTable(t, lang.Server{Command: fakelsp(), RootMarkers: []string{"go.mod"}})
	return newPool(Config{Root: root, LogDir: t.TempDir(), MaxInstances: limits.DefaultInstances}, table, newProcesses(t.TempDir()), zap.NewNop(), nil)
}

func readyAll(t *testing.T, pool *Pool, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		_, ok := pool.Sync(doc(dir+"/a.go", 1, "package a\n"))
		require.True(t, ok)
	}
	testsupport.Collect(t, pool, func(all []hover.Update) bool { return countStatus(all, hover.Ready) == len(dirs) })
}

func pid(pool *Pool, dir string) int {
	c := pool.instances[lang.Instance{Server: "fake", Root: dir}].client
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.pid
}

// groupGone reports whether no process is left in the group of pgid.
func groupGone(pgid int) bool {
	return errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH)
}

// fakelspLog is what fakelsp in dir read, each message as describe names
// it.
func fakelspLog(t *testing.T, pool *Pool, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(worktree(pool), dir, "fakelsp.log"))
	require.NoError(t, err)
	var seen []string
	for line := range strings.Lines(string(data)) {
		var m message
		if json.Unmarshal([]byte(line), &m) != nil {
			seen = append(seen, strings.TrimSpace(line))
			continue
		}
		seen = append(seen, describe(m))
	}
	return seen
}

func TestShutdownUnderPressure(t *testing.T) {
	pool := processPool(t, map[string]string{"reads-not": "stop-reading", "answers-not": "no-shutdown", "has-a-child": "child"})
	dirs := []string{"reads-not", "answers-not", "has-a-child"}
	readyAll(t, pool, dirs...)
	var groups []int
	for _, d := range dirs {
		groups = append(groups, pid(pool, d))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	begin := time.Now()
	require.NoError(t, pool.Stop(ctx))

	assert.Less(t, time.Since(begin), 3*time.Second-stopReserve, "the reserve is left to the hooks after")
	for i, g := range groups {
		assert.Eventually(t, func() bool { return groupGone(g) }, time.Second, 10*time.Millisecond, "process group of %s", dirs[i])
	}
}

func TestACrashedServerFails(t *testing.T) {
	pool := processPool(t, map[string]string{"p": "crash"})
	_, ok := pool.Sync(doc("p/a.go", 1, "package a\n"))
	require.True(t, ok)

	testsupport.Collect(t, pool, hasStatus(hover.ServerFailed))

	assert.Equal(t, hover.Failed, pool.Request(hover.Position{Req: 1, Path: "p/a.go", Version: 1}))
	stopPool(t, pool)
}

func TestALateAnswerThroughARealPipe(t *testing.T) {
	pool := processPool(t, map[string]string{"p": "hover func F()\nhover-delay 800ms"})
	readyAll(t, pool, "p")
	opened := `didOpen a.go v1 "package a\n"`
	require.Eventually(t, func() bool { return slices.Contains(fakelspLog(t, pool, "p"), opened) }, testsupport.Guard, 10*time.Millisecond)
	_, _ = pool.Sync(doc("p/a.go", 2, "package a\n\nfunc F() {}\n"))

	require.Equal(t, hover.Sent, pool.Request(hover.Position{Req: 1, Path: "p/a.go", Version: 2, Line: 2, Byte: 5}))
	got := results(testsupport.Collect(t, pool, hasResult))

	assert.Equal(t, []hover.Update{{Kind: hover.HoverDone, Req: 1, Version: 2, Outcome: hover.Text, Content: paragraph("func F()")}}, got)
	stopPool(t, pool)
	log := fakelspLog(t, pool, "p")
	assert.Equal(t, []string{
		"initialize",
		"initialized",
		opened,
		`didChange a.go v2 "package a\n\nfunc F() {}\n"`,
		"hover a.go 2:5",
		"shutdown",
		"exit",
	}, log)
}

// TestServersEndWithAKilledSeam: Pdeathsig ends a server when seam is
// killed with SIGKILL and cannot stop it itself.
func TestServersEndWithAKilledSeam(t *testing.T) {
	root := t.TempDir()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), helperEnv+"="+root)
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	line, err := bufio.NewReader(out).ReadString('\n')
	require.NoError(t, err)
	server, err := strconv.Atoi(strings.TrimSpace(line))
	require.NoError(t, err)

	require.NoError(t, cmd.Process.Kill())
	_ = cmd.Wait() // killed, as intended

	assert.Eventually(t, func() bool { return groupGone(server) }, 2*time.Second, 10*time.Millisecond)
}
