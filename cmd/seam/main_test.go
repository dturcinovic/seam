package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

// The session's logger starts lumberjack's goroutine, chroma regexp2's
// clock; why they are exempt is in the TestMain of internal/observability
// and internal/adapter/driven/chroma.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("gopkg.in/natefinch/lumberjack%2ev2.(*Logger).millRun"),
		goleak.IgnoreAnyFunction("github.com/dlclark/regexp2/v2.runClock"))
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
