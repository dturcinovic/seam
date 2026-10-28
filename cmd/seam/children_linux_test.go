package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// children are the commands of the test's child processes that still run.
func children(t *testing.T) []string {
	t.Helper()
	self := strconv.Itoa(os.Getpid())
	stats, err := filepath.Glob("/proc/[0-9]*/stat")
	require.NoError(t, err)
	var running []string
	for _, p := range stats {
		data, err := os.ReadFile(p)
		if err != nil {
			continue // ended meanwhile
		}
		// The command is in parentheses and may hold blanks; the fields
		// after it are state and parent.
		s := string(data)
		end := strings.LastIndexByte(s, ')')
		fields := strings.Fields(s[end+1:])
		if len(fields) > 1 && fields[1] == self && fields[0] != "Z" {
			running = append(running, s[strings.IndexByte(s, '(')+1:end])
		}
	}
	return running
}
