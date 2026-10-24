package archtest_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// moduleRoot walks up from the test's working directory to go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found")
		dir = parent
	}
}

// eachSourceFile parses every non-test Go file of the module and calls fn
// with its path relative to the module root.
func eachSourceFile(t *testing.T, fset *token.FileSet, fn func(rel string, f *ast.File)) {
	t.Helper()
	root := moduleRoot(t)
	// cmd/ and internal/ hold all Go code of the module. Walking only these,
	// not the root, keeps the test cached when bin/ or .git/ change.
	for _, dir := range []string{"cmd", "internal"} {
		walkSources(t, fset, root, filepath.Join(root, dir), fn)
	}
}

func walkSources(t *testing.T, fset *token.FileSet, root, dir string, fn func(rel string, f *ast.File)) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fn(filepath.ToSlash(rel), f)
		return nil
	})
	require.NoError(t, err)
}

// Every goroutine needs an owner that stops it and catches its panics, which
// only supervise provides. forbidigo cannot see a go statement, as it is no
// identifier.
func TestNoGoStatementOutsideSupervise(t *testing.T) {
	fset := token.NewFileSet()
	var found []string
	eachSourceFile(t, fset, func(rel string, f *ast.File) {
		if strings.HasPrefix(rel, "internal/supervise/") {
			return
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if stmt, ok := n.(*ast.GoStmt); ok {
				found = append(found, fmt.Sprintf("%s:%d", rel, fset.Position(stmt.Pos()).Line))
			}
			return true
		})
	})
	assert.Empty(t, found, "go statement outside internal/supervise; start the goroutine with supervise.Group.Go")
}
