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

### Viewer

```toml
[ui]
theme = "light"          # dark (the default) or light
context_lines = 3        # around each change in the hunk view, 0 to 1000
hover_delay = "300ms"    # how long the mouse rests before a hover, up to 5s
layout = "auto"          # auto, split (side by side) or unified
split_min_width = 120    # auto is unified below this many columns, 0 to 1000

[limits]
max_file_size = "16M"    # as --max-file-size, which wins over it
max_instances = 4        # language servers running at once, 1 to 16

# Over the theme's palette: "#rrggbb", an ANSI colour "0" to "255", or ""
# for the terminal's own.
[colors]
inserted_bg = "#d7f5dd"
keyword = "#a626a4"

# Keys replace an action's defaults; [] leaves it without, but quit needs one.
[keys]
quit = ["q", "ctrl+c"]
next_change = ["n", "ctrl+n"]
```

Keys are written as the terminal sends them: modifiers in the order `ctrl`,
`alt`, `shift`, `meta`, `hyper`, `super`, then the key's character (`G`,
not `shift+g`) or its name (`enter`, `tab`, `esc`, `space`, `backspace`,
`up`, `down`, `left`, `right`, `home`, `end`, `pgup`, `pgdown`, `insert`,
`delete`, `f1` to `f63`). A key you bind leaves the defaults of the other
actions in its place. A key does one thing in each place; the popup,
the focused file list and the caret hide the keys of the view beneath them.

The width `split_min_width` counts is the diff's: the terminal's less the
file list, so opening the list can turn the view unified. `u` turns the
sides the other way for the rest of the session, whatever the width; in the
unified view, `o` goes between the old and the new half of a changed line.

| Place | Actions (default keys) |
|---|---|
| view and file list | `down` (j, down), `up` (k, up), `half_page_down` (ctrl+d), `half_page_up` (ctrl+u), `page_down` (pgdown, space), `page_up` (pgup), `top` (g, home), `bottom` (G, end) |
| view | `quit` (q, esc, ctrl+c), `reload` (r), `toggle_view` (f), `toggle_layout` (u), `next_change` (n), `prev_change` (p), `next_file` (tab, ]), `prev_file` (shift+tab, [), `scroll_left` (h, left), `scroll_right` (l, right), `toggle_files` (b), `start_caret` (i) |
| file list | `open` (enter), `back` (esc), `prev_folder` ([), `next_folder` (]) |
| caret | `leave_caret` (i, esc), `caret_down` (j, down), `caret_up` (k, up), `caret_left` (h, left), `caret_right` (l, right), `word_forward` (w), `word_back` (b), `word_end` (e), `other_side` (o), `hover` (K) |
| popup | `close_popup` (esc) |

The colours are `deleted_bg`, `deleted_strong_bg`, `inserted_bg`,
`inserted_strong_bg`, `changed_bg`, `changed_strong_bg` (the strong ones
mark changed words), `absent_bg`, `dim_fg`, `header_bg`, `thumb_bg`,
`popup_bg`, `code_span_bg`, `error_fg`, `selected_bg`, `selected_fg`,
`dir_fg`, `gutter_fg`, `zero_fg`, `deleted_fg`, `inserted_fg`, `changed_fg`
and those of the syntax: `plain`, `keyword`, `type`, `function`, `builtin`,
`constant`, `string`, `escape`, `number`, `comment`, `meta`, `operator`,
`punctuation`, `tag`, `attribute`, `variable`.
