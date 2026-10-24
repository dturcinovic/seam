package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"seam/internal/domain/syntax"
)

// style names what a stretch of the screen shows; the theme gives it its
// colours. The pane styles come in one block per tone, see paneStyle.
type style uint8

const (
	plain style = iota
	headerStyle
	footerStyle
	errorStyle
	separatorStyle
	foldStyle
	// The stand-in styles dim replaced graphemes in the text above.
	plainStandIn
	headerStandIn
	footerStandIn
	errorStandIn
	firstPaneStyle
)

// standIn is how a replaced grapheme shows in text of style st. The pane
// styles have their own, see standInRole.
func (st style) standIn() style {
	switch st {
	case plain:
		return plainStandIn
	case headerStyle:
		return headerStandIn
	case footerStyle:
		return footerStandIn
	case errorStyle:
		return errorStandIn
	case separatorStyle, foldStyle, plainStandIn, headerStandIn, footerStandIn, errorStandIn, firstPaneStyle:
	}
	return st
}

// tone is the colour of one side of a row: what happened to its line.
type tone uint8

const (
	equalTone tone = iota
	deletedTone
	insertedTone
	changedTone
	// absentTone is the side of a row that has no line.
	absentTone
	tones
)

// role is a part of a pane line in its tone. Text comes in one role per
// syntax kind, see textStyle.
type role uint8

const (
	gutterRole role = iota
	// standInRole is a replaced grapheme, inlineStandInRole one within a
	// changed stretch; both are dimmed rather than coloured by kind.
	standInRole
	inlineStandInRole
	textRole
	// inlineRole is a changed stretch within a changed line.
	inlineRole = textRole + role(syntax.Kinds)
	roles      = inlineRole + role(syntax.Kinds)
)

const styles = int(firstPaneStyle) + int(tones)*int(roles)

// A style must fit its type; more kinds or tones fail the build here.
const _ = uint8(styles - 1)

func paneStyle(t tone, r role) style {
	return firstPaneStyle + style(int(t)*int(roles)+int(r))
}

// textStyle is text of kind k in tone t, in a changed stretch if inline.
func textStyle(t tone, k syntax.Kind, inline bool) style {
	r := textRole
	if inline {
		r = inlineRole
	}
	return paneStyle(t, r+role(k))
}

// theme holds the SGR sequences that switch each style on and off. Its zero
// value has none: everything comes out uncoloured.
type theme struct {
	open, close [styles]string
}

// The palette, for a dark background.
const (
	deletedBg        = "#3f2326"
	deletedStrongBg  = "#7a2e35"
	insertedBg       = "#213a28"
	insertedStrongBg = "#2f6b3e"
	changedBg        = "#1f2f47"
	changedStrongBg  = "#2f4f80"
	absentBg         = "#262626"
	dimFg            = "#7a7a7a"
	headerBg         = "#303030"
	errorFg          = "#ff6b6b"
)

// kindColours are the foregrounds of the syntax kinds, readable on every
// tone's background. An empty colour is the terminal's own.
func kindColours() [syntax.Kinds]string {
	return [syntax.Kinds]string{
		syntax.Keyword:   "#c678dd",
		syntax.Type:      "#e5c07b",
		syntax.Function:  "#61afef",
		syntax.Builtin:   "#56b6c2",
		syntax.Constant:  "#d19a66",
		syntax.String:    "#98c379",
		syntax.Escape:    "#56b6c2",
		syntax.Number:    "#d19a66",
		syntax.Comment:   "#7f848e",
		syntax.Meta:      "#e06c75",
		syntax.Operator:  "#56b6c2",
		syntax.Tag:       "#e06c75",
		syntax.Attribute: "#d19a66",
		syntax.Variable:  "#e06c75",
	}
}

// newTheme derives the sequences from Lip Gloss styles once: Lip Gloss
// renders a whole string per call, while a line switches style per stretch
// of cells and writes them itself.
func newTheme() *theme {
	var t theme
	for st, ls := range lipglossStyles() {
		// The placeholder is no part of any SGR sequence.
		t.open[st], t.close[st], _ = strings.Cut(ls.Render("x"), "x")
	}
	return &t
}

func lipglossStyles() map[style]lipgloss.Style {
	base := lipgloss.NewStyle()
	dim := base.Foreground(lipgloss.Color(dimFg))
	header := base.Bold(true).Background(lipgloss.Color(headerBg))
	errorText := base.Foreground(lipgloss.Color(errorFg))
	all := map[style]lipgloss.Style{
		headerStyle:    header,
		footerStyle:    dim,
		errorStyle:     errorText,
		separatorStyle: dim,
		foldStyle:      dim.Faint(true),
		plainStandIn:   base.Faint(true),
		headerStandIn:  header.Faint(true),
		footerStandIn:  dim.Faint(true),
		errorStandIn:   errorText.Faint(true),
	}
	// An empty colour is none: equal lines keep the terminal's background.
	backgrounds := [tones][2]string{
		equalTone:    {"", ""},
		deletedTone:  {deletedBg, deletedStrongBg},
		insertedTone: {insertedBg, insertedStrongBg},
		changedTone:  {changedBg, changedStrongBg},
		absentTone:   {absentBg, absentBg},
	}
	for t, bg := range backgrounds {
		normal, strong := base.Background(lipgloss.Color(bg[0])), base.Background(lipgloss.Color(bg[1]))
		all[paneStyle(tone(t), gutterRole)] = normal.Foreground(lipgloss.Color(dimFg))
		all[paneStyle(tone(t), standInRole)] = normal.Faint(true)
		all[paneStyle(tone(t), inlineStandInRole)] = strong.Faint(true)
		for k, fg := range kindColours() {
			all[textStyle(tone(t), syntax.Kind(k), false)] = normal.Foreground(lipgloss.Color(fg))
			all[textStyle(tone(t), syntax.Kind(k), true)] = strong.Foreground(lipgloss.Color(fg))
		}
	}
	return all
}
