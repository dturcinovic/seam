package command_test

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/dturcinovic/seam/internal/adapter/driven/command"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestACommandWritesWhereItIsTold(t *testing.T) {
	var out, errOut bytes.Buffer
	r := command.New(command.Config{Stdin: strings.NewReader("in"), Stdout: &out, Stderr: &errOut})

	require.NoError(t, r.Run(t.Context(), "sh", "-c", "cat; echo err >&2"))
	assert.Equal(t, "in", out.String())
	assert.Equal(t, "err\n", errOut.String())
}

func TestACommandThatFailsKeepsItsExitCode(t *testing.T) {
	r := command.New(command.Config{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	err := r.Run(t.Context(), "sh", "-c", "exit 3")
	exitErr, ok := errors.AsType[*exec.ExitError](err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, 3, exitErr.ExitCode())

	require.ErrorContains(t, r.Run(t.Context(), "no-such-command-seam"), `"no-such-command-seam"`, "exec names it")
}

func TestACommandIsLeftToEndWhenSeamsContextDoes(t *testing.T) {
	var out bytes.Buffer
	r := command.New(command.Config{Stdout: &out, Stderr: &bytes.Buffer{}})
	// The context ends as Ctrl-C ends seam's, while the command runs.
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	require.NoError(t, r.Run(ctx, "sh", "-c", "sleep 0.3; echo finished"), "it did its work")
	assert.Equal(t, "finished\n", out.String(), "seam neither interrupts nor kills it")
	ctx, cancel = context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	require.Error(t, r.Run(ctx, "sh", "-c", "sleep 0.3; exit 2"), "a command that fails still fails")
}
