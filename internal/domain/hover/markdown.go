package hover

import (
	"strings"

	"github.com/dturcinovic/seam/internal/domain/lang"
)

// Markdown reads the part of Markdown that hover answers and agents'
// replies use: fences, indented code, rules, ATX headings, paragraphs and
// list items, with references resolved across the whole text. A fence
// names its language as languages knows it.
func Markdown(md string, languages lang.Table) []Block {
	r := blockReader{defs: map[string]string{}, languages: languages, blank: true}
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		i = r.line(lines, i)
	}
	r.flush()
	in := inlineParser{defs: r.defs}
	for i, b := range r.blocks {
		if b.Kind == Paragraph || b.Kind == Heading {
			r.blocks[i].Spans = in.spans(r.raw[i])
		}
	}
	return r.blocks
}

// blockReader splits Markdown into blocks; the text of a paragraph or
// heading waits in raw until the references, which may come last, are
// known.
type blockReader struct {
	blocks    []Block
	raw       []string
	defs      map[string]string
	languages lang.Table
	// para holds the lines of the paragraph being read; afterList says the
	// last paragraph had a list item: the indented lines after a list
	// continue it rather than start code.
	para      []paraLine
	afterList bool
	// blank says the line before was blank or there was none.
	blank bool
}

// line reads lines[i] and returns the index of the last line it took.
func (r *blockReader) line(lines []string, i int) int {
	l := lines[i]
	indent, rest := indentOf(l)
	wasBlank := r.blank
	r.blank = false
	if indent < 4 && rest != "" {
		r.afterList = false
	}
	switch {
	case strings.TrimSpace(l) == "":
		r.flush()
		r.blank = true
	case indent < 4 && isFence(rest):
		r.flush()
		return r.fence(lines, i, indent, rest)
	case indent >= 4 && len(r.para) == 0 && wasBlank && !r.afterList:
		return r.indented(lines, i)
	case indent < 4 && isRule(rest):
		r.flush()
		r.add(Block{Kind: Rule}, "")
	case indent < 4 && isHeading(rest):
		r.flush()
		r.add(Block{Kind: Heading}, headingText(rest))
	case indent < 4 && len(r.para) == 0 && r.definition(rest):
	default:
		item, ok := listItem(rest)
		if !ok {
			item = rest
		}
		r.para = append(r.para, paraLine{item, ok})
	}
	return i
}

// paraLine is a line of a paragraph; item says it starts a list item.
type paraLine struct {
	text string
	item bool
}

func (r *blockReader) add(b Block, raw string) {
	r.raw = append(r.raw, raw)
	r.blocks = append(r.blocks, b)
}

// flush ends the paragraph being read. Its lines flow into each other but
// where one ends in a hard break or the next is a list item.
func (r *blockReader) flush() {
	if len(r.para) == 0 {
		return
	}
	var b strings.Builder
	hard, list := false, false
	for i, pl := range r.para {
		l := pl.text
		list = list || pl.item
		switch {
		case i == 0:
		case pl.item || hard:
			b.WriteByte('\n')
		default:
			b.WriteByte(' ')
		}
		hard = strings.HasSuffix(l, "  ") || strings.HasSuffix(l, "\\")
		l = strings.TrimRight(l, " ")
		if hard {
			l = strings.TrimSuffix(l, "\\")
		}
		b.WriteString(l)
	}
	r.add(Block{Kind: Paragraph}, b.String())
	r.afterList = list
	r.para = nil
}

// fence reads a fenced code block from its opening line i up to the
// closing fence, or to the end if there is none.
func (r *blockReader) fence(lines []string, i, indent int, open string) int {
	mark := open[:len(open)-len(strings.TrimLeft(open, open[:1]))]
	info, _, _ := strings.Cut(strings.TrimSpace(open[len(mark):]), " ")
	var code []string
	j := i + 1
	for ; j < len(lines); j++ {
		in, rest := indentOf(lines[j])
		if in < 4 && strings.HasPrefix(rest, mark) && strings.TrimSpace(strings.TrimLeft(rest, mark[:1])) == "" {
			break
		}
		code = append(code, dropIndent(lines[j], indent))
	}
	r.add(Block{Kind: Code, Lang: r.languages.FromFence(info), Code: strings.Join(code, "\n")}, "")
	r.blank = true
	return j
}

// indented reads an indented code block from line i: the indented and
// blank lines after it, less one level of indent and the blank lines at
// its end.
func (r *blockReader) indented(lines []string, i int) int {
	var code []string
	j := i
	for ; j < len(lines); j++ {
		in, _ := indentOf(lines[j])
		if in < 4 && strings.TrimSpace(lines[j]) != "" {
			break
		}
		code = append(code, dropIndent(lines[j], 4))
	}
	for len(code) > 0 && strings.TrimSpace(code[len(code)-1]) == "" {
		code = code[:len(code)-1]
	}
	r.add(Block{Kind: Code, Code: strings.Join(code, "\n")}, "")
	r.blank = true
	return j - 1
}

