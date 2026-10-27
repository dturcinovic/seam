package configfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dturcinovic/seam/internal/adapter/driven/configfile"
	"github.com/dturcinovic/seam/internal/domain/lang"
	"github.com/dturcinovic/seam/internal/domain/ui"
)

func write(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(p, []byte(text), 0o600))
	return p
}

func load(t *testing.T, text string) lang.Table {
	t.Helper()
	f, err := configfile.Load(write(t, text))
	require.NoError(t, err)
	return f.Languages
}

func TestWithoutAFileTheDefaultsHold(t *testing.T) {
	f, err := configfile.Load(filepath.Join(t.TempDir(), "seam", "config.toml"))
	require.NoError(t, err)
	assert.Equal(t, configfile.File{Languages: lang.Defaults(), UI: ui.DefaultSettings(), UpdateCheck: true}, f)
}

func TestAnEmptyFileIsTheDefaults(t *testing.T) {
	assert.Equal(t, lang.Defaults(), load(t, "# nothing set\n"))
}

func TestAnOwnCommand(t *testing.T) {
	table := load(t, `
[servers.gopls]
command = "/opt/go tools/gopls"
args = ["-remote=auto", "serve"]
`)
	s, _, ok := table.Server(lang.Go)
	require.True(t, ok)
	assert.Equal(t, "/opt/go tools/gopls", s.Command)
	assert.Equal(t, []string{"-remote=auto", "serve"}, s.Args)
	assert.Equal(t, []string{"go.work", "go.mod"}, s.RootMarkers, "not set, so the default stays")
}

func TestAShellLineIsOneProgram(t *testing.T) {
	// No shell reads the command: the whole line names one program, which
	// is then not found rather than run as a pipeline.
	table := load(t, `servers.pyright.command = "pyright-langserver --stdio | tee /tmp/x"`)
	s, _, ok := table.Server(lang.Python)
	require.True(t, ok)
	assert.Equal(t, "pyright-langserver --stdio | tee /tmp/x", s.Command)
	assert.Equal(t, []string{"--stdio"}, s.Args)
}

func TestADisabledServer(t *testing.T) {
	table := load(t, `
[servers.tsgo]
enabled = false
`)
	for _, l := range []lang.Lang{lang.JavaScript, lang.TypeScript, lang.JSX, lang.TSX} {
		_, _, ok := table.Server(l)
		assert.False(t, ok, l)
		assert.Equal(t, l, table.FromFence(string(l)), "still a language for the colours")
	}
	_, _, ok := table.Server(lang.Go)
	assert.True(t, ok)

	table = load(t, "[servers.tsgo]\nenabled = false\n[servers.tsgo.settings]\n")
	_, _, ok = table.Server(lang.TSX)
	assert.False(t, ok)
	table = load(t, "servers.gopls.enabled = true\n")
	_, _, ok = table.Server(lang.Go)
	assert.True(t, ok)
}

func TestALanguageWithoutHover(t *testing.T) {
	table := load(t, `languages.html.server = ""`)
	_, _, ok := table.Server(lang.HTML)
	assert.False(t, ok)
	assert.Equal(t, lang.HTML, table.Detect("index.html", ""))
}

func TestOptionsAndSettings(t *testing.T) {
	table := load(t, `
[servers.pyright.settings.python]
pythonPath = "/srv/app/.venv/bin/python"

[servers.pyright.initialization_options]
verbose = true
depth = 2
`)
	s, _, ok := table.Server(lang.Python)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"python": map[string]any{"pythonPath": "/srv/app/.venv/bin/python"}}, s.Settings)
	assert.Equal(t, map[string]any{"verbose": true, "depth": int64(2)}, s.InitializationOptions)
}

func TestAMappingAndRootMarkers(t *testing.T) {
	table := load(t, `
[languages.typescript]
extensions = [".ts", ".mts", ".cts", ".tsx"]
language_id = "ts"

[languages.tsx]
extensions = []

[servers.tsgo]
root_markers = ["deno.json"]
`)
	assert.Equal(t, lang.TypeScript, table.Detect("App.tsx", ""))
	_, id, ok := table.Server(lang.TypeScript)
	require.True(t, ok)
	assert.Equal(t, "ts", id)
	_, id, _ = table.Server(lang.JSX)
	assert.Equal(t, "javascriptreact", id, "untouched")

	has := func(dir, name string) bool { return dir == "app" && name == "deno.json" }
	i, ok := table.Instance(lang.TypeScript, "app/src/a.ts", has)
	require.True(t, ok)
	assert.Equal(t, "app", i.Root)
}

func TestANewLanguageAndServer(t *testing.T) {
	table := load(t, `
[servers.vue-language-server]
command = "vue-language-server"
args = ["--stdio"]
root_markers = ["package.json"]

[languages.vue]
extensions = [".vue"]
server = "vue-language-server"
`)
	assert.Equal(t, lang.Lang("vue"), table.Detect("src/App.vue", ""))
	s, id, ok := table.Server("vue")
	require.True(t, ok)
	assert.Equal(t, "vue-language-server", s.Command)
	assert.Equal(t, "vue", id)
	assert.Equal(t, lang.Go, table.Detect("main.go", ""), "the defaults stay")
}

func TestABrokenFile(t *testing.T) {
	tests := map[string]struct {
		text string
		want []string
	}{
		"no TOML":                        {"[servers\n", []string{":1:"}},
		"a wrong type":                   {"[servers.gopls]\nenabled = \"no\"\n", []string{":2:", "servers.gopls.enabled"}},
		"a misspelt key":                 {"[servers.gopls]\ncomand = \"gopls\"\n", []string{":2:1: unknown key servers.gopls.comand"}},
		"two misspelt keys":              {"[languages.go]\nserver = \"gopls\"\nlanguage = \"go\"\nextension = []\n", []string{":3:1: unknown key languages.go.language", ":4:1: unknown key languages.go.extension"}},
		"an unknown section":             {"[server.gopls]\n", []string{"unknown key server"}},
		"an unknown server":              {"languages.go.server = \"go-ls\"\n", []string{`language "go": no server "go-ls"`}},
		"a new server without a command": {"servers.zls.args = []\n", []string{`server "zls" has no command`}},
		"an extension twice":             {"languages.vue.extensions = [\".js\"]\n", []string{`extension ".js" belongs to both "javascript" and "vue"`}},
		"an upper case language":         {"languages.Vue.extensions = [\".vue\"]\n", []string{`language "Vue"`}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			p := write(t, tt.text)
			_, err := configfile.Load(p)
			require.Error(t, err)
			assert.Contains(t, err.Error(), p)
			for _, w := range tt.want {
				assert.Contains(t, err.Error(), w)
			}
		})
	}
}

func TestALinkToAMissingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.Symlink(filepath.Join(t.TempDir(), "moved.toml"), p))

	_, err := configfile.Load(p)

	assert.ErrorContains(t, err, p+" links to a missing file")
}

func TestAnUnreadableFile(t *testing.T) {
	// A directory where the file should be: it exists, but cannot be read.
	_, err := configfile.Load(t.TempDir())
	assert.ErrorContains(t, err, "read configuration")
}
