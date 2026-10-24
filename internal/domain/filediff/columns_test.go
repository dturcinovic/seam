package filediff_test

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"

	"seam/internal/domain/filediff"
)

func TestColumnIndexAt(t *testing.T) {
	three := []filediff.Checkpoint{{Byte: 4096, Col: 4096, GraphemeCol: 4096}}
	nine := []filediff.Checkpoint{{Byte: 4097, Col: 4000, GraphemeCol: 3990}}
	c := filediff.ColumnIndex{Lines: []filediff.LineCheckpoints{{Line: 3, Points: three}, {Line: 9, Points: nine}}}

	assert.Equal(t, three, c.At(3))
	assert.Equal(t, nine, c.At(9))
	for _, line := range []int{0, 4, 10} {
		assert.Nil(t, c.At(line), "line %d", line)
	}
	assert.Nil(t, filediff.ColumnIndex{}.At(0))
}

func TestColumnsApproxBytes(t *testing.T) {
	points := make([]filediff.Checkpoint, 2, 3)
	c := filediff.Columns{
		Old: filediff.ColumnIndex{Lines: []filediff.LineCheckpoints{{Line: 1, Points: points}}},
		New: filediff.ColumnIndex{Lines: make([]filediff.LineCheckpoints, 0, 2)},
	}

	want := 3*unsafe.Sizeof(filediff.LineCheckpoints{}) + 3*unsafe.Sizeof(filediff.Checkpoint{})
	assert.Equal(t, int64(want), c.ApproxBytes())
	assert.Zero(t, filediff.Columns{}.ApproxBytes())
}
