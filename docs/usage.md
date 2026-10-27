# Usage

## What seam compares

Run seam inside a git repository.

```bash
seam                # HEAD against the worktree, untracked files included
seam <rev>          # a revision against the worktree, untracked files included
seam <rev> <rev>    # two commits
seam --staged       # HEAD against the index
seam --watch        # as seam, and reload whenever something changes
seam --files <a> <b>  # two files or two directories on disk, see below
```

A revision is anything git understands: `main`, `HEAD~3`, a tag or a
commit hash.

| Flag | Meaning |
|---|---|
| `-w`, `--watch` | Reload when the worktree, the index or HEAD changes, see [Watching](#watching) |
| `--staged` | Compare HEAD with the index |
| `--files` | Compare two files or two directories on disk, see [As git difftool](#as-git-difftool) |
| `--max-file-size <size>` | Largest side of a file to load, in bytes or with `K`, `M` or `G` (default `8M`) |
| `-v`, `--verbose` | Log at debug level |
| `--pprof <file>` | Write a heap profile to this file when seam ends |
| `--version` | Print the version, and a newer release if the last check found one |

The log goes to `$XDG_STATE_HOME/seam/seam.log` (else
`~/.local/state/seam/seam.log`), never to the terminal. The language
servers' own output lies next to it.

## As git difftool

`seam --files <left> <right>` compares two files or two directories on
disk, git or not. Two directories show the files that differ in the file
list; `/dev/null` stands for a side that does not exist. To have
`git difftool` open seam:

```bash
git config --global difftool.seam.cmd 'seam --files "$LOCAL" "$REMOTE"'
git config --global diff.tool seam
git config --global difftool.prompt false
```

Then `git difftool HEAD~1` opens seam once per changed file, and
`git difftool -d HEAD~1` once for all of them, with the file list.

Hovers and definitions work on the right side where it is a file of the
worktree of the repository seam runs in, as it is when git difftool
compares with the worktree. A file elsewhere, such as the old side git
writes to a temporary file, shows its diff and colours only. Outside a
repository seam needs no git at all. `--watch` does not apply.

## The screen

The header names the file, the view and the number of differences. The
footer lists the main keys and, on the right, the language server of the
file and its progress while it indexes. The file list on the left groups the
changed files by folder, with the lines added and removed; it opens by
itself when the terminal is wide enough for it and both sides.

Every key below is a default; [Configuration](configuration.md#keys) changes
them.

## Moving around

| Key | Does |
|---|---|
| `j` / `k`, arrows | Scroll a line |
| `ctrl+d` / `ctrl+u` | Half a page |
| `space`, `pgdown` / `pgup` | A page |
| `g`, `home` / `G`, `end` | Top, bottom |
| `h` / `l`, left / right | Scroll sideways, for long lines |
| `n` / `p` | Next, previous change |
| `tab`, `]` / `shift+tab`, `[` | Next, previous file |
| `r` | Reload: the changed files and the revisions anew |
| `q`, `esc`, `ctrl+c` | Quit |

The mouse wheel scrolls what is under it: the diff, the file list or a
popup. With `shift`, or the wheel's own sideways buttons, it scrolls
sideways. A click on a fold (`⋯ 156 unchanged lines`) opens it.

## Views

| Key | Does |
|---|---|
| `f` | Only the changed hunks, or the whole file |
| `u` | Side by side, or one column (unified) |
| `s` | One file at a time, or all files in one stream |

seam starts with the hunks of one file. Below 120 columns of diff it shows
one column instead of two sides; `u` decides for the rest of the session.
In the unified view, `o` in the caret goes between the old and the new half
of a changed line.

In the stream, all changed files follow each other, each under its own
header; `f` and `u` apply to all of them. Switching between the stream and
one file keeps the line at the top.

## The file list

`b` opens the list, or gives an open one the focus; on a focused list it
closes it. While it has the focus, the motion keys move in it, `enter` opens
the file, `[` / `]` go to the previous and next folder, and `esc` gives the
focus back. A click on a file opens it, whether the list has the focus or
not; a click in the diff takes the focus from it.

## Hover

Rest the mouse on a symbol, and after 300 ms a popup shows what the
language server says about it: its type, signature and documentation, with
the code in colour. The wheel scrolls a long popup; moving away or `esc`
closes it. Hover works on both sides, also on the old side of a commit:
seam checks the commit out for the server, see
[Language servers](language-servers.md#comparing-commits).

Without a mouse, use the caret.

## The caret

`i` puts a caret on the screen; `i` or `esc` takes it away.

| Key | Does |
|---|---|
| `h` `j` `k` `l`, arrows | Move by a character or line |
| `w` / `b` / `e` | Next word, previous word, end of the word |
| `o` | The other side of the line |
| `K` | Hover at the caret |
| `d` | Go to the definition |

## Definitions

`ctrl` + click, or `d` in the caret, goes to the definition of a symbol.
`ctrl+o` goes back, as often as you went.

- A definition in a changed file shows that file in full at its line; in
  the stream, the stream scrolls there.
- A definition in another file of the worktree shows it, read only, with
  only its new side.
- A definition outside the repository, such as in the standard library or a
  module cache, is named in the footer.

## Watching

`--watch` reloads as `r` does whenever a file of the worktree, the index or
HEAD changes, once the changes have been quiet for 200 ms but no later than
a second after the first; with `--staged` only the index and HEAD count. The
header says `watch` while it runs. Both `r` and `--watch` resolve the
revisions anew, so after a commit or a checkout the diff is against the new
HEAD.

Directories git ignores are not watched, also those made after the start.
On Linux each watched directory takes one of the user's inotify watches;
when they run out, the footer says so, seam goes on without watching, and
`fs.inotify.max_user_watches` is the setting to raise. Two commits never
change, so `--watch` with two of them is an error.

## Limits

A side larger than `--max-file-size` shows as too large, a binary file as
binary, instead of a diff. Syntax colours stop at a line longer than 64 KiB
and for the rest of a file that takes too long to colour; the diff stays
complete.

## Newer releases

Once a day at most, while the viewer is open, seam asks GitHub's public
release API for its latest release. If it is newer, seam names it when it
ends, with how to update this install:

```
seam v0.2.0 is out (you have v0.1.0): brew upgrade seam
```

The request carries nothing but `seam/<version>` as its User-Agent. A failed
or slow one says nothing and does not delay seam; the next try is a day
later. Builds that are no release, such as `dev` or a build between tags,
never ask. The last answer is kept in `$XDG_STATE_HOME/seam/release.json`.

Turn the check off with `check = false` under [`[updates]`](configuration.md#updates)
or with `SEAM_NO_UPDATE_CHECK=1`. `SEAM_UPDATE_URL` asks another server in
GitHub's place, such as a mirror, at the same path.
