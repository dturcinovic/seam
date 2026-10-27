package process_test

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/adapter/driven/process"
)

func TestAProcessIsAliveUntilItEnds(t *testing.T) {
	p := process.New()
	assert.True(t, p.Alive(os.Getpid()))
	assert.True(t, p.Alive(1), "another user's process")
	assert.False(t, p.Alive(0))
	assert.False(t, p.Alive(-1))

	cmd := exec.CommandContext(t.Context(), "true")
	require.NoError(t, cmd.Run())
	assert.False(t, p.Alive(cmd.Process.Pid), "ended and reaped")
}
