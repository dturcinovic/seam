package tui

import (
	"path"

	"seam/internal/domain/change"
	"seam/internal/domain/hover"
	"seam/internal/domain/lang"
)

// lspState is what the footer tells of the language server of the file
// shown. Statuses come per instance, whether its file is shown or not, so
// one shown again has its status at once.
type lspState struct {
	// Shown is the instance that serves the file shown; zero for none,
	// which has no status.
	Shown  lang.Instance
	Status map[lang.Instance]hover.ServerStatus
}

// track tells the server of the file just shown: its right side, or that
// the text it may have had is no longer on screen, as when the file turned
// binary or too large.
func (m *model) track() {
	if d := m.s.Snapshot.RightDoc; d.Path != "" {
		m.s.LSP.Shown, _ = m.p.Docs.Track(d)
		return
	}
	m.s.LSP.Shown = lang.Instance{}
	if path := m.s.Snapshot.File.NewPath; path != "" {
		m.p.Docs.Untrack(path)
	}
}

// gone are the paths of the right sides in before that after has not: a
// server must not keep their text, which a revert has made old.
func gone(before, after []change.ChangedFile) []string {
	kept := make(map[string]bool, len(after))
	for _, f := range after {
		kept[f.NewPath] = true
	}
	var out []string
	for _, f := range before {
		if f.NewPath != "" && !kept[f.NewPath] {
			out = append(out, f.NewPath)
		}
	}
	return out
}

// update takes a status change; answers are for the popup.
func (l *lspState) update(u hover.Update) {
	if u.Kind != hover.StatusChanged {
		return
	}
	if l.Status == nil {
		l.Status = make(map[lang.Instance]hover.ServerStatus)
	}
	l.Status[u.Instance] = u.Status
}

// lspText is the footer's word on the server of the file shown, as
// "gopls starting…", or "no LSP for .go" when it is not installed; empty
// when no server is meant to serve the file.
func lspText(s *state) string {
	l := &s.LSP
	name := shownText(l.Shown.Server)
	switch l.Status[l.Shown] {
	case hover.Missing:
		return "no LSP for " + kindOf(s.Snapshot.RightDoc)
	case hover.Starting:
		return name + " starting…"
	case hover.Ready:
		return name
	case hover.ServerFailed:
		return name + " failed"
	case hover.Stopped:
		return name + " stopped"
	}
	return ""
}

// kindOf names the kind of file d is: its extension, else its language.
func kindOf(d hover.Document) string {
	if ext := path.Ext(d.Path); ext != "" {
		return shownText(ext)
	}
	return string(d.Lang)
}
