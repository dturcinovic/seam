package lsp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"seam/internal/domain/hover"
)

// docSet is the documents an instance's server is to have open, with the
// text it is to have for each. The pool owns it for the instance's life;
// each client of the instance reads it, so a server started anew finds
// every document without a copy.
type docSet struct {
	mu   sync.Mutex
	docs map[string]desiredDoc
	// dirty is the signal of the client now serving the set.
	dirty chan struct{}
}

func newDocSet() *docSet { return &docSet{docs: make(map[string]desiredDoc)} }

// attach makes dirty the signal of a change, and sets it: a new client has
// everything to send.
func (s *docSet) attach(dirty chan struct{}) {
	s.mu.Lock()
	s.dirty = dirty
	s.mu.Unlock()
	notify(dirty)
}

func (s *docSet) put(d desiredDoc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[d.Path] = d
	s.notifyLocked()
}

// remove drops path and reports how many documents are left.
func (s *docSet) remove(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, path)
	s.notifyLocked()
	return len(s.docs)
}

func (s *docSet) get(path string) (desiredDoc, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[path]
	return d, ok
}

func (s *docSet) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.docs)
}

func (s *docSet) notifyLocked() {
	if s.dirty != nil {
		notify(s.dirty)
	}
}

// since compares the set with what a server has: the paths to close and
// the documents to open or change, each in path order.
func (s *docSet) since(sent map[string]hover.DocVersion) (closed []string, changed []desiredDoc) {
	s.mu.Lock()
	for _, d := range s.docs {
		if v, ok := sent[d.Path]; !ok || v != d.Version {
			changed = append(changed, d)
		}
	}
	for path := range sent {
		if _, ok := s.docs[path]; !ok {
			closed = append(closed, path)
		}
	}
	s.mu.Unlock()
	slices.Sort(closed)
	slices.SortFunc(changed, func(a, b desiredDoc) int { return strings.Compare(a.Path, b.Path) })
	return closed, changed
}

// markers answers whether a directory of the repository holds a root
// marker. The answers are kept for the session: a stat on every hover
// would add up, and a project root that appears mid-session is rare.
type markers struct {
	root  string
	known map[[2]string]bool
}

func newMarkers(root string) *markers {
	return &markers{root: root, known: make(map[[2]string]bool)}
}

// has reports whether dir, relative to the repository, holds name. Its
// caller serialises the calls.
func (m *markers) has(dir, name string) bool {
	key := [2]string{dir, name}
	if has, ok := m.known[key]; ok {
		return has
	}
	_, err := os.Stat(filepath.Join(m.root, filepath.FromSlash(dir), name))
	m.known[key] = err == nil
	return err == nil
}