// definition takes a link reference definition, `[label]: url`, and says
// whether line was one.
func (r *blockReader) definition(line string) bool {
	if !strings.HasPrefix(line, "[") {
		return false
	}
	label, rest, ok := strings.Cut(line[1:], "]:")
	if !ok || label == "" || strings.Contains(label, "]") {
		return false
	}
	url, _, _ := strings.Cut(strings.TrimSpace(rest), " ")
	if url == "" {
		return false
	}
	if key := refKey(label); r.defs[key] == "" {
		r.defs[key] = strings.Trim(url, "<>")
	}
	return true
}

// indentOf is the columns of blank before the text of l, a tab counting to
// the next multiple of four, and the text.
func indentOf(l string) (int, string) {
	cols := 0
	for i := range len(l) {
		switch l[i] {
		case ' ':
			cols++
		case '\t':
			cols += 4 - cols%4
		default:
			return cols, l[i:]
		}
	}
	return cols, ""
}

// dropIndent removes up to cols columns of blank from the start of l.
func dropIndent(l string, cols int) string {
	at := 0
	for i := range len(l) {
		if at >= cols {
			return l[i:]
		}
		switch l[i] {
		case ' ':
			at++
		case '\t':
			at += 4 - at%4
		default:
			return l[i:]
		}
	}
	return ""
}

// isFence says l opens a fence. A backtick fence holds no backtick after
// its run, so a line starting with a code span is text.
func isFence(l string) bool {
	if strings.HasPrefix(l, "~~~") {
		return true
	}
	return strings.HasPrefix(l, "```") && !strings.Contains(strings.TrimLeft(l, "`"), "`")
}

// isRule says l is three or more of one of -, * or _, blanks between.
func isRule(l string) bool {
	if l == "" || !strings.ContainsAny(l[:1], "-*_") {
		return false
	}
	l = strings.ReplaceAll(strings.TrimSpace(l), " ", "")
	if len(l) < 3 || !strings.ContainsAny(l[:1], "-*_") {
		return false
	}
	return strings.Trim(l, l[:1]) == ""
}

// headingText is the text of heading l, less the run of # that closes it
// after a blank, as in "# A #", but not the one of "# C#".
func headingText(l string) string {
	text := strings.TrimSpace(strings.TrimLeft(l, "#"))
	if closed := strings.TrimRight(text, "#"); closed == "" || strings.HasSuffix(closed, " ") || strings.HasSuffix(closed, "\t") {
		return strings.TrimSpace(closed)
	}
	return text
}

func isHeading(l string) bool {
	n := len(l) - len(strings.TrimLeft(l, "#"))
	return n >= 1 && n <= 6 && (len(l) == n || l[n] == ' ' || l[n] == '\t')
}

// listItem is the text of a list item with its bullet shown as one, and
// whether l starts one.
func listItem(l string) (string, bool) {
	if len(l) >= 2 && strings.ContainsAny(l[:1], "-*+") && (l[1] == ' ' || l[1] == '\t') {
		return "• " + strings.TrimLeft(l[2:], " \t"), true
	}
	digits := len(l) - len(strings.TrimLeft(l, "0123456789"))
	if digits >= 1 && digits <= 9 && len(l) > digits+1 && strings.ContainsAny(l[digits:digits+1], ".)") && l[digits+1] == ' ' {
		return l, true
	}
	return "", false
}

// refKey is a reference label as it is matched: case and runs of blank do
// not count.
func refKey(label string) string {
	return strings.ToLower(strings.Join(strings.Fields(label), " "))
}

// Tidy drops the blocks that show nothing, with the rules at either end
// and all but one of rules in a row; code loses its blank lines at either
// end.
func Tidy(blocks []Block) []Block {
	var kept []Block
	for _, b := range blocks {
		switch b.Kind {
		case Code:
			b.Code = strings.Trim(b.Code, "\n")
			if strings.TrimSpace(b.Code) == "" {
				continue
			}
		case Paragraph, Heading:
			if blank(b.Spans) {
				continue
			}
		case Rule:
			if len(kept) == 0 || kept[len(kept)-1].Kind == Rule {
				continue
			}
		}
		kept = append(kept, b)
	}
	if n := len(kept); n > 0 && kept[n-1].Kind == Rule {
		kept = kept[:n-1]
	}
	return kept
}

func blank(spans []Span) bool {
	for _, s := range spans {
		if strings.TrimSpace(s.Text) != "" {
			return false
		}
	}
	return true
}
