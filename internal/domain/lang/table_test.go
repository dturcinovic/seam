package lang_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/domain/lang"
)

func TestDefaultsKeepTheRulesOfNewTable(t *testing.T) {
	d := lang.Defaults()
	table, err := lang.NewTable(d.Languages(), d.Servers())
	require.NoError(t, err)
	assert.Equal(t, d, table)
}

func TestDefaultServers(t *testing.T) {
	tests := []struct {
		l          lang.Lang
		server     string
		program    string
		args       []string
		languageID string
	}{
		{lang.Go, "gopls", "gopls", nil, "go"},
		{lang.Rust, "rust-analyzer", "rust-analyzer", nil, "rust"},
		{lang.JavaScript, "tsgo", "tsc", []string{"--lsp", "--stdio"}, "javascript"},
		{lang.TypeScript, "tsgo", "tsc", []string{"--lsp", "--stdio"}, "typescript"},
		{lang.JSX, "tsgo", "tsc", []string{"--lsp", "--stdio"}, "javascriptreact"},
		{lang.TSX, "tsgo", "tsc", []string{"--lsp", "--stdio"}, "typescriptreact"},
		{lang.HTML, "vscode-html-language-server", "vscode-html-language-server", []string{"--stdio"}, "html"},
		{lang.PHP, "intelephense", "intelephense", []string{"--stdio"}, "php"},
		{lang.Python, "pyright", "pyright-langserver", []string{"--stdio"}, "python"},
	}
	require.Len(t, tests, len(lang.Defaults().Languages()))
	for _, tt := range tests {
		t.Run(string(tt.l), func(t *testing.T) {
			s, id, ok := lang.Defaults().Server(tt.l)
			require.True(t, ok)
			assert.Equal(t, tt.server, s.Name)
			assert.Equal(t, tt.program, s.Command)
			assert.Equal(t, tt.args, s.Args)
			assert.Equal(t, tt.languageID, id)
		})
	}
}

func TestServerOfALanguageWithout(t *testing.T) {
	table, err := lang.NewTable([]lang.Language{
		{Lang: "plain", Extensions: []string{".txt"}},
		{Lang: lang.Go, Extensions: []string{".go"}, Server: "gopls"},
	}, []lang.Server{{Name: "gopls", Disabled: true}})
	require.NoError(t, err)

	for _, l := range []lang.Lang{"plain", lang.Go, "unknown", ""} {
		_, _, ok := table.Server(l)
		assert.False(t, ok, l)
	}
	// A disabled server still names its language: highlighting does not
	// depend on it.
	assert.Equal(t, lang.Go, table.Detect("a.go", ""))
}

