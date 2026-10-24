package chroma

import (
	"context"
	"errors"
	"fmt"
	"strings"

	chromalib "github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// delegation lexes a file of two languages the way chroma's delegating
// lexers do: an inner language where its own lexer finds it, the root
// language around it, lexed as one text without the inner parts. chroma's
// own collects every token of both before it yields the first, which no
// budget could stop: 23 s past the budget for 8 MiB of PHP. This one checks
// ctx while it lexes the inner language and streams the root's tokens.
type delegation struct {
	language, root chromalib.Lexer
}

// outside is the state of the text around PHP. chroma's PHTML renames
// PHP's root instead and takes "root" for the outside, so PHP's own rules
// that lex an interpolation such as "{$a}" from its root would lex it as
// HTML; this keeps PHP's root as it is.
const outside = "outside"

// phpInHTML is PHP within HTML, built as chroma builds its PHTML lexer but
// for the state of the text around it.
func phpInHTML() (*delegation, error) {
	php, ok := lexers.Get("PHP").(*chromalib.RegexLexer)
	html := lexers.Get("HTML")
	if !ok || html == nil {
		return nil, errors.New("chroma: no PHP or HTML lexer")
	}
	rules, err := php.Rules()
	if err != nil {
		return nil, fmt.Errorf("chroma: PHP in HTML: %w", err)
	}
	rules = rules.Merge(chromalib.Rules{outside: {
		{Pattern: `<\?(php)?`, Type: chromalib.CommentPreproc, Mutator: chromalib.Push("root")},
		{Pattern: `[^<]+`, Type: chromalib.Other},
		{Pattern: `<`, Type: chromalib.Other},
	}})
	language, err := chromalib.NewLexer(&chromalib.Config{Name: "PHP", DotAll: true, CaseInsensitive: true, EnsureNL: true},
		func() chromalib.Rules { return rules })
	if err != nil {
		return nil, fmt.Errorf("chroma: PHP in HTML: %w", err)
	}
	return &delegation{language: language, root: html}, nil
}

// insertion is a stretch of the inner language at [start, end) of the
// text, with its tokens.
type insertion struct {
	start, end int
	tokens     []chromalib.Token
}

// tokenise lexes the inner language first, keeping its tokens and the text
// around them. If ctx ends meanwhile, it yields nothing: without the root's
// tokens no line is complete.
func (d *delegation) tokenise(ctx context.Context, options *chromalib.TokeniseOptions, text string) (chromalib.Iterator, error) {
	inner := *options
	inner.State = outside
	it, err := d.language.Tokenise(&inner, text)
	if err != nil {
		return nil, err
	}
	var (
		others     strings.Builder
		insertions []insertion
		offset     int
		inOther    = true
	)
	for t := it(); t != chromalib.EOF; t = it() {
		if ctx.Err() != nil {
			return chromalib.Literator(), nil
		}
		if t.Type == chromalib.Other {
			others.WriteString(t.Value)
			inOther = true
		} else {
			if inOther {
				insertions = append(insertions, insertion{start: offset})
				inOther = false
			}
			in := &insertions[len(insertions)-1]
			in.tokens = append(in.tokens, t)
			in.end = offset + len(t.Value)
		}
		offset += len(t.Value)
	}
	// Without the inner language the root lexes the text itself: the text
	// around the inner parts carries the newline the inner lexer appends.
	if len(insertions) == 0 {
		return d.root.Tokenise(options, text)
	}
	root, err := d.root.Tokenise(options, others.String())
	if err != nil {
		return nil, err
	}
	return interleave(root, insertions), nil
}

// interleave yields the root's tokens with the insertions put back at their
// offsets, splitting a root token an insertion falls into. It yields an
// insertion's tokens from the insertion itself and lets go of them after:
// a file's PHP may be most of it.
func interleave(root chromalib.Iterator, insertions []insertion) chromalib.Iterator {
	var pending []chromalib.Token
	t, offset := root(), 0
	return func() chromalib.Token {
		if len(pending) > 0 {
			next := pending[0]
			pending = pending[1:]
			return next
		}
		switch {
		case t == chromalib.EOF && len(insertions) == 0:
			return chromalib.EOF
		case t == chromalib.EOF || (len(insertions) > 0 && insertions[0].start < offset+len(t.Value)):
			in := insertions[0]
			insertions[0].tokens = nil
			insertions = insertions[1:]
			var before chromalib.Token
			before, t = split(t, in.start-offset)
			offset += len(before.Value) + in.end - in.start
			if t == chromalib.EOF {
				t = root()
			}
			// An insertion has a token at least.
			if before == chromalib.EOF {
				before, in.tokens = in.tokens[0], in.tokens[1:]
			}
			pending = in.tokens
			return before
		}
		next := t
		offset += len(t.Value)
		t = root()
		return next
	}
}

// split cuts t at byte at; either half is EOF if empty.
func split(t chromalib.Token, at int) (before, after chromalib.Token) {
	switch {
	case t == chromalib.EOF:
		return chromalib.EOF, chromalib.EOF
	case at <= 0:
		return chromalib.EOF, t
	case at >= len(t.Value):
		return t, chromalib.EOF
	}
	before, after = t, t
	before.Value, after.Value = t.Value[:at], t.Value[at:]
	return before, after
}
