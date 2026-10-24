package git_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"seam/internal/adapter/driven/git"
	"seam/internal/testsupport"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// nlName has a newline in it, which a line-based protocol would split.
const nlName = "new\nline.txt"

// renamedContent is long enough for git's rename detection to pair the
// old and new name.
var renamedContent = strings.Repeat("a line that stays the same\n", 20)

// fixture is a repository with a file in three versions (HEAD, index,
// worktree) and every kind of change seam has to list:
//
//	first   a.txt=one   gone.txt  old-name.txt  bin.dat=\0\1  nlName=nl one
//	HEAD    a.txt=HEAD                          bin.dat=\0\2  nlName=nl HEAD
//	index   a.txt=INDEX  added.txt  gone.txt removed  old-name.txt → new-name.txt  nlName=nl index
//	work    a.txt=WORK   nlName=nl work  untracked.txt  ignored.log
type fixture struct {
	*testsupport.GitRepo
	first string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	f := fixture{GitRepo: testsupport.NewGitRepo(t)}

	f.Write(".gitignore", "*.log\n")
	f.Write("a.txt", "one\n")
	f.Write("gone.txt", "bye\n")
	f.Write("old-name.txt", renamedContent)
	f.Write("bin.dat", "\x00\x01")
	f.Write(nlName, "nl one\n")
	f.Git("add", "-A")
	f.Git("commit", "-q", "-m", "first")
	f.first = strings.TrimSpace(f.Git("rev-parse", "HEAD"))

	f.Write("a.txt", "HEAD\n")
	f.Write("bin.dat", "\x00\x02")
	f.Write(nlName, "nl HEAD\n")
	f.Git("commit", "-q", "-a", "-m", "second")

	f.Write("a.txt", "INDEX\n")
	f.Write("added.txt", "added\n")
	f.Write(nlName, "nl index\n")
	f.Git("add", "a.txt", "added.txt", nlName)
	f.Git("rm", "-q", "gone.txt")
	f.Git("mv", "old-name.txt", "new-name.txt")

	f.Write("a.txt", "WORK\n")
	f.Write(nlName, "nl work\n")
	f.Write("untracked.txt", "u\n")
	f.Write("ignored.log", "ignored\n")
	return f
}

func (f fixture) open(t *testing.T) *git.Repo {
	t.Helper()
	repo, err := git.Open(f.Dir, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, repo.Close(t.Context())) })
	return repo
}
