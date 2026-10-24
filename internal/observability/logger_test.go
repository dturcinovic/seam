package observability_test

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"seam/internal/config"
	"seam/internal/observability"
	"seam/internal/supervise"
)

// lumberjack starts its compression goroutine on the first write and never
// stops it; it outlives every logger by design and is the one exception.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"))
}

func readLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var lines []map[string]any
	for line := range strings.Lines(string(data)) {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		lines = append(lines, entry)
	}
	return lines
}

func TestLoggerWritesJSONToTheFile(t *testing.T) {
	cfg := config.Config{LogFile: filepath.Join(t.TempDir(), "nested", "seam.log")}
	l, err := observability.NewLogger(cfg)
	require.NoError(t, err)

	l.Zap().Info("hello")
	l.Zap().Debug("hidden without verbose")
	require.NoError(t, l.Close())

	lines := readLines(t, cfg.LogFile)
	require.Len(t, lines, 1)
	assert.Equal(t, "hello", lines[0]["msg"])
}

func TestVerboseLogsDebug(t *testing.T) {
	cfg := config.Config{LogFile: filepath.Join(t.TempDir(), "seam.log"), Verbose: true}
	l, err := observability.NewLogger(cfg)
	require.NoError(t, err)

	l.Zap().Debug("shown")
	require.NoError(t, l.Close())

	assert.Len(t, readLines(t, cfg.LogFile), 1)
}

func TestUnwritableLogDirectory(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))

	_, err := observability.NewLogger(config.Config{LogFile: filepath.Join(blocker, "seam.log")})
	assert.ErrorContains(t, err, "create log directory")
}

func TestPanicFuncLogsTheFailure(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)

	observability.PanicFunc(zap.New(core))("lsp", supervise.Failure{Task: "read", Value: "kaputt", Stack: []byte("trace")})

	entries := logs.All()
	require.Len(t, entries, 1)
	assert.Equal(t, map[string]any{
		"owner": "lsp", "task": "read", "value": "kaputt", "stack": "trace",
	}, entries[0].ContextMap())
}
