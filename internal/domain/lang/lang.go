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

// spec is how to recognise a language: by file extension, by the program a
// #! line names (a prefix, as in python3), by how its text starts, and by
// the other names a code fence gives it.
type spec struct {
	lang         Lang
	extensions   []string
	interpreters []string
	heads        []string
	fences       []string
}

// specs are the languages seam knows by default, one row each, so a new
// language is one more row.
func specs() []spec {
	return []spec{
		{lang: Go, extensions: []string{".go"}, fences: []string{"golang"}},
		{lang: Rust, extensions: []string{".rs"}, fences: []string{"rs"}},
		{lang: JavaScript, extensions: []string{".js", ".mjs", ".cjs"}, interpreters: []string{"node"}, fences: []string{"js", "mjs", "cjs", "node"}},
		{lang: TypeScript, extensions: []string{".ts", ".mts", ".cts"}, fences: []string{"ts", "mts", "cts"}},
		{lang: JSX, extensions: []string{".jsx"}, fences: []string{"javascriptreact"}},
		{lang: TSX, extensions: []string{".tsx"}, fences: []string{"typescriptreact"}},
		{lang: HTML, extensions: []string{".html", ".htm"}, fences: []string{"htm", "xhtml"}},
		{lang: PHP, extensions: []string{".php"}, interpreters: []string{"php"}, heads: []string{"<?php"}},
		{lang: Python, extensions: []string{".py", ".pyi"}, interpreters: []string{"python"}, fences: []string{"py", "python3", "py3", "pyi"}},
	}
}

// Known returns the languages seam knows by default.
func Known() []Lang {
	var all []Lang
	for _, s := range specs() {
		all = append(all, s.lang)
	}
	return all
}

// Detect returns the language of the file at p, whose text starts with
// head: by its extension, else by how the text starts or the program a #!
// line names, as scripts often have no extension.
func Detect(p, head string) Lang {
	ext := strings.ToLower(path.Ext(p))
	program := interpreter(head)
	for _, s := range specs() {
		if slices.Contains(s.extensions, ext) {
			return s.lang
		}
	}
	for _, s := range specs() {
		if slices.ContainsFunc(s.heads, func(h string) bool { return strings.HasPrefix(head, h) }) ||
			program != "" && slices.ContainsFunc(s.interpreters, func(i string) bool { return strings.HasPrefix(program, i) }) {
			return s.lang
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
// servers write them, or the zero value for one seam does not know.
func FromFence(name string) Lang {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, s := range specs() {
		if name == string(s.lang) || slices.Contains(s.fences, name) {
			return s.lang
		}
	}
	return ""
}
