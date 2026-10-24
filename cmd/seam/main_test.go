package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// The session's logger starts lumberjack's goroutine; why it is exempt is in
// the TestMain of internal/observability.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"))
}

func runSeam(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(t.Context(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersion(t *testing.T) {
	code, stdout, stderr := runSeam(t, "--version")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "seam version ")
	assert.Empty(t, stderr)
}

func TestHelp(t *testing.T) {
	code, stdout, _ := runSeam(t, "--help")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "--verbose")
}

func TestUnknownFlagFails(t *testing.T) {
	code, _, stderr := runSeam(t, "--no-such-flag")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unknown flag")
}

func TestComparisonArgumentsAreNotAcceptedYet(t *testing.T) {
	code, _, _ := runSeam(t, "HEAD~1")
	assert.Equal(t, 1, code)
}

func TestSessionLogsToTheStateDirectoryOnly(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	code, stdout, stderr := runSeam(t, "--verbose")

	require.Equal(t, 0, code, stderr)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
	assert.FileExists(t, filepath.Join(state, "seam", "seam.log"))
}
