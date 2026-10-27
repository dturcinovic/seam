package lsp

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/hover"
	"github.com/dturcinovic/seam/internal/domain/lang"
)

// part is one piece of a hover's contents as the server sent it: Markdown,
// plain text, or code with the name of its language.
type part struct {
	format string
	fence  string
	text   string
}

const (
	markdownPart  = "markdown"
	plaintextPart = "plaintext"
	codePart      = "code"
)

// hoverContent is the content of a hover result, without blocks for none.
// The result is null, or holds a MarkupContent, a MarkedString, or a list
// of them; a fence names its language as languages knows it.
func hoverContent(result jsontext.Value, languages lang.Table) (hover.Content, error) {
	if len(result) == 0 || string(result) == "null" {
		return hover.Content{}, nil
	}
	var h struct {
		Contents jsontext.Value `json:"contents"`
	}
	if err := json.Unmarshal(result, &h); err != nil {
		return hover.Content{}, fmt.Errorf("hover result: %w", err)
	}
	parts, err := contents(h.Contents)
	if err != nil {
		return hover.Content{}, fmt.Errorf("hover contents: %w", err)
	}
	var c hover.Content
	parts, c.Cut = cutParts(parts, hover.MaxText)
	for _, p := range parts {
		c.Blocks = append(c.Blocks, p.blocks(languages)...)
	}
	c.Blocks = hover.Tidy(c.Blocks)
	return c, nil
}

func contents(v jsontext.Value) ([]part, error) {
	// An if chain rather than a switch: the other kinds hold no text, and
	// listing all of them for exhaustive would say nothing.
	if v.Kind() == '"' {
		var s string
		err := json.Unmarshal(v, &s)
		return []part{newPart(markdownPart, "", s)}, err
	}
	if v.Kind() == '[' {
		var items []jsontext.Value
		if err := json.Unmarshal(v, &items); err != nil {
			return nil, err
		}
		var parts []part
		for _, item := range items {
			p, err := contents(item)
			if err != nil {
				return nil, err
			}
			parts = append(parts, p...)
		}
		return parts, nil
	}
	if v.Kind() == '{' {
		var o struct {
			Kind     string `json:"kind"`
			Language string `json:"language"`
			Value    string `json:"value"`
		}
		if err := json.Unmarshal(v, &o); err != nil {
			return nil, err
		}
		// A MarkedString with a language is code; a MarkupContent has a kind.
		switch {
		case o.Language != "":
			return []part{newPart(codePart, o.Language, o.Value)}, nil
		case o.Kind == plaintextPart:
			return []part{newPart(plaintextPart, "", o.Value)}, nil
		}
		return []part{newPart(markdownPart, "", o.Value)}, nil
	}
	return nil, nil
}

// newPart is a part with its line breaks as newlines only.
func newPart(format, fence, text string) part {
	return part{format: format, fence: fence, text: strings.ReplaceAll(text, "\r\n", "\n")}
}

// cutParts keeps the first limit bytes of the parts' texts, ending at a
// line break where there is one, else at a character, and says whether it
// cut any.
func cutParts(parts []part, limit int) ([]part, bool) {
	left := limit
	for i, p := range parts {
		if len(p.text) <= left {
			left -= len(p.text)
			continue
		}
		// A line break far back would cost most of what may be kept.
		end := strings.LastIndexByte(p.text[:left], '\n')
		if end < left/2 {
			end = runeStart(p.text, left)
		}
		// A copy, so the blocks do not hold on to the whole answer.
		parts[i].text = strings.Clone(p.text[:end])
		return parts[:i+1], true
	}
	return parts, false
}

func (p part) blocks(languages lang.Table) []hover.Block {
	switch p.format {
	case codePart:
		return []hover.Block{{Kind: hover.Code, Lang: languages.FromFence(p.fence), Code: p.text}}
	case plaintextPart:
		var bs []hover.Block
		for para := range strings.SplitSeq(p.text, "\n\n") {
			bs = append(bs, hover.Block{Kind: hover.Paragraph, Spans: []hover.Span{{Text: strings.Trim(para, "\n")}}})
		}
		return bs
	}
	return hover.Markdown(p.text, languages)
}
