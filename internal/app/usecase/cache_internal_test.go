package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"seam/internal/domain/lang"
)

func TestAnLRUKeepsToItsBytes(t *testing.T) {
	s := newLRU[string, lang.Instance](16, 10)
	inst := lang.Instance{Server: "gopls"}

	assert.Empty(t, s.put("a.go", inst, 4))
	assert.Empty(t, s.put("b.go", inst, 4))
	assert.Equal(t, []string{"a.go"}, s.put("c.go", inst, 4), "the oldest goes")
	assert.Equal(t, []string{"b.go", "c.go"}, s.put("d.go", inst, 20), "the newest stays beyond the bound")
	assert.Equal(t, []string{"d.go"}, s.put("e.go", inst, 1))
	assert.Equal(t, int64(1), s.bytes)
}
