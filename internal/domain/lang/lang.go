package lang

import (
	"path"
	"slices"
	"strings"
)

// Lang is a language by its canonical code fence name, which is also the
// name chroma knows it by. It is a string, not a closed set, so languages
// can come from configuration too. The zero value is no known language:
// such text has no syntax colours.
type Lang string

// The languages of the plan's table of languages and servers.
const (
	Go         Lang = "go"
	Rust       Lang = "rust"
	JavaScript Lang = "javascript"
	TypeScript Lang = "typescript"
	JSX        Lang = "jsx"
	TSX        Lang = "tsx"
	HTML       Lang = "html"
	PHP        Lang = "php"
	Python     Lang = "python"
)

// Language is how a language is recognised and which server hovers over
// it. It is a plain value; NewTable checks a set of them.
type Language struct {
	Lang Lang
	// Extensions are lower case with their dot, as in ".go".
	Extensions []string
	// Interpreters are programs a #! line names, by prefix, as python
	// matches python3.
	Interpreters []string
	// Heads are how a file's text may start, as "<?php".
	Heads []string
	// Fences are the other names a Markdown code fence gives it.
	Fences []string
	// Server names the server that hovers over it; empty is none.
	Server string
	// LanguageID is the LSP languageId; empty means Lang.
	LanguageID string
}

// Detect returns the language of the file at p, whose text starts with
// head: by its extension, else by how the text starts or the program a #!
// line names, as scripts often have no extension.
func (t Table) Detect(p, head string) Lang {
	ext := strings.ToLower(path.Ext(p))
	for _, l := range t.languages {
		if slices.Contains(l.Extensions, ext) {
			return l.Lang
		}
	}
	program := interpreter(head)
	for _, l := range t.languages {
		if slices.ContainsFunc(l.Heads, func(h string) bool { return strings.HasPrefix(head, h) }) ||
			program != "" && slices.ContainsFunc(l.Interpreters, func(i string) bool { return strings.HasPrefix(program, i) }) {
			return l.Lang
		}
	}
	return ""
}

// interpreter returns the program a #! line names, directly or through
// env, as in "#!/usr/bin/env -S python3 -u".
func interpreter(head string) string {
	first, _, _ := strings.Cut(head, "\n")
	rest, ok := strings.CutPrefix(first, "#!")
	if !ok {
		return ""
	}
	words := strings.Fields(rest)
	if len(words) > 0 && path.Base(words[0]) == "env" {
		words = words[1:]
		for len(words) > 0 && strings.HasPrefix(words[0], "-") {
			words = words[1:]
		}
	}
	if len(words) == 0 {
		return ""
	}
	return path.Base(words[0])
}

// FromFence returns the language a Markdown code fence names, as language
// servers write them, or the zero value for one the table does not know.
func (t Table) FromFence(name string) Lang {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, l := range t.languages {
		if name == string(l.Lang) || slices.Contains(l.Fences, name) {
			return l.Lang
		}
	}
	return ""
}
