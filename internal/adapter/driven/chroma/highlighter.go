package chroma

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	chromalib "github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"go.uber.org/zap"

	"seam/internal/domain/diff"
	"seam/internal/domain/lang"
	"seam/internal/domain/syntax"
)

// Highlighter implements out.Highlighter with chroma. It keeps no state
// between calls.
type Highlighter struct {
	log *zap.Logger
	// own are the languages chroma's registry would lex wrongly or
	// without bound; the rest it finds by name.
	own map[lang.Lang]lexer
}

// lexer is how a language is lexed: ctx bounds what tokenise does before
// it yields the first token.
type lexer struct {
	name     string
	tokenise func(ctx context.Context, options *chromalib.TokeniseOptions, text string) (chromalib.Iterator, error)
}

// New returns a highlighter that logs to log why it stopped short.
func New(log *zap.Logger) (*Highlighter, error) {
	// PHP files are HTML with PHP in it; chroma's PHP lexer would read the
	// HTML around <?php as PHP.
	php, err := phpInHTML()
	if err != nil {
		return nil, err
	}
	return &Highlighter{log: log, own: map[lang.Lang]lexer{lang.PHP: {name: "PHTML", tokenise: php.tokenise}}}, nil
}

// Highlight finds the tokens of text in l; see out.Highlighter.
func (h *Highlighter) Highlight(ctx context.Context, l lang.Lang, text string) (syntax.Tokens, bool) {
	lx, ok := h.lexerFor(l)
	if !ok {
		return syntax.Tokens{}, false
	}
	w := walker{text: text, valid: utf8.ValidString(text)}
	if err := w.run(ctx, lx); err != nil {
		h.log.Warn("syntax colours stop early", zap.String("lexer", lx.name),
			zap.Int("line", w.b.Lines()+1), zap.Error(err))
	}
	return w.b.Tokens(), true
}

// lexerFor finds the lexer by the language's name, which chroma knows as an
// alias.
func (h *Highlighter) lexerFor(l lang.Lang) (lexer, bool) {
	if lx, ok := h.own[l]; ok {
		return lx, true
	}
	found := lexers.Get(string(l))
	if l == "" || found == nil {
		return lexer{}, false
	}
	return lexer{
		name: found.Config().Name,
		tokenise: func(_ context.Context, options *chromalib.TokeniseOptions, text string) (chromalib.Iterator, error) {
			return found.Tokenise(options, text)
		},
	}, true
}

// walker lays chroma's tokens over the lines of text. Tokens carry text,
// not offsets, so it advances through text by each token's length: bytes
// for valid UTF-8, else runes, as chroma lexes []rune(text), in which an
// invalid byte became one U+FFFD.
type walker struct {
	text  string
	valid bool
	b     syntax.Builder
	// at is the offset in text the next token starts at, line where the
	// current line starts.
	at, line int
}

// run walks the tokens until text or ctx ends. It returns why it stopped
// before the end, but not for ctx, which the caller knows of.
func (w *walker) run(ctx context.Context, lx lexer) (err error) {
	// chroma's iterators report their errors as panics.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("lexer panicked: %v", r)
		}
	}()
	// EnsureLF off: turning CRLF into LF would shift every offset after.
	it, err := lx.tokenise(ctx, &chromalib.TokeniseOptions{State: "root"}, w.text)
	if err != nil {
		return err
	}
	// Done is fetched once: Err on a context with a deadline locks it,
	// for each of millions of tokens.
	done := ctx.Done()
	for tok := it(); tok != chromalib.EOF; tok = it() {
		select {
		case <-done:
			return nil
		default:
		}
		if err := w.add(tok.Value, kindOf(tok.Type)); err != nil {
			return err
		}
	}
	// The last line may have no newline to end it. A lexer that ended
	// before the text did leaves it uncovered.
	if w.at == len(w.text) && w.at > w.line {
		w.endLine()
	}
	return nil
}

// add lays one token over the text, line by line. Past the end of text is
// only the newline some lexers append.
func (w *walker) add(value string, k syntax.Kind) error {
	for value != "" && w.at < len(w.text) {
		part := value
		if nl := strings.IndexByte(value, '\n'); nl >= 0 {
			part = value[:nl+1]
		}
		// Offsets come from lengths alone; a lexer that drops or changes
		// text, as chroma's Ignore tokens do, would shift every run after.
		if w.valid && !strings.HasPrefix(w.text[w.at:], part) && w.at+len(part) <= len(w.text) {
			return fmt.Errorf("token %q does not match the text at %d", part, w.at)
		}
		value = value[len(part):]
		w.at = min(w.at+w.lengthOf(part), len(w.text))
		if err := w.b.Add(w.at-w.line, k); err != nil {
			return err
		}
		if strings.HasSuffix(part, "\n") {
			w.endLine()
		}
	}
	return nil
}

// lengthOf is how many bytes of text, from at, part stands for.
func (w *walker) lengthOf(part string) int {
	if w.valid {
		return len(part)
	}
	n := 0
	for range utf8.RuneCountInString(part) {
		_, size := utf8.DecodeRuneInString(w.text[w.at+n:])
		n += size
	}
	return n
}

func (w *walker) endLine() {
	w.b.EndLine(len(diff.Content(w.text[w.line:w.at])))
	w.line = w.at
}

// kindOf maps chroma's token types onto the kinds. The switch has no tag,
// as chroma's types are too many for a case each.
func kindOf(t chromalib.TokenType) syntax.Kind {
	switch {
	case t == chromalib.KeywordType, t == chromalib.NameClass, t == chromalib.NameException:
		return syntax.Type
	case t == chromalib.KeywordConstant, t == chromalib.NameConstant:
		return syntax.Constant
	case t.InCategory(chromalib.Keyword):
		return syntax.Keyword
	case t.InSubCategory(chromalib.NameFunction):
		return syntax.Function
	case t.InSubCategory(chromalib.NameBuiltin):
		return syntax.Builtin
	case t.InSubCategory(chromalib.NameVariable):
		return syntax.Variable
	case t == chromalib.NameTag:
		return syntax.Tag
	case t == chromalib.NameAttribute:
		return syntax.Attribute
	case t == chromalib.NameDecorator, t.InSubCategory(chromalib.CommentPreproc):
		return syntax.Meta
	case t == chromalib.LiteralStringEscape, t == chromalib.NameEntity:
		return syntax.Escape
	case t.InSubCategory(chromalib.LiteralString):
		return syntax.String
	case t.InSubCategory(chromalib.LiteralNumber):
		return syntax.Number
	case t.InCategory(chromalib.Comment):
		return syntax.Comment
	case t.InCategory(chromalib.Operator):
		return syntax.Operator
	case t.InCategory(chromalib.Punctuation):
		return syntax.Punctuation
	}
	return syntax.Plain
}
