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

func TestParseDiffRejectsBadRawEntries(t *testing.T) {
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
		_, err := parseDiff([]byte(raw + "1\t1\ta\x00"))
		assert.Error(t, err, name)
	}
}

func TestParseDiffRejectsBadCounts(t *testing.T) {
	id := strings.Repeat("a", 40)
	modified := ":100644 100644 " + id + " " + id + " M\x00a\x00"
	renamed := ":100644 100644 " + id + " " + id + " R100\x00a\x00b\x00"
	tests := map[string]string{
		"no counts":         modified,
		"counts too few":    modified + "1\t1\x00",
		"another path":      modified + "1\t1\tb\x00",
		"not a number":      modified + "x\t1\ta\x00",
		"negative":          modified + "-1\t1\ta\x00",
		"beyond int":        modified + "18446744073709551615\t1\ta\x00",
		"one dash":          modified + "-\t1\ta\x00",
		"rename one path":   renamed + "1\t1\t\x00a\x00",
		"rename other path": renamed + "1\t1\t\x00a\x00c\x00",
		"left over":         modified + "1\t1\ta\x00extra\x00",
	}
	for name, raw := range tests {
		_, err := parseDiff([]byte(raw))
		assert.Error(t, err, name)
	}
}

func TestParseDiff(t *testing.T) {
	a, b, zero := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("0", 40)
	raw := ":000000 100644 " + zero + " " + b + " A\x00new\x00" +
		":100644 000000 " + a + " " + zero + " D\x00old\x00" +
		":100644 100644 " + a + " " + b + " R087\x00from\x00to\x00" +
		":120000 100644 " + a + " " + b + " T\x00link\x00" +
		":100644 000000 " + a + " " + zero + " U\x00conflict\x00" +
		"3\t0\tnew\x00" + "0\t7\told\x00" + "1\t2\t\x00from\x00to\x00" + "-\t-\tlink\x00" + "0\t0\tconflict\x00"

	files, err := parseDiff([]byte(raw))
	require.NoError(t, err)
	require.Len(t, files, 5)

	assert.Equal(t, change.ChangedFile{Status: change.Added, NewPath: "new", NewMode: 0o100644, NewID: mustID(t, b),
		Counts: change.LineCounts{Added: 3, Known: true}}, files[0])
	assert.Equal(t, change.ChangedFile{Status: change.Deleted, OldPath: "old", OldMode: 0o100644, OldID: mustID(t, a),
		Counts: change.LineCounts{Deleted: 7, Known: true}}, files[1])
	assert.Equal(t, change.LineCounts{Added: 1, Deleted: 2, Known: true}, files[2].Counts, "a rename names both paths")
	assert.False(t, files[3].Counts.Known, "git counts no lines of a binary file")
	assert.Equal(t, change.LineCounts{Known: true}, files[4].Counts)
	assert.Equal(t, "from", files[2].OldPath)
	assert.Equal(t, "to", files[2].NewPath)
	assert.Equal(t, change.Mode(0o120000), files[3].OldMode)
	assert.Equal(t, change.TypeChanged, files[3].Status)
	assert.Equal(t, change.Unmerged, files[4].Status, "a conflict stays a conflict")
	assert.Empty(t, files[4].NewPath, "its index side does not exist")
	assert.Equal(t, "conflict", files[4].Path())

	files, err = parseDiff(nil)
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
