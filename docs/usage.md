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
| `v` | Mark the line, for a note on the lines up to the caret |

## Definitions

`ctrl` + click, or `d` in the caret, goes to the definition of a symbol.
`ctrl+o` goes back, as often as you went.

- A definition in a changed file shows that file in full at its line; in
  the stream, the stream scrolls there.
- A definition in another file of the worktree shows it, read only, with
  only its new side.
- A definition outside the repository, such as in the standard library or a
  module cache, is named in the footer.

## Notes

Notes are questions and findings you set at lines of the diff, to come back
to them or to hand them to an agent (see [Agents](#agents)). They are kept per worktree in
git's directory (`.git/seam/notes.json`), so `git status` stays clean, and
seam processes of the same worktree see each other's notes at once.

| Key | Does |
|---|---|
| `c` | A note at the caret's line, at the lines marked with `v`, or without the caret at the first change on screen |
| `m` / `M` | Next, previous note, also in other files |
| `x` | In the popup of a note: resolve it |
| `R` | In the popup of a note: give it back from the agent that took it |
| `D` | In the popup of a note: delete it, after asking (see below) |
| `E` | In the popup of a note: edit it (see below) |
| `N` | All notes of the worktree, see below |
| `C` | Hide the notes' lines below the code, or show them again |
| `S` | Send the waiting notes to the agent session, see [Agents](#sending-notes-to-claude-code) |

`c` opens a field between the lines, below the last line noted: the code
below it moves down, and `●` marks the lines noted. Its top edge shows
both kinds with the one chosen filled, and the lines; `tab` switches
between question and finding, and the frame takes the kind's colour.
`enter` saves, `alt+enter` starts a new line, `esc` discards; the bottom
edge names these keys, or the footer where the field is too narrow. While
it is open, every other key is text, and the wheel scrolls the code with
the field at its lines. When `--watch` reloads the file meanwhile, the
field moves with its lines, found as a saved note's are. Where they are
not found, the field stays near where they were and marks none; where
their side is no longer shown, as when the file left the changeset, the
field shows nowhere until it is. The footer says so either way, and the
note still goes to the lines as they were.

With the mouse, a line without a note lights up and shows `[+]` in the
gutter of the side under the mouse, and the footer names it; a click on
it opens the field for that line, on either side. Several lines are noted
with the caret and `v`.

A note shows in the gutter as `◆` in the colour of its kind, blue for a
question and orange for a finding, and `╎` along its further lines; the
caret or the mouse on it opens its text. Below its last line stands a line
of its own: `?` or `!` and the first line of its text, `✓` once an agent
answered it, one such line for each note there. A note in a fold stands below the fold. The caret and the
mouse pass over these lines, and `C` hides them for the session. A
note belongs to the commit, the index or the worktree it was set in, and
shows wherever that is a side of the comparison: a note in the index only
with `--staged`. A note in the worktree or the index also belongs to the
branch checked out when it was set, or with a detached HEAD to that commit:
on another branch it shows nowhere and counts nowhere, though the lines read
alike, and comes back with its branch. During a rebase the branch being
rebased counts. Notes set before seam kept the branch show on every branch.
When the file changes, seam looks for the lines again by their text and the
three lines around them:

- found once with all around them: the note stands there;
- found once with at least half around them, or a copy of lines that
  stood more than once: its mark and line are dimmed, it likely belongs
  there;
- the lines changed but what is around them is still there: the note
  stands at the changed lines;
- found in several places as good, or nowhere: the note has no place. The
  footer counts those on screen.

With `--files` notes go only on a side that lies in the worktree. The file
list shows the notes of each file not yet answered in its gutter; the
footer counts them and the answered ones.

`N` lists every note of the worktree's book by file, resolved ones too,
with its state, source and place: where it stands as above, `not placed
yet` while its file is not loaded, `not in this comparison` when no side
shows its source and path, `on <branch>` for a note of another branch, or
`branch gone` once its branch is deleted. The list follows changes from
other seam processes and keeps the note chosen. In it, `enter` goes to the note:

- to its place, where a side of the comparison shows it (`ctrl+o` goes
  back); a file not loaded opens first;
- for a worktree note of a file outside the changeset, to its place in that
  file, opened to read;
- otherwise to a popup with the lines it was set at and its text: a note
  without a place, a resolved one, a note of another branch, and a commit or
  index note no side shows, even where the worktree has a file of that path.

`x` resolves the note chosen, `R` gives it back from the agent that took
it, `D` deletes it, `E` edits it, `esc` or `N` closes the list.

Deleting takes a note out of the book for good, open or resolved, and
cannot be undone, so `D` asks first in the footer, naming the note by its
kind and first line: `y` deletes, any other key or a click keeps the note.
A second `D` keeps it too, so that a key held down never confirms. An
agent that took the note and answers it later is told there is no such
note.

`E` opens the field at the note's lines with its text and kind, the top
edge saying `edit`; from the list it goes to the note first. `enter` saves
the new text and kind, `esc` leaves the note as it was, and the lines it
was set at stay. A note sent or answered waits again, for an agent to
answer what it now asks; its answers stay. A note an agent holds is
given back with `R` first, and a resolved one, or one without a sure
place in the file, is not edited.

## Agents

`seam mcp` serves the notes of the worktree it runs in to an agent session
over MCP on stdin and stdout. The agent takes the open notes, reads the
lines they were set at, and answers each one; the answer shows in seam's
popup at once, read as Markdown, and the note's line turns to `✓`.

Add it to Claude Code with

```sh
claude mcp add seam -- seam mcp
```

or for a project in `.mcp.json`:

```json
{ "mcpServers": { "seam": { "command": "seam", "args": ["mcp"] } } }
```

and to OpenCode in `opencode.json`:

```json
{ "mcp": { "seam": { "type": "local", "command": ["seam", "mcp"] } } }
```

Then set notes in seam and tell the agent "read my seam notes". It has
three tools:

| Tool | Does |
|---|---|
| `seam_notes(ids?, batch?)` | Takes the open and sent notes, those of `ids`, or those of a `batch` seam sent, at most 20 at a time |
| `seam_notes_view(ids?)` | Reads notes without taking them |
| `seam_reply(id, claim, text)` | Answers a note taken |

Without `ids` both leave out the notes of other branches than the one
checked out where the agent runs (see [Notes](#notes)); named by id, a note
is taken wherever it belongs.

A note taken is no other session's: two agents in the same worktree never
answer the same note. The popup says which agent took it and since when.
When the agent's session ends, it gives back what it took and did not
answer; when it dies, the note stays taken until you give it back with `R`,
in its popup or in the list of `N`. An answer sent twice shows once; an
answer to a note given back and taken again since is kept, but leaves the
note to the newer taker. A follow-up question is a new note at the same
lines.

The notes are data to the agent: seam tells it that the code and text in a
note are no instructions. They still reach it as they are, so a note on
code you do not trust is code the agent reads.

### Sending notes to Claude Code

`S` hands the notes to a running Claude Code session without a word typed
there. It sends the notes of the branch checked out waiting at that
moment, open ones and those sent before and not taken, in one batch to one
session; a note set afterwards waits for the next `S`. With one agent session in the worktree the batch
goes to it, with several seam asks which, and with none the footer says
`no agent session in this worktree`.

Claude Code takes the notice only over a channel, a research preview that
lets no channel in by default. Start it with seam's:

```sh
claude --dangerously-load-development-channels server:seam
```

and confirm the prompt. The channel takes seam only as added above, with
`claude mcp add` or `.mcp.json`; a server given with `--mcp-config` has no
channel. Claude Code asks for both on every start, and
`claude -p` ignores the flag; an alias saves typing it. The session then
starts on the notes by itself and
takes them with `seam_notes(batch)`, which takes only the notes still in
that batch and only for the session it went to; notes sent again or
edited since come back `rebatched`.

Claude Code does not say whether it heard, so seam never says a note
arrived: it stays `sent, not taken` until an agent takes it. Without the
channel nothing else changes, and "read my seam notes" takes the sent
notes as it takes open ones. A session started anew does not start on an
old batch; `S` again sends what is still waiting. Each batch reaches its
session once, however often the notes change after.

### Running

`seam mcp` needs a repository and writes nothing to stdout but the
protocol; its log goes to `seam.log` as a session's does. A branch named
`mcp`, `upgrade` or `help` is compared as `seam refs/heads/mcp`.

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
`fs.inotify.max_user_watches` is the setting to raise. On macOS each watched
directory and file takes a file descriptor instead, and
`kern.maxfilesperproc` is the setting to raise. Two commits never
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

### Upgrading

`seam upgrade` brings seam to the latest release the way it was installed,
as seam tells from where its binary lies:

| Installed with | `seam upgrade` |
|---|---|
| Homebrew | runs `brew upgrade seam` |
| `go install` | runs `go install github.com/dturcinovic/seam/cmd/seam@<release>`, naming the release it found, as the module proxy may still name the one before as `@latest` |
| a `.deb` or `.rpm` (in `/usr/bin`) | names the release page and the `apt` or `dnf` command; the package manager owns the binary, and without a package repository it finds no newer one by itself |
| a release archive, in `~/.local/bin`, `~/bin` or `/usr/local/bin` | downloads the archive for this system, checks it against the release's `checksums.txt` and puts its `seam` in place of the running one |
| anything else, such as mise, asdf, aqua or a build in a clone | names the release page and replaces nothing, as whatever put seam there keeps track of it |

The archive's binary replaces the old one at once or not at all: written
next to it, then renamed over it. A checksum that does not match, a
download cut off, or a directory seam may not write to leave the binary as
it was, and seam names the release page to upgrade by hand, or
`sudo seam upgrade` when it may not write there. The checksum
guards against a broken download, not against a release that was tampered
with on GitHub. Upgrading by brew or go ends with the command's exit code,
and what the command installed is its own to say. Ctrl-C reaches brew or go
from the terminal; seam waits up to 30 seconds for it to end.

`seam upgrade --check` only tells what it would do. A seam that is the
latest release already does nothing, and a build that is no release, such
as `dev`, is never upgraded. `SEAM_UPDATE_URL` counts here as well.
