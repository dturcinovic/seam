package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dturcinovic/seam/internal/domain/change"
	"github.com/dturcinovic/seam/internal/domain/filediff"
	"github.com/dturcinovic/seam/internal/domain/ui"
	"github.com/dturcinovic/seam/internal/domain/view"
)

// hint is an entry of the footer's reminder of the keys: the first key of
// each of its actions, or every key if every is set, joined by a slash,
// then what they do.
type hint struct {
	actions []ui.Action
	does    string
	every   bool
}

func firstKeys(does string, actions ...ui.Action) hint { return hint{actions: actions, does: does} }

func everyKey(does string, a ui.Action) hint {
	return hint{actions: []ui.Action{a}, does: does, every: true}
}

// The reminders of the view's keys, of those the file list takes while it
// has the focus, and of those of the caret.
func viewHints() []hint {
	return []hint{
		// The layout early: a narrow footer is cut, and the narrow screen
		// is where seam turns unified by itself.
		firstKeys("quit", ui.Quit), firstKeys("reload", ui.Reload), firstKeys("full/hunks", ui.ToggleView),
		firstKeys("unified/split", ui.ToggleLayout),
		firstKeys("change", ui.NextChange, ui.PrevChange), firstKeys("file", ui.NextFile, ui.PrevFile),
		firstKeys("stream", ui.ToggleStream),
		firstKeys("scroll", ui.Down, ui.Up), firstKeys("sideways", ui.ScrollLeft, ui.ScrollRight),
		firstKeys("files", ui.ToggleFiles), firstKeys("caret", ui.StartCaret), firstKeys("back", ui.JumpBack),
	}
}

func listHints() []hint {
	return []hint{
		firstKeys("open", ui.Open), firstKeys("back", ui.Back), firstKeys("move", ui.Down, ui.Up),
		firstKeys("folder", ui.PrevFolder, ui.NextFolder), firstKeys("first/last", ui.Top, ui.Bottom),
		firstKeys("close", ui.ToggleFiles), firstKeys("quit", ui.Quit),
	}
}

func caretHints() []hint {
	return []hint{
		firstKeys("hover", ui.Hover), firstKeys("definition", ui.Definition), firstKeys("move", ui.CaretLeft, ui.CaretDown, ui.CaretUp, ui.CaretRight),
		firstKeys("word", ui.WordForward, ui.WordBack, ui.WordEnd), firstKeys("other side", ui.OtherSide),
		everyKey("leave", ui.LeaveCaret),
		firstKeys("change", ui.NextChange, ui.PrevChange), firstKeys("file", ui.NextFile, ui.PrevFile),
		firstKeys("quit", ui.Quit),
	}
}

// keyTexts are the reminders of the keys of the view, the focused file
// list and the caret.
type keyTexts struct {
	view, list, caret string
}

func newKeyTexts(k ui.Keymap) keyTexts {
	return keyTexts{
		view:  keyText(k, ui.ViewLayer, viewHints()),
		list:  keyText(k, ui.ListLayer, listHints()),
		caret: keyText(k, ui.CaretLayer, caretHints()),
	}
}

// keyText is the reminder of hints with the keys of k, as they act while
// layer l is on top: a key l takes for another action drops out. An
// action without a key drops out, and so does an entry left with none.
func keyText(k ui.Keymap, l ui.Layer, hints []hint) string {
	var b strings.Builder
	for _, h := range hints {
		var keys []string
		for _, a := range h.actions {
			ks := slices.DeleteFunc(k.Keys(a), func(key string) bool {
				other, ok := k.Action(l, key)
				return ok && other != a
			})
			if !h.every {
				ks = ks[:min(len(ks), 1)]
			}
			keys = append(keys, ks...)
		}
		if len(keys) == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("  ")
		}
		b.WriteString(strings.Join(keys, "/") + " " + h.does)
	}
	return b.String()
}

// shownText is how a path or part of one shows: textcell replaces all it
// may hold, but line breaks show as ␤ in names, as the plan has it.
func shownText(p string) string {
	return strings.ReplaceAll(p, "\n", "␤")
}

// pathText is how the headers name f: a file renamed or copied names where
// it comes from too.
func pathText(f change.ChangedFile) string {
	if f.OldPath != "" && f.NewPath != "" && f.OldPath != f.NewPath {
		return shownText(f.OldPath) + " → " + shownText(f.NewPath)
	}
	return shownText(f.Path())
}

// header names the file, where it stands in the list, the view and how many
// differences it has, as IntelliJ's diff does.
func header(s *state) string {
	f, ok := s.file()
	if !ok {
		return "no changes"
	}
	// A file visited has no place in the list, no layout to choose and no
	// differences to count.
	visit := s.visiting()
	var h string
	if visit {
		h = fmt.Sprintf("visit  %s  not changed", shownText(f.Path()))
	} else {
		h = fmt.Sprintf("%d/%d  %s %s  %s", s.Current+1, len(s.Files), f.Status, pathText(f), modeName(s.Mode))
		if s.Layout.Unified {
			h += " unified"
		}
	}
	if s.Stream.On {
		h += "  stream"
	}
	switch {
	case s.Watching:
		h += "  watch"
	case s.WatchFailed:
		h += "  watch failed"
	}
	snap, loaded := s.loadedOf(s.Current)
	switch {
	case !loaded && s.loadErr(s.Current) != nil:
		return h + "  failed"
	case !loaded:
		return h + "  loading…"
	}
	if visit {
		return h + noticesText(snap.Notices)
	}
	return h + fmt.Sprintf("  %d differences", len(snap.Diff.Changes)) + noticesText(snap.Notices)
}

// noticesText is the notices, each after two blanks.
func noticesText(ns []filediff.Notice) string {
	var b strings.Builder
	for _, n := range ns {
		b.WriteString("  " + noticeText(n))
	}
	return b.String()
}

// placeholder stands in the first line when there are no lines to show.
func placeholder(s *state) string {
	switch {
	case len(s.Files) == 0:
		return "nothing to compare"
	case !s.Loaded:
		return ""
	case len(s.Snapshot.Notices) > 0 && len(s.Snapshot.Diff.Rows) == 0:
		return "not shown as text"
	}
	return "no lines"
}

// footer is the last error, or else the keys.
func footer(s *state) (string, style) {
	if s.Err != nil {
		return "error: " + s.Err.Error(), errorStyle
	}
	if s.Note != "" {
		return s.Note, footerStyle
	}
	if s.Field.Open {
		if s.Field.Hint != "" {
			return fieldHints + "  " + s.Field.Hint, footerStyle
		}
		return fieldHints, footerStyle
	}
	notes := s.notesText()
	if s.List.Focus {
		return notes + s.KeyText.list, footerStyle
	}
	if s.Caret.On {
		return notes + s.markText() + s.KeyText.caret, footerStyle
	}
	return notes + s.KeyText.view, footerStyle
}

func modeName(m view.Mode) string {
	if m == view.Full {
		return "full file"
	}
	return "hunks"
}

func noticeText(n filediff.Notice) string {
	side := ""
	switch n.Side {
	case filediff.OldSide:
		side = "old side "
	case filediff.NewSide:
		side = "new side "
	}
	switch n.Kind {
	case filediff.TooLarge:
		return fmt.Sprintf("%stoo large (%d bytes)", side, n.Size)
	case filediff.Binary:
		return side + "binary"
	case filediff.Simplified:
		return "diff simplified"
	case filediff.HighlightStopped:
		return fmt.Sprintf("%sno syntax colours from line %d", side, n.Line)
	}
	return ""
}
