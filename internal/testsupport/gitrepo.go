package testsupport

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// GitRepo is a throwaway repository for one test.
type GitRepo struct {
	t   *testing.T
	Dir string
}

// NewGitRepo makes an empty repository on branch main. It shuts out the
// user's and the system's git configuration, so hooks, signing or a default
// branch name cannot change what the test sees, and the repository of a git
// hook the tests run in, as the pre-commit hook; it therefore sets
// environment variables and rules out t.Parallel.
func NewGitRepo(t *testing.T) *GitRepo {
	t.Helper()
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_PREFIX"} {
		t.Setenv(name, "") // restores the value after the test
		require.NoError(t, os.Unsetenv(name))
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "seam")
	t.Setenv("GIT_AUTHOR_EMAIL", "seam@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "seam")
	t.Setenv("GIT_COMMITTER_EMAIL", "seam@example.com")
	r := &GitRepo{t: t, Dir: t.TempDir()}
	r.Git("init", "-q", "-b", "main")
	return r
}

// Git runs git in the repository and returns its output.
func (r *GitRepo) Git(args ...string) string {
	r.t.Helper()
	cmd := exec.CommandContext(r.t.Context(), "git", append([]string{"-C", r.Dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(r.t, err, "git %v: %s", args, out)
	return string(out)
}

// Write writes a file of the worktree.
func (r *GitRepo) Write(name, content string) {
	r.t.Helper()
	require.NoError(r.t, os.WriteFile(filepath.Join(r.Dir, name), []byte(content), 0o600))
}
