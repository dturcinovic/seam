package git

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/change"
	"seam/internal/domain/limits"
	"seam/internal/testsupport"
)

// A /proc file reports size 0 to stat and has content all the same: the
// same as a file that grows after Stat, and deterministic.
func TestFileThatGrowsAfterStatIsCaughtByTheRead(t *testing.T) {
	root, err := os.OpenRoot("/proc/self")
	require.NoError(t, err)
	defer func() { _ = root.Close() }()
	info, err := root.Lstat("status")
	require.NoError(t, err)
	require.Zero(t, info.Size())

	max, err := limits.NewFileSize(10)
	require.NoError(t, err)
	_, _, err = readLimited(root, "status", max)

	tooLarge, ok := errors.AsType[*change.TooLargeError](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, int64(11), tooLarge.Size, "reading stops one byte past the limit")
}

func TestCloseEndsCatFile(t *testing.T) {
	git := testsupport.NewGitRepo(t)
	git.Write("f", "content\n")
	id, err := change.ParseObjectID(strings.TrimSpace(git.Git("hash-object", "-w", "f")))
	require.NoError(t, err)
	repo, err := Open(git.Dir, nil)
	require.NoError(t, err)
	_, err = repo.ReadBlob(t.Context(), id, limits.DefaultFileSize)
	require.NoError(t, err)
	cat := repo.blobs

	require.NoError(t, repo.Close(t.Context()))

	assert.NotNil(t, cat.cmd.ProcessState, "the process has been waited for")
	_, err = repo.ReadBlob(t.Context(), id, limits.DefaultFileSize)
	assert.ErrorIs(t, err, ErrClosed, "and no read starts another")
	assert.Nil(t, repo.blobs)
}

func TestParseRawRejectsGarbage(t *testing.T) {
	id := strings.Repeat("a", 40)
	tests := map[string]string{
		"no colon":        "100644 100644 " + id + " " + id + " M\x00a\x00",
		"short header":    ":100644 " + id + " M\x00a\x00",
		"unknown status":  ":100644 100644 " + id + " " + id + " X\x00a\x00",
		"missing path":    ":100644 100644 " + id + " " + id + " M\x00",
		"rename one path": ":100644 100644 " + id + " " + id + " R100\x00a\x00",
		"bad mode":        ":10064x 100644 " + id + " " + id + " M\x00a\x00",
		"bad id":          ":100644 100644 xyz " + id + " M\x00a\x00",
	}
	for name, raw := range tests {
		_, err := parseRaw([]byte(raw))
		assert.Error(t, err, name)
	}
}

func TestParseRaw(t *testing.T) {
	a, b, zero := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("0", 40)
	raw := ":000000 100644 " + zero + " " + b + " A\x00new\x00" +
		":100644 000000 " + a + " " + zero + " D\x00old\x00" +
		":100644 100644 " + a + " " + b + " R087\x00from\x00to\x00" +
		":120000 100644 " + a + " " + b + " T\x00link\x00" +
		":100644 000000 " + a + " " + zero + " U\x00conflict\x00"

	files, err := parseRaw([]byte(raw))
	require.NoError(t, err)
	require.Len(t, files, 5)

	assert.Equal(t, change.ChangedFile{Status: change.Added, NewPath: "new", NewMode: 0o100644, NewID: mustID(t, b)}, files[0])
	assert.Equal(t, change.ChangedFile{Status: change.Deleted, OldPath: "old", OldMode: 0o100644, OldID: mustID(t, a)}, files[1])
	assert.Equal(t, "from", files[2].OldPath)
	assert.Equal(t, "to", files[2].NewPath)
	assert.Equal(t, change.Mode(0o120000), files[3].OldMode)
	assert.Equal(t, change.TypeChanged, files[3].Status)
	assert.Equal(t, change.Unmerged, files[4].Status, "a conflict stays a conflict")
	assert.Empty(t, files[4].NewPath, "its index side does not exist")
	assert.Equal(t, "conflict", files[4].Path())

	files, err = parseRaw(nil)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func mustID(t *testing.T, s string) change.ObjectID {
	t.Helper()
	id, err := change.ParseObjectID(s)
	require.NoError(t, err)
	return id
}

func TestReasonSkipsWarningsAndHints(t *testing.T) {
	assert.Equal(t, "bad revision", reason("warning: refname is ambiguous.\nhint: see docs\nfatal: bad revision"))
	assert.Equal(t, "cannot lock", reason("error: cannot lock"))
	assert.Equal(t, "something else", reason("something else\nmore"))
}
