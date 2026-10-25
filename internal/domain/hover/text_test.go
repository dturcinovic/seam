package hover_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
)

func TestOnlyCodeInAKnownLanguageIsToColour(t *testing.T) {
	para := hover.Block{Kind: hover.Paragraph, Spans: []hover.Span{{Text: "a"}}}
	plain := hover.Block{Kind: hover.Code, Code: "x"}
	golang := hover.Block{Kind: hover.Code, Lang: lang.Go, Code: "x"}

	assert.False(t, hover.Content{}.HasCode())
	assert.False(t, hover.Content{Blocks: []hover.Block{para, plain, {Kind: hover.Rule}}}.HasCode())
	assert.True(t, hover.Content{Blocks: []hover.Block{para, golang}}.HasCode())
}
