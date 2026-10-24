package usecase_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/filediff"
)

// TestLongLinesGetCheckpoints loads a file whose second line is 10 KiB: an
// accent that straddles byte 4096 puts the first checkpoint after it, and a
// ZWJ sequence before byte 8192 is wider counted by code point.
func TestLongLinesGetCheckpoints(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	family := "👨\u200d👩\u200d👧" // 18 bytes, 6 columns by code point, 2 as a grapheme
	long := strings.Repeat("a", 4095) + "e\u0301" + family + strings.Repeat("b", 10<<10-4095-3-len(family))
	file := f.modified(t, "min.js", "short\n"+long+"\n", "short\n"+long+"x\n")

	u := loadOne(t, uc, worktreeComparison(t), 1, file, false)

	require.NoError(t, u.Err)
	want := []filediff.Checkpoint{
		{Byte: 4098, Col: 4096, GraphemeCol: 4096},
		{Byte: 8192, Col: 8192 - 2 - len(family) + 6, GraphemeCol: 8192 - 2 - len(family) + 2},
	}
	assert.Equal(t, want, u.Snapshot.Columns.Old.At(1))
	assert.Equal(t, want, u.Snapshot.Columns.New.At(1))
	assert.Nil(t, u.Snapshot.Columns.New.At(0), "a short line has none")
	assert.Len(t, u.Snapshot.Columns.New.Lines, 1)
}

// TestALineOfExactlyTheLimitHasNoCheckpoints: the line ending does not count.
func TestALineOfExactlyTheLimitHasNoCheckpoints(t *testing.T) {
	uc, f := newFileDiffs(t)
	start(t, uc)
	line := strings.Repeat("a", 4<<10)
	file := f.modified(t, "a.txt", line+"\r\n", line+"\n")

	u := loadOne(t, uc, worktreeComparison(t), 1, file, false)

	assert.Empty(t, u.Snapshot.Columns.Old.Lines)
	assert.Empty(t, u.Snapshot.Columns.New.Lines)
}
