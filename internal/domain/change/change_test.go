package change_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/change"
)

const sha1 = "0123456789abcdef0123456789abcdef01234567"

func TestParseObjectID(t *testing.T) {
	id, err := change.ParseObjectID(sha1)
	require.NoError(t, err)
	assert.Equal(t, sha1, id.String())
	assert.False(t, id.IsZero())

	sha256 := strings.Repeat("ab", 32)
	id, err = change.ParseObjectID(sha256)
	require.NoError(t, err)
	assert.Equal(t, sha256, id.String())

	id, err = change.ParseObjectID(strings.Repeat("0", 40))
	require.NoError(t, err)
	assert.True(t, id.IsZero(), "git's all-zero name is no object")

	for _, bad := range []string{"", "abc", strings.ToUpper(sha1), "g" + sha1[1:], sha1 + "0"} {
		_, err := change.ParseObjectID(bad)
		assert.ErrorIs(t, err, change.ErrInvalidObjectID, "%q", bad)
	}
}

func TestSources(t *testing.T) {
	id, err := change.ParseObjectID(sha1)
	require.NoError(t, err)

	c := change.Commit("main~1")
	assert.Equal(t, change.KindCommit, c.Kind())
	assert.Equal(t, "main~1", c.Rev())
	assert.True(t, c.ID().IsZero())
	resolved, err := c.Resolved(id)
	require.NoError(t, err)
	assert.Equal(t, id, resolved.ID())
	_, err = c.Resolved(change.ObjectID{})
	assert.ErrorIs(t, err, change.ErrNotACommit, "the zero id names no commit")
	_, err = change.Worktree().Resolved(id)
	assert.ErrorIs(t, err, change.ErrNotACommit)
	assert.Equal(t, "main~1", c.String())

	assert.Equal(t, change.KindIndex, change.Index().Kind())
	assert.Equal(t, "index", change.Index().String())
	assert.Equal(t, change.KindWorktree, change.Worktree().Kind())
	assert.Equal(t, "worktree", change.Worktree().String())
	assert.Equal(t, "unset", change.Source{}.String())
}

func TestPathSources(t *testing.T) {
	dir := change.Dir("/tmp/left")
	assert.Equal(t, change.KindPath, dir.Kind())
	assert.True(t, dir.IsDir())
	assert.Equal(t, "/tmp/left", dir.Path())
	assert.Equal(t, "/tmp/left/sub/a.go", dir.At("sub/a.go"))
	assert.Equal(t, "/tmp/left", dir.String())

	file := change.File("/tmp/x_a.go")
	assert.False(t, file.IsDir())
	assert.Equal(t, "/tmp/x_a.go", file.At("x_a.go"), "a file side is its own file")
	assert.Equal(t, "/dev/null", change.File("").String())

	_, err := file.Resolved(change.ObjectID{})
	assert.ErrorIs(t, err, change.ErrNotACommit)
}

func TestNewComparison(t *testing.T) {
	valid := []struct{ left, right change.Source }{
		{change.Commit("HEAD"), change.Worktree()},
		{change.Commit("HEAD"), change.Index()},
		{change.Commit("a"), change.Commit("b")},
		{change.File("a"), change.File("b")},
		{change.File(""), change.File("b")},
		{change.File("a"), change.File("")},
		{change.Dir("a"), change.Dir("b")},
	}
	for _, tt := range valid {
		c, err := change.NewComparison(tt.left, tt.right)
		require.NoError(t, err)
		assert.Equal(t, tt.left, c.Left)
		assert.Equal(t, tt.right, c.Right)
	}

	invalid := []struct {
		name        string
		left, right change.Source
	}{
		{"index on the left", change.Index(), change.Worktree()},
		{"left commit without revision", change.Commit(""), change.Worktree()},
		{"right commit without revision", change.Commit("HEAD"), change.Commit("")},
		{"right side unset", change.Commit("HEAD"), change.Source{}},
		{"file against commit", change.File("a"), change.Commit("HEAD")},
		{"commit against file", change.Commit("HEAD"), change.File("a")},
		{"file against directory", change.File("a"), change.Dir("b")},
		{"directory without path", change.Dir(""), change.Dir("b")},
		{"both files missing", change.File(""), change.File("")},
	}
	for _, tt := range invalid {
		_, err := change.NewComparison(tt.left, tt.right)
		assert.ErrorIs(t, err, change.ErrInvalidComparison, tt.name)
	}
}

func TestStatusString(t *testing.T) {
	statuses := map[change.Status]string{
		change.Added: "A", change.Deleted: "D", change.Modified: "M", change.Renamed: "R",
		change.Copied: "C", change.TypeChanged: "T", change.Unmerged: "U", change.Untracked: "?", change.Status(0): "-",
	}
	for s, want := range statuses {
		assert.Equal(t, want, s.String())
	}
}

func TestChangedFilePath(t *testing.T) {
	assert.Equal(t, "new", change.ChangedFile{OldPath: "old", NewPath: "new"}.Path())
	assert.Equal(t, "gone", change.ChangedFile{OldPath: "gone"}.Path(), "a deleted file keeps its old path")
}

func TestSubprojectText(t *testing.T) {
	id, err := change.ParseObjectID(sha1)
	require.NoError(t, err)
	assert.Equal(t, "Subproject commit "+sha1+"\n", change.SubprojectText(id, false))
	assert.Equal(t, "Subproject commit "+sha1+"-dirty\n", change.SubprojectText(id, true))
}

func TestTooLargeError(t *testing.T) {
	assert.EqualError(t, &change.TooLargeError{Size: 42}, "file too large: 42 bytes")
}

func TestIsBinary(t *testing.T) {
	assert.False(t, change.IsBinary(nil))
	assert.False(t, change.IsBinary([]byte("plain text\n")))
	assert.True(t, change.IsBinary([]byte("a\x00b")))

	late := append(bytes.Repeat([]byte("x"), 8<<10), 0)
	assert.False(t, change.IsBinary(late), "only the first 8 KiB count")
}

func TestStampEqual(t *testing.T) {
	now := time.Unix(1700000000, 5)
	s := change.Stamp{Size: 3, ModTime: now, Inode: 7}

	assert.True(t, s.Equal(change.Stamp{Size: 3, ModTime: now.In(time.FixedZone("x", 3600)), Inode: 7}), "the same instant elsewhere")
	assert.False(t, s.Equal(change.Stamp{Size: 4, ModTime: now, Inode: 7}))
	assert.False(t, s.Equal(change.Stamp{Size: 3, ModTime: now.Add(time.Nanosecond), Inode: 7}))
	assert.False(t, s.Equal(change.Stamp{Size: 3, ModTime: now, Inode: 8}))
}

func TestVisits(t *testing.T) {
	tests := []struct {
		name string
		c    change.Comparison
		ok   bool
	}{
		{"worktree", change.Comparison{Left: change.Commit("HEAD"), Right: change.Worktree()}, true},
		{"files on disk", change.Comparison{Left: change.File("/tmp/x_a.go"), Right: change.File("a.go")}, true},
		{"index", change.Comparison{Left: change.Commit("HEAD"), Right: change.Index()}, false},
		{"commits", change.Comparison{Left: change.Commit("a"), Right: change.Commit("b")}, false},
	}
	for _, tt := range tests {
		got, ok := tt.c.Visits()
		assert.Equal(t, tt.ok, ok, tt.name)
		if ok {
			assert.Equal(t, change.KindWorktree, got.Right.Kind(), tt.name)
		}
	}
}
