package lang_test

import (
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"seam/internal/domain/lang"
)

// files answers has from a set of repository paths.
func files(paths ...string) func(dir, name string) bool {
	return func(dir, name string) bool { return slices.Contains(paths, path.Join(dir, name)) }
}

func TestInstance(t *testing.T) {
	repo := files("go.mod", "tools/go.mod", "web/package.json", "web/app/tsconfig.json", "crates/a/Cargo.toml", "api/composer.json", "py/pyproject.toml")
	tests := []struct {
		file string
		want lang.Instance
	}{
		{"main.go", lang.Instance{Server: "gopls", Root: "."}},
		{"internal/x/x.go", lang.Instance{Server: "gopls", Root: "."}},
		{"tools/cmd/gen/main.go", lang.Instance{Server: "gopls", Root: "tools"}},
		{"crates/a/src/lib.rs", lang.Instance{Server: "rust-analyzer", Root: "crates/a"}},
		{"crates/b/src/lib.rs", lang.Instance{Server: "rust-analyzer", Root: "."}},
		{"web/server.js", lang.Instance{Server: "typescript-language-server", Root: "web"}},
		{"web/app/page.tsx", lang.Instance{Server: "typescript-language-server", Root: "web/app"}},
		{"web/index.html", lang.Instance{Server: "vscode-html-language-server", Root: "."}},
		{"api/src/index.php", lang.Instance{Server: "intelephense", Root: "api"}},
		{"py/pkg/mod.py", lang.Instance{Server: "pyright", Root: "py"}},
		{"./py/pkg/../mod.py", lang.Instance{Server: "pyright", Root: "py"}},
	}
	table := lang.Defaults()
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got, ok := table.Instance(table.Detect(tt.file, ""), tt.file, repo)
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestJavaScriptAndTypeScriptOfOneWorkspaceShareAnInstance(t *testing.T) {
	repo := files("web/tsconfig.json")
	table := lang.Defaults()
	var all []lang.Instance
	for _, f := range []string{"web/a.js", "web/b.ts", "web/c/C.jsx", "web/c/D.tsx", "web/e.mjs", "web/f.cts"} {
		i, ok := table.Instance(table.Detect(f, ""), f, repo)
		require.True(t, ok, f)
		all = append(all, i)
	}
	for _, i := range all {
		assert.Equal(t, lang.Instance{Server: "typescript-language-server", Root: "web"}, i)
	}
}

func TestInstanceOfAGoWorkspace(t *testing.T) {
	// The nearer go.mod wins over a go.work further up; see the plan.
	table := lang.Defaults()
	got, ok := table.Instance(lang.Go, "mod/a.go", files("go.work", "mod/go.mod"))
	require.True(t, ok)
	assert.Equal(t, "mod", got.Root)
	got, _ = table.Instance(lang.Go, "other/a.go", files("go.work", "mod/go.mod"))
	assert.Equal(t, ".", got.Root)
}

func TestNoInstanceOutsideTheRepository(t *testing.T) {
	has := func(string, string) bool { return true }
	for _, f := range []string{"/abs/a.go", "../a.go", "a/../../a.go", ".."} {
		_, ok := lang.Defaults().Instance(lang.Go, f, has)
		assert.False(t, ok, f)
	}
}

func TestNoInstanceWithoutAServer(t *testing.T) {
	table, err := lang.NewTable([]lang.Language{{Lang: lang.Go, Extensions: []string{".go"}, Server: "gopls"}, {Lang: "plain"}},
		[]lang.Server{{Name: "gopls", Command: "gopls", Disabled: true}})
	require.NoError(t, err)
	asked := false
	has := func(string, string) bool { asked = true; return true }

	for _, l := range []lang.Lang{lang.Go, "plain", ""} {
		_, ok := table.Instance(l, "a.go", has)
		assert.False(t, ok, l)
	}
	assert.False(t, asked, "no server, no need to look for its root")
}

// TestTheRootIsAnAncestor: whatever the markers, the root is the file's
// directory or one above it inside the repository, and the search always
// ends.
func TestTheRootIsAnAncestor(t *testing.T) {
	segment := rapid.SampledFrom([]string{"a", "b", "web", ".", "..", ""})
	rapid.Check(t, func(t *rapid.T) {
		file := strings.Join(rapid.SliceOfN(segment, 1, 6).Draw(t, "file"), "/") + "/x.go"
		marked := rapid.SliceOf(rapid.SampledFrom([]string{".", "a", "a/b", "web", "a/web", ".."})).Draw(t, "marked")
		has := func(dir, _ string) bool { return slices.Contains(marked, dir) }

		got, ok := lang.Defaults().Instance(lang.Go, file, has)

		clean := path.Clean(file)
		if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			require.False(t, ok)
			return
		}
		require.True(t, ok)
		dir := path.Dir(clean)
		assert.True(t, got.Root == "." || dir == got.Root || strings.HasPrefix(dir, got.Root+"/"), "%q is not above %q", got.Root, dir)
	})
}
