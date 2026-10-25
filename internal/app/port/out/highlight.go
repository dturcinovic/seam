package out

import (
	"context"

	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/syntax"
)

//counterfeiter:generate . Highlighter

// Highlighter finds the tokens of text in language l, text that holds no
// line over syntax.MaxLine. It covers the lines from the first up to where
// it stops: when ctx ends, which it checks between tokens, or when the
// lexer fails. ok is false if it has no lexer for l.
type Highlighter interface {
	Highlight(ctx context.Context, l lang.Lang, text string) (t syntax.Tokens, ok bool)
}
