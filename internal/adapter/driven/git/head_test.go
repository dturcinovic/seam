package git_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/adapter/driven/git"
	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/testsupport"
)

// headRepo is a repository with two commits on main.
func headRepo(t *testing.T) *testsupport.GitRepo {
	t.Helper()
	r := testsupport.NewGitRepo(t)
	r.Write("a.txt", "one\n")
	r.Git("add", "a.txt")
	r.Git("commit", "-q", "-m", "one")
	r.Write("a.txt", "two\n")
	r.Git("commit", "-q", "-a", "-m", "two")
	return r
}

func readHead(t *testing.T, dir string) change.Head {
	t.Helper()
	h, err := git.NewHead(dir).ReadHead(t.Context())
	require.NoError(t, err)
	return h
}

func TestReadHead(t *testing.T) {
	t.Run("a branch", func(t *testing.T) {
		r := headRepo(t)
		r.Git("switch", "-q", "-c", "feature/x")
		assert.Equal(t, change.Head{Branch: "feature/x"}, readHead(t, r.Dir))
	})
	t.Run("a branch without commits", func(t *testing.T) {
		r := testsupport.NewGitRepo(t)
		assert.Equal(t, change.Head{Branch: "main"}, readHead(t, r.Dir))
	})
	t.Run("detached", func(t *testing.T) {
		r := headRepo(t)
		r.Git("switch", "-q", "--detach", "HEAD~1")
		id, err := change.ParseObjectID(strings.TrimSpace(r.Git("rev-parse", "HEAD")))
		require.NoError(t, err)
		assert.Equal(t, change.Head{Commit: id}, readHead(t, r.Dir))
	})
	t.Run("during a rebase", func(t *testing.T) {
		r := headRepo(t)
		t.Setenv("GIT_SEQUENCE_EDITOR", "sed -i.bak 1s/^pick/edit/")
		r.Git("rebase", "-q", "-i", "HEAD~1")
		require.Equal(t, "HEAD", strings.TrimSpace(r.Git("rev-parse", "--abbrev-ref", "HEAD")), "HEAD is detached while the rebase stops")
		assert.Equal(t, change.Head{Branch: "main"}, readHead(t, r.Dir))
	})
	t.Run("a linked worktree", func(t *testing.T) {
		r := headRepo(t)
		other := filepath.Join(testsupport.RealTempDir(t), "other")
		r.Git("worktree", "add", "-q", "-b", "side", other)
		assert.Equal(t, change.Head{Branch: "side"}, readHead(t, other))
		assert.Equal(t, change.Head{Branch: "main"}, readHead(t, r.Dir))
	})
}

func TestListBranches(t *testing.T) {
	r := headRepo(t)
	r.Git("branch", "feature/x")
	r.Git("branch", "gone")
	r.Git("branch", "-D", "gone")
	names, err := git.NewHead(r.Dir).ListBranches(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"feature/x", "main"}, names)
}
