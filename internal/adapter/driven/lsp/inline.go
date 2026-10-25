package lsp

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dturcinovic/seam/internal/domain/hover"
)

// linkReach bounds how far the end of a link's text or target, or of a
// reference label or autolink, may be from its start.
const linkReach = 4 << 10

// inlineParser turns the text of a paragraph into spans: escapes, code,
// emphasis, links and images, as hover answers use them.
type inlineParser struct {
	defs map[string]string
}

func (p inlineParser) spans(text string) []hover.Span {
	var w spanWriter
	p.parse(&w, text, hover.Plain)
	return w.done()
}

// spanWriter collects spans, merging neighbours of one kind; the last one
// grows in a builder, so many small pieces cost no copies of it.
type spanWriter struct {
	spans []hover.Span
	last  strings.Builder
	kind  hover.SpanKind
}

func (w *spanWriter) add(k hover.SpanKind, s string) {
	if s == "" {
		return
	}
	if k != w.kind {
		w.end()
		w.kind = k
	}
	w.last.WriteString(s)
}

func (w *spanWriter) end() {
	if w.last.Len() > 0 {
		w.spans = append(w.spans, hover.Span{Kind: w.kind, Text: w.last.String()})
		w.last.Reset()
	}
}

func (w *spanWriter) done() []hover.Span {
	w.end()
	return w.spans
}

// unclosed remembers the delimiters found without a closer from some
// point on: a later one of the same kind has none either, so each is
// looked for once.
type unclosed map[string]bool

// inline is the text of one parse with what is found out about it once:
// the delimiters without closer and where each bracket closes.
type inline struct {
	s       string
	none    unclosed
	closers []int
}

// parse writes the spans of s in kind k, or in the kind of a construct
// within it where that comes later in hover.SpanKind.
func (p inlineParser) parse(w *spanWriter, s string, k hover.SpanKind) {
	in := inline{s: s, none: unclosed{}, closers: closers(s)}
	for i := 0; i < len(s); {
		if end, ok := p.construct(w, in, i, k); ok {
			i = end
			continue
		}
		if s[i] == '\\' && i+1 < len(s) && isASCIIPunct(s[i+1]) {
			w.add(k, s[i+1:i+2])
			i += 2
			continue
		}
		w.add(k, s[i:i+1])
		i++
	}
}

// construct writes the code span, link, image, autolink or emphasis that
// starts at s[i], if one does, and returns where it ends.
func (p inlineParser) construct(w *spanWriter, in inline, i int, k hover.SpanKind) (int, bool) {
	s := in.s
	switch s[i] {
	case '`':
		code, end, ok := codeSpan(s, i, in.none)
		if ok {
			w.add(hover.InlineCode, code)
		}
		return end, ok
	case '!':
		if i+1 < len(s) && s[i+1] == '[' {
			// An image shows its text only; its target is often a data URL.
			if text, _, end, ok := p.link(in, i+1); ok {
				p.parse(w, text, k)
				return end, true
			}
		}
	case '[':
		if text, url, end, ok := p.link(in, i); ok {
			p.writeLink(w, text, url, k)
			return end, true
		}
	case '<':
		if url, end, ok := autolink(s, i); ok {
			w.add(max(k, hover.URL), url)
			return end, true
		}
	case '*', '_':
		if inner, kind, end, ok := emphasis(s, i, in.none); ok {
			p.parse(w, inner, max(k, kind))
			return end, true
		}
	}
	return i, false
}

// writeLink writes a link's text and, dimmed after it, its target if that
// is somewhere to go and says more than the text.
func (p inlineParser) writeLink(w *spanWriter, text, url string, k hover.SpanKind) {
	shown := strings.HasPrefix(url, "http:") || strings.HasPrefix(url, "https:") || strings.HasPrefix(url, "file:")
	if shown && text == url {
		w.add(max(k, hover.URL), url)
		return
	}
	p.parse(w, text, max(k, hover.Link))
	if shown {
		w.add(k, " ")
		w.add(max(k, hover.URL), url)
	}
}

// link reads the link whose text starts with the bracket at s[i]: inline,
// a full or collapsed reference, or a shortcut one with a definition.
func (p inlineParser) link(in inline, i int) (text, url string, end int, ok bool) {
	s := in.s
	rb := in.closer(i)
	if rb < 0 {
		return "", "", 0, false
	}
	text, end = s[i+1:rb], rb+1
	if end < len(s) && s[end] == '(' {
		if rp := in.closer(end); rp >= 0 {
			return text, destination(s[end+1 : rp]), rp + 1, true
		}
	}
	label := text
	if end+1 < len(s) && s[end] == '[' {
		if rb := strings.IndexByte(s[end+1:min(len(s), end+1+linkReach)], ']'); rb >= 0 {
			if l := s[end+1 : end+1+rb]; l != "" {
				label = l
			}
			end += rb + 2
		}
	}
	url, ok = p.defs[refKey(label)]
	return text, url, end, ok
}

