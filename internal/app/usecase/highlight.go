package usecase

import (
	"context"

	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/syntax"
)

// highlightSide finds the tokens of one side's text in l, of lines lines, and
// notes in s where its colours stop short; late says the budget ran out.
// Each side has its own budget, so the new side, the one worked on, never
// goes without for the old one's.
func (f *FileDiffs) highlightSide(ctx context.Context, s *filediff.Snapshot, side filediff.Which, l lang.Lang, text string, lines int) (t syntax.Tokens, late bool, err error) {
	if l == "" || lines == 0 {
		return syntax.Tokens{}, false, nil
	}
	budget, cancel := context.WithTimeout(ctx, highlightBudget)
	defer cancel()
	t, ok := f.highlighter.Highlight(budget, l, syntax.Lexable(text))
	// As with the diff, the budget ending leaves the rest plain; the job
	// being cancelled leaves nothing.
	if err := ctx.Err(); err != nil {
		return syntax.Tokens{}, false, err
	}
	if ok && t.Lines() < lines {
		s.Notices = append(s.Notices, filediff.Notice{Kind: filediff.HighlightStopped, Side: side, Line: t.Lines() + 1})
	}
	return t, budget.Err() != nil, nil
}
