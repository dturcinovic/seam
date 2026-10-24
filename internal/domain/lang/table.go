package lang

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrInvalid is a table of languages and servers that breaks one of the
// rules NewTable checks.
var ErrInvalid = errors.New("invalid language table")

// Server is how to start a language server. Command is one program, run
// as is, with Args as separate arguments; no shell sees either.
type Server struct {
	Name    string
	Command string
	Args    []string
	// Disabled keeps the server from starting; its languages have no hover.
	Disabled bool
	// RootMarkers are file names that mark a project root for it, as
	// "go.mod".
	RootMarkers []string
	// InitializationOptions and Settings go to the server unread, the
	// first with initialize, the second as workspace/configuration answers.
	// A table hands them out shared: they are read, never changed.
	InitializationOptions map[string]any
	Settings              map[string]any
}

// Table is the languages and servers of a session. Its zero value knows no
// language; Defaults is the plan's table.
type Table struct {
	languages []Language
	servers   map[string]Server
}

// NewTable checks languages and servers against each other and returns
// them as a table. The languages are tried in the order given.
func NewTable(languages []Language, servers []Server) (Table, error) {
	seen := make(map[string]bool, len(servers))
	for _, s := range servers {
		if err := checkServer(s); err != nil {
			return Table{}, err
		}
		if seen[s.Name] {
			return Table{}, fmt.Errorf("%w: server %q appears twice", ErrInvalid, s.Name)
		}
		seen[s.Name] = true
	}
	t := build(languages, servers)
	if err := t.checkLanguages(); err != nil {
		return Table{}, err
	}
	return t, nil
}

// build is the one way a table is made; NewTable checks what it made. It
// copies the slices, so a caller cannot change a checked table afterwards.
func build(languages []Language, servers []Server) Table {
	t := Table{languages: make([]Language, 0, len(languages)), servers: make(map[string]Server, len(servers))}
	for _, l := range languages {
		t.languages = append(t.languages, l.clone())
	}
	for _, s := range servers {
		t.servers[s.Name] = s.clone()
	}
	return t
}

func (l Language) clone() Language {
	l.Extensions = slices.Clone(l.Extensions)
	l.Interpreters = slices.Clone(l.Interpreters)
	l.Heads = slices.Clone(l.Heads)
	l.Fences = slices.Clone(l.Fences)
	return l
}

func (s Server) clone() Server {
	s.Args = slices.Clone(s.Args)
	s.RootMarkers = slices.Clone(s.RootMarkers)
	return s
}

func checkServer(s Server) error {
	if s.Name == "" {
		return fmt.Errorf("%w: a server has no name", ErrInvalid)
	}
	if !s.Disabled && s.Command == "" {
		return fmt.Errorf("%w: server %q has no command", ErrInvalid, s.Name)
	}
	for _, m := range s.RootMarkers {
		if m == "" || m == "." || m == ".." || strings.Contains(m, "/") {
			return fmt.Errorf("%w: server %q: root marker %q is no file name", ErrInvalid, s.Name, m)
		}
	}
	return nil
}

// checkLanguages checks each language, then that no extension and no name
// a fence may give belongs to two: Detect and FromFence take the first that
// matches, so the second could never be found.
func (t Table) checkLanguages() error {
	extensions := make(map[string]Lang)
	names := make(map[string]Lang)
	for _, l := range t.languages {
		if err := t.checkLanguage(l); err != nil {
			return err
		}
		for _, ext := range l.Extensions {
			if other, dup := extensions[ext]; dup {
				return fmt.Errorf("%w: extension %q belongs to both %q and %q", ErrInvalid, ext, other, l.Lang)
			}
			extensions[ext] = l.Lang
		}
		for _, name := range append([]string{string(l.Lang)}, l.Fences...) {
			if other, dup := names[name]; dup {
				return fmt.Errorf("%w: name %q belongs to both %q and %q", ErrInvalid, name, other, l.Lang)
			}
			names[name] = l.Lang
		}
	}
	return nil
}

func (t Table) checkLanguage(l Language) error {
	// FromFence lowers the name it is given, so an upper case language
	// could never be found by it.
	if l.Lang == "" || string(l.Lang) != strings.ToLower(string(l.Lang)) || strings.ContainsAny(string(l.Lang), " \t") {
		return fmt.Errorf("%w: language %q needs a lower case name without spaces", ErrInvalid, l.Lang)
	}
	if _, ok := t.servers[l.Server]; l.Server != "" && !ok {
		return fmt.Errorf("%w: language %q: no server %q", ErrInvalid, l.Lang, l.Server)
	}
	for _, ext := range l.Extensions {
		// Detect compares path.Ext, the part from the last dot: ".d.ts"
		// could never match.
		if len(ext) < 2 || strings.LastIndexByte(ext, '.') != 0 || ext != strings.ToLower(ext) || strings.Contains(ext, "/") {
			return fmt.Errorf("%w: language %q: extension %q needs one leading dot and lower case", ErrInvalid, l.Lang, ext)
		}
	}
	return nil
}

// Languages returns the languages in the order they are tried.
func (t Table) Languages() []Language {
	all := make([]Language, 0, len(t.languages))
	for _, l := range t.languages {
		all = append(all, l.clone())
	}
	return all
}

// Servers returns the servers by name.
func (t Table) Servers() []Server {
	all := make([]Server, 0, len(t.servers))
	for _, s := range t.servers {
		all = append(all, s.clone())
	}
	slices.SortFunc(all, func(a, b Server) int { return strings.Compare(a.Name, b.Name) })
	return all
}

// Server returns the server that hovers over l and the languageId l has
// for it. ok is false for a language without a server or with a disabled
// one.
func (t Table) Server(l Lang) (s Server, languageID string, ok bool) {
	i := slices.IndexFunc(t.languages, func(x Language) bool { return x.Lang == l })
	if i < 0 || t.languages[i].Server == "" {
		return Server{}, "", false
	}
	s = t.servers[t.languages[i].Server].clone()
	if s.Disabled {
		return Server{}, "", false
	}
	if languageID = t.languages[i].LanguageID; languageID == "" {
		languageID = string(l)
	}
	return s, languageID, true
}