// destination is a link's target without its title.
func destination(d string) string {
	d = strings.TrimSpace(d)
	if strings.HasPrefix(d, "<") {
		if gt := strings.IndexByte(d, '>'); gt > 0 {
			return d[1:gt]
		}
	}
	url, _, _ := strings.Cut(d, " ")
	return url
}

// closer is where the bracket or parenthesis at i closes, within
// linkReach; -1 for none.
func (in inline) closer(i int) int {
	if c := in.closers[i]; c >= 0 && c-i <= linkReach {
		return c
	}
	return -1
}

// closers holds, for each [ and ( of s, where it closes, nested ones and
// escaped ones not counting; -1 elsewhere. One pass finds all, so no
// bracket is looked for again.
func closers(s string) []int {
	at := make([]int, len(s))
	var brackets, parens []int
	pop := func(stack *[]int, j int) {
		if n := len(*stack); n > 0 {
			at[(*stack)[n-1]] = j
			*stack = (*stack)[:n-1]
		}
	}
	for j := 0; j < len(s); j++ {
		at[j] = -1
		switch s[j] {
		case '\\':
			if j+1 < len(s) {
				j++
				at[j] = -1
			}
		case '[':
			brackets = append(brackets, j)
		case '(':
			parens = append(parens, j)
		case ']':
			pop(&brackets, j)
		case ')':
			pop(&parens, j)
		}
	}
	return at
}

// codeSpan reads the code span whose backticks start at s[i]: up to a run
// of as many, with line breaks as blanks and one blank at both ends
// dropped. Without such a run the backticks are text, ending at end.
func codeSpan(s string, i int, none unclosed) (code string, end int, ok bool) {
	n := run(s, i)
	key := s[i : i+n]
	if none[key] {
		return "", i + n, false
	}
	for j := i + n; j < len(s); {
		at := strings.IndexByte(s[j:], '`')
		if at < 0 {
			break
		}
		j += at
		if m := run(s, j); m != n {
			j += m
			continue
		}
		code = strings.ReplaceAll(s[i+n:j], "\n", " ")
		if len(code) >= 2 && code[0] == ' ' && code[len(code)-1] == ' ' && strings.Trim(code, " ") != "" {
			code = code[1 : len(code)-1]
		}
		return code, j + n, true
	}
	none[key] = true
	return "", i + n, false
}

// emphasis reads the emphasis whose delimiters start at s[i], one for
// Emphasis and two or three for Strong, simplified from CommonMark: an
// opener is followed by a non-blank, a closer preceded by one, _ neither
// opens nor closes within a word, and a code span hides its delimiters.
func emphasis(s string, i int, none unclosed) (inner string, kind hover.SpanKind, end int, ok bool) {
	n := run(s, i)
	key := s[i : i+n]
	if n > 3 || none[key] || !opens(s, i, n) {
		return "", 0, 0, false
	}
	kind = hover.Emphasis
	if n > 1 {
		kind = hover.Strong
	}
	for j := i + n; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '`':
			if _, end, ok := codeSpan(s, j, none); ok {
				j = end - 1
			}
		case s[i]:
			m := run(s, j)
			if m == n && closes(s, j, n) {
				return s[i+n : j], kind, j + n, true
			}
			j += m - 1
		}
	}
	none[key] = true
	return "", 0, 0, false
}

func opens(s string, i, n int) bool {
	after, _ := utf8.DecodeRuneInString(s[i+n:])
	if i+n == len(s) || unicode.IsSpace(after) {
		return false
	}
	if s[i] == '_' && i > 0 {
		before, _ := utf8.DecodeLastRuneInString(s[:i])
		return !isWordRune(before)
	}
	return true
}

func closes(s string, j, n int) bool {
	before, _ := utf8.DecodeLastRuneInString(s[:j])
	if j == 0 || unicode.IsSpace(before) {
		return false
	}
	if s[j] == '_' && j+n < len(s) {
		after, _ := utf8.DecodeRuneInString(s[j+n:])
		return !isWordRune(after)
	}
	return true
}

// autolink reads <scheme:target> at s[i].
func autolink(s string, i int) (url string, end int, ok bool) {
	gt := strings.IndexByte(s[i:min(len(s), i+linkReach)], '>')
	if gt < 0 {
		return "", 0, false
	}
	url = s[i+1 : i+gt]
	scheme, _, found := strings.Cut(url, ":")
	if !found || len(scheme) < 2 || strings.ContainsAny(url, " \t\n<") || strings.Trim(scheme, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+.-") != "" {
		return "", 0, false
	}
	return url, i + gt + 1, true
}

// run is the length of the run of s[i] starting there.
func run(s string, i int) int {
	n := 1
	for i+n < len(s) && s[i+n] == s[i] {
		n++
	}
	return n
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func isASCIIPunct(b byte) bool {
	return b < utf8.RuneSelf && unicode.IsPunct(rune(b)) || strings.IndexByte("$+<=>^`|~", b) >= 0
}
