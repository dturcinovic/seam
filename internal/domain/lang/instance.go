package lang

import (
	"path"
	"slices"
	"strings"
)

// Instance is one running server: a server for one workspace. Files that
// map to the same instance share a process, as JavaScript and TSX of one
// project do.
type Instance struct {
	Server string
	// Root is the workspace, relative to the repository root in slash
	// form; "." is the repository root.
	Root string
}

// Instance returns the instance that hovers over file, a path relative to
// the repository root, in language l. The root is the nearest directory
// above file that has one of the server's root markers, else the
// repository root. has reports whether dir holds a file named name; the
// table reads no filesystem itself. ok is false where Server's is, and for
// a path that is absolute or leaves the repository.
func (t Table) Instance(l Lang, file string, has func(dir, name string) bool) (Instance, bool) {
	file = path.Clean(file)
	if path.IsAbs(file) || file == ".." || strings.HasPrefix(file, "../") {
		return Instance{}, false
	}
	s, _, ok := t.Server(l)
	if !ok {
		return Instance{}, false
	}
	return Instance{Server: s.Name, Root: root(path.Dir(file), s.RootMarkers, has)}, true
}

func root(dir string, markers []string, has func(dir, name string) bool) string {
	for {
		if slices.ContainsFunc(markers, func(m string) bool { return has(dir, m) }) {
			return dir
		}
		if dir == "." {
			return "."
		}
		dir = path.Dir(dir)
	}
}