func TestNewTableRejects(t *testing.T) {
	gopls := lang.Server{Name: "gopls", Command: "gopls"}
	golang := lang.Language{Lang: lang.Go, Extensions: []string{".go"}, Server: "gopls"}
	tests := map[string]struct {
		languages []lang.Language
		servers   []lang.Server
	}{
		"a server without a name":        {servers: []lang.Server{{Command: "x"}}},
		"a server without a command":     {servers: []lang.Server{{Name: "gopls"}}},
		"a server twice":                 {servers: []lang.Server{gopls, gopls}},
		"a root marker with a slash":     {servers: []lang.Server{{Name: "x", Command: "x", RootMarkers: []string{"a/go.mod"}}}},
		"an empty root marker":           {servers: []lang.Server{{Name: "x", Command: "x", RootMarkers: []string{""}}}},
		"a root marker of ..":            {servers: []lang.Server{{Name: "x", Command: "x", RootMarkers: []string{".."}}}},
		"a language without a name":      {languages: []lang.Language{{Extensions: []string{".x"}}}},
		"an upper case language":         {languages: []lang.Language{{Lang: "Go"}}},
		"a language with a space":        {languages: []lang.Language{{Lang: "my lang"}}},
		"a language twice":               {languages: []lang.Language{{Lang: "x"}, {Lang: "x"}}},
		"an unknown server":              {languages: []lang.Language{golang}},
		"an extension without a dot":     {languages: []lang.Language{{Lang: "x", Extensions: []string{"go"}}}},
		"an extension of only a dot":     {languages: []lang.Language{{Lang: "x", Extensions: []string{"."}}}},
		"an upper case extension":        {languages: []lang.Language{{Lang: "x", Extensions: []string{".GO"}}}},
		"an extension with a slash":      {languages: []lang.Language{{Lang: "x", Extensions: []string{".a/b"}}}},
		"an extension of two languages":  {languages: []lang.Language{{Lang: "x", Extensions: []string{".x"}}, {Lang: "y", Extensions: []string{".x"}}}},
		"an extension twice in one lang": {languages: []lang.Language{{Lang: "x", Extensions: []string{".x", ".x"}}}},
		"an extension of two dots":       {languages: []lang.Language{{Lang: "x", Extensions: []string{".d.ts"}}}},
		"a name that is another's fence": {languages: []lang.Language{{Lang: "js", Fences: []string{"node"}}, {Lang: "node"}}},
		"a fence of two languages":       {languages: []lang.Language{{Lang: "x", Fences: []string{"f"}}, {Lang: "y", Fences: []string{"f"}}}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := lang.NewTable(tt.languages, tt.servers)
			require.ErrorIs(t, err, lang.ErrInvalid)
		})
	}
}

func TestNewTableAccepts(t *testing.T) {
	table, err := lang.NewTable([]lang.Language{
		{Lang: "vue", Extensions: []string{".vue"}, Server: "vls", LanguageID: "vue"},
		{Lang: "plain"},
	}, []lang.Server{
		{Name: "vls", Command: "/opt/my tools/vls", Args: []string{"--stdio", "a b"}},
		// Off, so it needs no command.
		{Name: "unused", Disabled: true},
	})
	require.NoError(t, err)

	assert.Equal(t, lang.Lang("vue"), table.Detect("src/App.vue", ""))
	s, id, ok := table.Server("vue")
	require.True(t, ok)
	assert.Equal(t, "/opt/my tools/vls", s.Command)
	assert.Equal(t, []string{"--stdio", "a b"}, s.Args)
	assert.Equal(t, "vue", id)
	assert.Equal(t, []lang.Lang{"vue", "plain"}, langs(table))
	assert.Equal(t, []string{"unused", "vls"}, names(table.Servers()))
}

func TestTheZeroTableKnowsNothing(t *testing.T) {
	var table lang.Table
	assert.Empty(t, table.Detect("a.go", ""))
	assert.Empty(t, table.FromFence("go"))
	assert.Empty(t, table.Languages())
	_, _, ok := table.Server(lang.Go)
	assert.False(t, ok)
}

func TestATableKeepsItsOwnCopies(t *testing.T) {
	languages := []lang.Language{{Lang: "x", Extensions: []string{".x"}, Server: "s"}}
	servers := []lang.Server{{Name: "s", Command: "s", Args: []string{"a"}, RootMarkers: []string{"m"}}}
	table, err := lang.NewTable(languages, servers)
	require.NoError(t, err)

	languages[0].Lang = "y"
	languages[0].Extensions[0] = ".Y"
	servers[0].Args[0] = "changed"
	table.Languages()[0].Extensions[0] = ".Z"
	table.Servers()[0].RootMarkers[0] = "changed"
	s, _, _ := table.Server("x")
	s.Args[0] = "changed"

	assert.Equal(t, lang.Lang("x"), table.Detect("a.x", ""))
	s, _, _ = table.Server("x")
	assert.Equal(t, []string{"a"}, s.Args)
	assert.Equal(t, []string{"m"}, s.RootMarkers)
}

func names(servers []lang.Server) []string {
	var all []string
	for _, s := range servers {
		all = append(all, s.Name)
	}
	return all
}
