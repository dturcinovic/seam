# Configuration

seam runs without a configuration. To change its defaults, write
`$XDG_CONFIG_HOME/seam/config.toml` (else `~/.config/seam/config.toml`).

A key that is set replaces the default; one that is not keeps it. A
misspelt key or a broken file stops seam with the file, line and reason, so
a typo never goes unnoticed.

The file has seven tables: [`[servers]` and `[languages]`](#language-servers-and-languages)
for hover, [`[ui]`](#viewer) for the view, [`[limits]`](#limits),
[`[colors]`](#colours), [`[keys]`](#keys) and [`[updates]`](#updates).

## Language servers and languages

Which servers seam knows by default, and how to install them, is in
[Language servers](language-servers.md).

```toml
# The command is one program, its arguments separate; no shell reads them.
[servers.gopls]
command = "/opt/go/bin/gopls"
args = ["serve"]
root_markers = ["go.work", "go.mod"]

# Off: no hover for its languages.
[servers.vscode-html-language-server]
enabled = false

# Sent with initialize, unread by seam.
[servers.intelephense.initialization_options]
storagePath = "/tmp/intelephense"

# Answers to workspace/configuration, by section: here the
# Python environment pyright resolves imports in.
[servers.pyright.settings.python]
pythonPath = ".venv/bin/python"

# Map files to a language, or take its hover away with server = "".
[languages.typescript]
extensions = [".ts", ".mts", ".cts"]
language_id = "typescript"
server = "tsgo"

# A new language: its name is the chroma lexer's.
[servers.vue-language-server]
command = "vue-language-server"
args = ["--stdio"]
root_markers = ["package.json"]

[languages.vue]
extensions = [".vue"]
server = "vue-language-server"
```

| Key | Meaning |
|---|---|
| `servers.<name>.command` | The program, looked up on the `PATH` unless it is a path |
| `servers.<name>.args` | Its arguments |
| `servers.<name>.enabled` | `false` turns the server off |
| `servers.<name>.root_markers` | Files that mark a workspace root: the nearest directory above the file that holds one; else the repository root |
| `servers.<name>.initialization_options` | Sent with `initialize` as they are |
| `servers.<name>.settings` | The answers to `workspace/configuration`, by section |
| `languages.<name>.extensions` | The file extensions of the language, with the dot |
| `languages.<name>.language_id` | The LSP language id, if it differs from the name |
| `languages.<name>.server` | The server that answers for it; `""` takes the hover away |

The built-in languages are `go`, `rust`, `javascript`, `typescript`, `jsx`,
`tsx`, `html`, `php` and `python`. A language of another name is new; its
name selects the [chroma](https://github.com/alecthomas/chroma) lexer that
colours it. An extension belongs to one language; to move `.js` to another,
take it from `javascript` too.

## Viewer

```toml
[ui]
theme = "light"          # dark (the default) or light
context_lines = 3        # around each change in the hunk view, 0 to 1000
hover_delay = "300ms"    # how long the mouse rests before a hover, up to 5s
layout = "auto"          # auto, split (side by side) or unified
split_min_width = 120    # auto is unified below this many columns, 0 to 1000
view = "file"            # file (one at a time) or stream (all files in one)
```

The values shown are the defaults, except `theme`.

The width `split_min_width` counts is the diff's: the terminal's less the
file list, so opening the list can turn the view unified. `u` turns the
sides the other way for the rest of the session, whatever the width.

## Limits

```toml
[limits]
max_file_size = "8M"     # largest side of a file to load; --max-file-size wins
max_instances = 4        # language servers running at once, 1 to 16
```

`max_file_size` takes bytes, or a number with `K`, `M` or `G`. A larger
file shows as too large instead of its diff.

## Colours

```toml
# Over the theme's palette: "#rrggbb", an ANSI colour "0" to "255", or ""
# for the terminal's own.
[colors]
inserted_bg = "#d7f5dd"
keyword = "#a626a4"
```

The colours of the view are `deleted_bg`, `deleted_strong_bg`,
`inserted_bg`, `inserted_strong_bg`, `changed_bg`, `changed_strong_bg` (the
strong ones mark changed words), `absent_bg`, `dim_fg`, `header_bg`,
`thumb_bg`, `popup_bg`, `code_span_bg`, `error_fg`, `selected_bg`,
`selected_fg`, `dir_fg`, `gutter_fg`, `zero_fg`, `deleted_fg`,
`inserted_fg` and `changed_fg`.

Those of the syntax are `plain`, `keyword`, `type`, `function`, `builtin`,
`constant`, `string`, `escape`, `number`, `comment`, `meta`, `operator`,
`punctuation`, `tag`, `attribute` and `variable`.

## Keys

```toml
# Keys replace an action's defaults; [] leaves it without, but quit needs one.
[keys]
quit = ["q", "ctrl+c"]
next_change = ["n", "ctrl+n"]
```

Keys are written as the terminal sends them: modifiers in the order `ctrl`,
`alt`, `shift`, `meta`, `hyper`, `super`, then the key's character (`G`,
not `shift+g`) or its name (`enter`, `tab`, `esc`, `space`, `backspace`,
`up`, `down`, `left`, `right`, `home`, `end`, `pgup`, `pgdown`, `insert`,
`delete`, `f1` to `f63`).

A key does one thing in each place. The popup, the focused file list and
the caret hide the keys of the view beneath them. A key you bind leaves the
defaults of the other actions in its place; two actions you bind to the
same key in one place are an error.

| Place | Action | Default keys |
|---|---|---|
| view and file list | `down` | `j`, `down` |
| | `up` | `k`, `up` |
| | `half_page_down` | `ctrl+d` |
| | `half_page_up` | `ctrl+u` |
| | `page_down` | `pgdown`, `space` |
| | `page_up` | `pgup` |
| | `top` | `g`, `home` |
| | `bottom` | `G`, `end` |
| view | `quit` | `q`, `esc`, `ctrl+c` |
| | `reload` | `r` |
| | `toggle_view` | `f` |
| | `toggle_layout` | `u` |
| | `toggle_stream` | `s` |
| | `next_change` | `n` |
| | `prev_change` | `p` |
| | `next_file` | `tab`, `]` |
| | `prev_file` | `shift+tab`, `[` |
| | `scroll_left` | `h`, `left` |
| | `scroll_right` | `l`, `right` |
| | `toggle_files` | `b` |
| | `start_caret` | `i` |
| | `jump_back` | `ctrl+o` |
| | `note` | `c` |
| | `next_note` | `m` |
| | `prev_note` | `M` |
| | `notes` | `N` |
| file list | `open` | `enter` |
| | `back` | `esc` |
| | `prev_folder` | `[` |
| | `next_folder` | `]` |
| caret | `leave_caret` | `i`, `esc` |
| | `caret_down` | `j`, `down` |
| | `caret_up` | `k`, `up` |
| | `caret_left` | `h`, `left` |
| | `caret_right` | `l`, `right` |
| | `word_forward` | `w` |
| | `word_back` | `b` |
| | `word_end` | `e` |
| | `other_side` | `o` |
| | `hover` | `K` |
| | `definition` | `d` |
| | `select_lines` | `v` |
| popup | `close_popup` | `esc` |
| | `resolve_note` | `x` |

What the actions do is in [Usage](usage.md).

## Updates

```toml
[updates]
check = false            # never ask for a newer release
```

What the check does, and `SEAM_NO_UPDATE_CHECK` for machines without a
configuration file, is in [Usage](usage.md#newer-releases).
