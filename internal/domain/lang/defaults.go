package lang

// The default servers by name, as the configuration file names them.
const (
	gopls        = "gopls"
	rustAnalyzer = "rust-analyzer"
	tsServer     = "tsgo"
	htmlServer   = "vscode-html-language-server"
	intelephense = "intelephense"
	pyright      = "pyright"
)

// Defaults is the plan's table of languages and servers. None of the
// servers comes with seam; one that is not installed only means no hover.
func Defaults() Table {
	// Unchecked, as NewTable could only fail on a broken default; a test
	// holds the defaults to its rules instead.
	return build(defaultLanguages(), defaultServers())
}

func defaultLanguages() []Language {
	return []Language{
		{Lang: Go, Extensions: []string{".go"}, Fences: []string{"golang"}, Server: gopls},
		{Lang: Rust, Extensions: []string{".rs"}, Fences: []string{"rs"}, Server: rustAnalyzer},
		{Lang: JavaScript, Extensions: []string{".js", ".mjs", ".cjs"}, Interpreters: []string{"node"}, Fences: []string{"js", "mjs", "cjs", "node"}, Server: tsServer},
		{Lang: TypeScript, Extensions: []string{".ts", ".mts", ".cts"}, Fences: []string{"ts", "mts", "cts"}, Server: tsServer},
		{Lang: JSX, Extensions: []string{".jsx"}, Fences: []string{"javascriptreact"}, Server: tsServer, LanguageID: "javascriptreact"},
		{Lang: TSX, Extensions: []string{".tsx"}, Fences: []string{"typescriptreact"}, Server: tsServer, LanguageID: "typescriptreact"},
		{Lang: HTML, Extensions: []string{".html", ".htm"}, Fences: []string{"htm", "xhtml"}, Server: htmlServer},
		{Lang: PHP, Extensions: []string{".php"}, Interpreters: []string{"php"}, Heads: []string{"<?php"}, Server: intelephense},
		{Lang: Python, Extensions: []string{".py", ".pyi"}, Interpreters: []string{"python"}, Fences: []string{"py", "python3", "py3", "pyi"}, Server: pyright},
	}
}

func defaultServers() []Server {
	return []Server{
		{Name: gopls, Command: "gopls", RootMarkers: []string{"go.work", "go.mod"}},
		{Name: rustAnalyzer, Command: "rust-analyzer", RootMarkers: []string{"Cargo.toml"}},
		{Name: tsServer, Command: "tsc", Args: []string{"--lsp", "--stdio"}, RootMarkers: []string{"tsconfig.json", "jsconfig.json", "package.json"}},
		{Name: htmlServer, Command: "vscode-html-language-server", Args: []string{"--stdio"}},
		{Name: intelephense, Command: "intelephense", Args: []string{"--stdio"}, RootMarkers: []string{"composer.json"}},
		{Name: pyright, Command: "pyright-langserver", Args: []string{"--stdio"}, RootMarkers: []string{"pyproject.toml"}},
	}
}
