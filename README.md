# seam

A side-by-side diff viewer for the terminal. One key switches between the
changed hunks and the whole file, and hovering over a symbol shows its type
and documentation from the language server.

Work in progress: the diff works, the hover is not yet.

## Build

Requires Go 1.27 and [Task](https://taskfile.dev).

```bash
task check    # the full gate
task build    # bin/seam
```

The log is written to `$XDG_STATE_HOME/seam/seam.log`.

## Language servers

Hover needs a language server for the file's language. seam installs none
and starts one only when a file of its language is shown. A server that is
missing or disabled costs the hover and nothing else: the diff, navigation
and syntax colours work without it.

| Files | Server | Install |
|---|---|---|
| `.go` | gopls | `go install golang.org/x/tools/gopls@latest` |
| `.rs` | rust-analyzer | `rustup component add rust-analyzer` |
| `.js` `.mjs` `.cjs` `.ts` `.mts` `.cts` `.jsx` `.tsx` | typescript-language-server | `npm install -g typescript-language-server typescript` |
| `.html` `.htm` | vscode-html-language-server | `npm install -g vscode-langservers-extracted` |
| `.php` | intelephense | `npm install -g intelephense` |
| `.py` `.pyi` | pyright | `npm install -g pyright` |

The npm servers need Node.js on the `PATH`. typescript-language-server uses
the project's own `typescript` package when it has one. pyright finds the
packages of a project only in the Python environment it is told about; see
`settings` below.

Each server runs once per workspace: the nearest directory above the file
that holds one of its root markers (`go.work` or `go.mod`, `Cargo.toml`,
`tsconfig.json`, `jsconfig.json` or `package.json`, `composer.json`,
`pyproject.toml`), else the repository root. JavaScript, TypeScript, JSX and
TSX of one workspace share one server.

## Configuration

`$XDG_CONFIG_HOME/seam/config.toml` (else `~/.config/seam/config.toml`) is
optional. A key that is set replaces the default; one that is not keeps it.
A misspelt key or a broken file stops seam with the file, line and reason.

```toml
# The command is one program, its arguments separate; no shell reads them.
[servers.gopls]
command = "/opt/go/bin/gopls"
args = ["serve"]
root_markers = ["go.work", "go.mod"]

# Off: no hover for its languages.
[servers.intelephense]
enabled = false

# Sent with initialize, unread by seam.
[servers.typescript-language-server.initialization_options]
preferences = { includeInlayParameterNameHints = "none" }

# Answers to workspace/configuration, by section: here the
# Python environment pyright resolves imports in.
[servers.pyright.settings.python]
pythonPath = ".venv/bin/python"

# Map files to a language, or take its hover away with server = "".
[languages.typescript]
extensions = [".ts", ".mts", ".cts"]
language_id = "typescript"
server = "typescript-language-server"

# A new language: its name is the chroma lexer's.
[servers.vue-language-server]
command = "vue-language-server"
args = ["--stdio"]
root_markers = ["package.json"]

[languages.vue]
extensions = [".vue"]
server = "vue-language-server"
```

An extension belongs to one language; to move `.js` to another, take it
from `javascript` too.
