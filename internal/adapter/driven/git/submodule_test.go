package git_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/testsupport"
)

// withSubmodule is a repository with a committed submodule at vendor/lib.
func withSubmodule(t *testing.T) fixture {
	t.Helper()
	lib := testsupport.NewGitRepo(t)
	lib.Write("lib.txt", "lib\n")
	lib.Git("add", "lib.txt")
	lib.Git("commit", "-q", "-m", "lib")
	super := fixture{GitRepo: testsupport.NewGitRepo(t)}
	super.Git("-c", "protocol.file.allow=always", "submodule", "add", "-q", lib.Dir, "vendor/lib")
	super.Git("commit", "-q", "-m", "add submodule")
	return super
}

func TestASubmoduleIsReadAsGitDiffShowsIt(t *testing.T) {
	commit := func(super fixture) { super.Git("-C", "vendor/lib", "commit", "-q", "--allow-empty", "-m", "next") }
	edit := func(super fixture) { super.Write("vendor/lib/lib.txt", "changed\n") }
	tests := []struct {
		name   string
		change func(super fixture)
	}{
		{"new commit", commit},
		{"changed worktree", edit},
		{"new commit and an untracked file", func(super fixture) {
			commit(super)
			super.Write("vendor/lib/untracked.txt", "u\n")
		}},
		{"dirty but ignored so in .gitmodules", func(super fixture) {
			commit(super)
			edit(super)
			super.Git("config", "-f", ".gitmodules", "submodule.vendor/lib.ignore", "dirty")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			super := withSubmodule(t)
			tt.change(super)

			id, dirty, err := super.open(t).ReadSubmodule(t.Context(), "vendor/lib")
			require.NoError(t, err)
			assert.Equal(t, newSide(t, super.Git("diff", "--", "vendor/lib")), change.SubprojectText(id, dirty))
		})
	}
}

func TestASubmoduleThatIsNotCheckedOut(t *testing.T) {
	super := withSubmodule(t)
	require.NoError(t, os.RemoveAll(filepath.Join(super.Dir, "vendor/lib")))
	require.NoError(t, os.Mkdir(filepath.Join(super.Dir, "vendor/lib"), 0o700))

	_, _, err := super.open(t).ReadSubmodule(t.Context(), "vendor/lib")
	assert.ErrorIs(t, err, change.ErrNotExist, "git would answer for the repository around it")
}

// newSide is the text of the new side in git's diff of one submodule.
func newSide(t *testing.T, patch string) string {
	t.Helper()
	for line := range strings.Lines(patch) {
		if text, ok := strings.CutPrefix(line, "+Subproject commit "); ok {
			return "Subproject commit " + text
		}
	}
	require.Fail(t, "no new side", patch)
	return ""
}
