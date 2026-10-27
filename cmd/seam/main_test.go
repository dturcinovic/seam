package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/dturcinovic/seam/internal/adapter/driven/statefile"
	"github.com/dturcinovic/seam/internal/app/port/in/infakes"
	"github.com/dturcinovic/seam/internal/config"
	"github.com/dturcinovic/seam/internal/domain/release"
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
	code = run(t.Context(), args, strings.NewReader(""), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersion(t *testing.T) {
	code, stdout, stderr := runSeam(t, "--version")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "seam version ")
	assert.Empty(t, stderr)
}

func TestVersionNamesANewerRelease(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SEAM_NO_UPDATE_CHECK", "")
	cfg, err := config.Load(config.Flags{})
	require.NoError(t, err)
	latest, err := release.ParseVersion("v0.2.0")
	require.NoError(t, err)
	require.NoError(t, statefile.New(statefile.Config{ReleaseFile: cfg.ReleaseFile}).SaveCheck(t.Context(), release.Check{Latest: latest, At: time.Now()}))
	stamped := version
	t.Cleanup(func() { version = stamped })
	version = "v0.1.0"

	code, stdout, _ := runSeam(t, "--version", "extra", "args", "are", "ignored")

	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "seam version v0.1.0")
	assert.Contains(t, stdout, "seam v0.2.0 is out (you have v0.1.0): ")

	t.Setenv("SEAM_NO_UPDATE_CHECK", "1")
	_, stdout, _ = runSeam(t, "--version")
	assert.NotContains(t, stdout, "is out", "turned off, nothing is told")
}

func TestTheNoticeGoesToStderr(t *testing.T) {
	running, err := release.ParseVersion("v0.1.0")
	require.NoError(t, err)
	latest, err := release.ParseVersion("v0.2.0")
	require.NoError(t, err)
	releases := &infakes.FakeReleaseNotifier{}
	var stderr bytes.Buffer
	tellNewerRelease(t.Context(), releases, running, &stderr)
	assert.Empty(t, stderr.String(), "no newer release")

	releases.NewerReturns(latest, true)
	tellNewerRelease(t.Context(), releases, running, &stderr)
	assert.Contains(t, stderr.String(), "seam v0.2.0 is out (you have v0.1.0): ")
}

func TestHelp(t *testing.T) {
	code, stdout, _ := runSeam(t, "--help")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "--verbose")
	assert.Contains(t, stdout, "mcp ")
	assert.NotContains(t, stdout, "completion")
}

func TestUnknownFlagFails(t *testing.T) {
	code, _, stderr := runSeam(t, "--no-such-flag")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unknown flag")
}
