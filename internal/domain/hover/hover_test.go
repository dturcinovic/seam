package hover_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"seam/internal/domain/change"
	"seam/internal/domain/hover"
)

func TestADocumentAndAPositionInItHaveTheSameKey(t *testing.T) {
	id, err := change.ParseObjectID(strings.Repeat("a", 40))
	require.NoError(t, err)
	d := hover.Document{Commit: id, Path: "a.go", Version: 3, Text: "package a\n"}
	p := hover.Position{Req: 1, Commit: id, Path: "a.go", Version: 3, Line: 0, Byte: 8}

	assert.Equal(t, hover.DocKey{Commit: id, Path: "a.go"}, d.Key())
	assert.Equal(t, d.Key(), p.Key())
	assert.NotEqual(t, d.Key(), hover.Document{Path: "a.go"}.Key(), "the worktree's is another document")
}
