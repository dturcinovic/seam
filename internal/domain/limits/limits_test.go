package limits_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/limits"
)

func TestNewFileSize(t *testing.T) {
	size, err := limits.NewFileSize(1024)
	require.NoError(t, err)
	assert.Equal(t, int64(1024), size.Bytes())

	for _, n := range []int64{0, -1} {
		_, err := limits.NewFileSize(n)
		assert.ErrorIs(t, err, limits.ErrInvalidFileSize, "n=%d", n)
	}
}

func TestParseFileSize(t *testing.T) {
	tests := []struct {
		in      string
		want    limits.FileSize
		wantErr bool
	}{
		{"1000", 1000, false},
		{"16K", 16 << 10, false},
		{"16M", 16 << 20, false},
		{"2G", 2 << 30, false},
		{"", 0, true},
		{"0", 0, true},
		{"-1M", 0, true},
		{"16MB", 0, true},
		{"16m", 0, true},
		{"M", 0, true},
		{"9999999999G", 0, true},
		{"-17179869183G", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := limits.ParseFileSize(tt.in)

			if tt.wantErr {
				require.ErrorIs(t, err, limits.ErrInvalidFileSize)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
