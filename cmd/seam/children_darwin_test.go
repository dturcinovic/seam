package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// children are the commands of the test's child processes that still run,
// as on Linux: a server's guard, which is this test binary started again,
// counts as the server it guards.
func children(t *testing.T) []string {
	t.Helper()
	ps := exec.CommandContext(t.Context(), "ps", "-A", "-o", "pid=,ppid=,stat=,comm=")
	out, err := ps.Output()
	require.NoError(t, err)
	byParent := map[string][]psEntry{}
	for line := range strings.Lines(string(out)) {
		// The command is last and may hold blanks.
		fields := strings.Fields(line)
		// ps lists itself, a child of the test too.
		if len(fields) < 4 || strings.HasPrefix(fields[2], "Z") || fields[0] == strconv.Itoa(ps.Process.Pid) {
			continue
		}
		command := strings.Join(fields[3:], " ")
		byParent[fields[1]] = append(byParent[fields[1]], psEntry{pid: fields[0], name: filepath.Base(command)})
	}
	self, err := os.Executable()
	require.NoError(t, err)
	var running []string
	for _, p := range byParent[strconv.Itoa(os.Getpid())] {
		if p.name != filepath.Base(self) {
			running = append(running, p.name)
			continue
		}
		for _, server := range byParent[p.pid] {
			running = append(running, server.name)
		}
	}
	return running
}

// psEntry is a process as ps lists it.
type psEntry struct{ pid, name string }
