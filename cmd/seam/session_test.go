package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/testsupport"
)

// inRepo makes a repository with two commits, a staged and an unstaged
// change and an untracked file, and runs the test from inside it.
func inRepo(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	// Not the configuration file of whoever runs the tests.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := testsupport.NewGitRepo(t)
	t.Chdir(repo.Dir)
	repo.Write("committed.txt", "1\n")
	repo.Git("add", ".")
	repo.Git("commit", "-q", "-m", "first")
	repo.Git("tag", "first")
	repo.Write("committed.txt", "2\n")
	repo.Git("commit", "-q", "-a", "-m", "second")
	repo.Write("staged.txt", "s\n")
	repo.Git("add", "staged.txt")
	repo.Write("committed.txt", "3\n")
	repo.Write("untracked\x1b[31m.txt", "u\n")
}

func TestModesOnTheCommandLine(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, "M\tcommitted.txt\nA\tstaged.txt\n?\t\"untracked\\x1b[31m.txt\"\n"},
		{[]string{"first"}, "M\tcommitted.txt\nA\tstaged.txt\n?\t\"untracked\\x1b[31m.txt\"\n"},
		{[]string{"first", "main"}, "M\tcommitted.txt\n"},
		{[]string{"--staged"}, "A\tstaged.txt\n"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(append([]string{"seam"}, tt.args...), " "), func(t *testing.T) {
			inRepo(t)

			code, stdout, stderr := runSeam(t, tt.args...)

			require.Equal(t, 0, code, stderr)
			assert.Equal(t, tt.want, stdout)
			assert.Empty(t, stderr)
		})
	}
}

func TestSessionLogsToTheStateDirectory(t *testing.T) {
	inRepo(t)

	code, _, stderr := runSeam(t, "--verbose")

	require.Equal(t, 0, code, stderr)
	assert.FileExists(t, filepath.Join(os.Getenv("XDG_STATE_HOME"), "seam", "seam.log"))
}

// configure writes text as the configuration file of the session.
func configure(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "seam", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o700))
	require.NoError(t, os.WriteFile(p, []byte(text), 0o600))
	return p
}

func TestABrokenConfigurationStopsTheStart(t *testing.T) {
	inRepo(t)
	p := configure(t, "[servers.gopls]\ncomand = \"gopls\"\n")

	code, stdout, stderr := runSeam(t)

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "seam: "+p+":2:1: unknown key servers.gopls.comand\n", stderr)
}

func TestAMissingServerDoesNotStopTheStart(t *testing.T) {
	inRepo(t)
	configure(t, "servers.gopls.command = \"/no/such/gopls\"\n")

	code, stdout, stderr := runSeam(t, "--staged")

	require.Equal(t, 0, code, stderr)
	assert.Equal(t, "A\tstaged.txt\n", stdout)
}

func TestCommandLineErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown revision", []string{"no-such-branch"}, `seam: unknown revision "no-such-branch"` + "\n"},
		{"staged with a revision", []string{"--staged", "first"}, "seam: invalid comparison: --staged compares HEAD with the index and takes no revision\n"},
		{"three revisions", []string{"a", "b", "c"}, "seam: accepts at most 2 arg(s), received 3\n"},
		{"empty revision", []string{""}, "seam: invalid comparison: the left side must be a commit\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inRepo(t)

			code, stdout, stderr := runSeam(t, tt.args...)

			assert.Equal(t, 1, code)
			assert.Empty(t, stdout)
			assert.Equal(t, tt.want, stderr)
		})
	}
}

func TestOutsideARepository(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	dir := t.TempDir()
	t.Chdir(dir)

	code, stdout, stderr := runSeam(t)

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "seam: "+dir+": not inside a git worktree\n", stderr)
}

func TestErrorsAreMadePrintable(t *testing.T) {
	assert.Equal(t, "first line\n\"path\\x1b[2J: gone\"", printableLines("first line\npath\x1b[2J: gone"))
}

func TestPrintable(t *testing.T) {
	assert.Equal(t, "plain name.go", printable("plain name.go"))
	assert.Equal(t, "ünïcödé.go", printable("ünïcödé.go"))
	assert.Equal(t, `"new\nline"`, printable("new\nline"))
	assert.Equal(t, `"tab\there"`, printable("tab\there"))
	assert.Equal(t, `"bidi\u202e"`, printable("bidi\u202e"))
	assert.Equal(t, `"bad\xff"`, printable("bad\xff"))
}
