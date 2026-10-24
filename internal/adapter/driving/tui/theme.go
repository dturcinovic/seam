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
	popupStyle
	popupFrameStyle
	// The styles of the popup's text; its code has one per syntax kind,
	// see popupCodeStyle.
	popupEmphasisStyle
	popupStrongStyle
	popupLinkStyle
	popupDimStyle
	popupCodeSpanStyle
	// caretStyle is the grapheme under the caret, in any tone.
	caretStyle
	// The stand-in styles dim replaced graphemes in the text above.
	plainStandIn
	headerStandIn
	footerStandIn
	errorStandIn
	popupStandIn
	// The scrollbar styles come in one pair per tone, see scrollStyle; the
	// file list's follow, see listStyle.
	firstScrollStyle
)

const (
	firstListStyle      = firstScrollStyle + style(2*tones)
	firstPaneStyle      = firstListStyle + style(int(listParts)*int(listShades))
	firstPopupCodeStyle = firstPaneStyle + style(int(tones)*int(roles))
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
	case popupStyle, popupEmphasisStyle, popupStrongStyle, popupLinkStyle, popupDimStyle, popupCodeSpanStyle:
		return popupStandIn
	case separatorStyle, foldStyle, popupFrameStyle, caretStyle, plainStandIn, headerStandIn, footerStandIn, errorStandIn, popupStandIn,
		firstScrollStyle, firstListStyle, firstPaneStyle, firstPopupCodeStyle:
	}
	if st >= firstPopupCodeStyle && st < firstPopupCodeStyle+style(syntax.Kinds) {
		return popupStandIn
	}
	// A path in the file list dims its stand-ins in the row's own colours.
	if sh, ok := st.listShade(); ok {
		return listStyle(listDimPart, sh)
	}
	return st
}

// scrollStyle is a scrollbar cell marked in tone t, on the thumb if
// onThumb; equalTone marks nothing.
func scrollStyle(t tone, onThumb bool) style {
	st := firstScrollStyle + style(2*t)
	if onThumb {
		st++
	}
	return st
}

// listPart is a part of a line of the file list, coloured by what it says.
type listPart uint8

const (
	listNamePart listPart = iota
	listAddedPart
	listDeletedPart
	listChangedPart
	listDimPart
	listDirPart
	listGutterPart
	listZeroPart
	listParts
)

// listShade is how a file of the list stands out: selectedShade marks the
// cursor of a focused list or else the file shown, currentShade the file
// shown while the cursor is elsewhere.
type listShade uint8

const (
	plainShade listShade = iota
	selectedShade
	currentShade
	listShades
)

// listStyle is part p of a line in shade sh.
func listStyle(p listPart, sh listShade) style {
	return firstListStyle + style(int(p)*int(listShades)+int(sh))
}

// listShade is the shade of a list style, the inverse of listStyle; ok is
// false for any other style.
func (st style) listShade() (sh listShade, ok bool) {
	if st < firstListStyle || st >= firstPaneStyle {
		return 0, false
	}
	return listShade((st - firstListStyle) % style(listShades)), true
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

const styles = int(firstPopupCodeStyle) + int(syntax.Kinds)

// A style must fit its type; more kinds or tones fail the build here.
const _ = uint8(styles - 1)

func paneStyle(t tone, r role) style {
	return firstPaneStyle + style(int(t)*int(roles)+int(r))
}

// popupCodeStyle is code of kind k in the popup.
func popupCodeStyle(k syntax.Kind) style {
	return firstPopupCodeStyle + style(k)
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
	thumbBg          = "#4a4a4a"
	popupBg          = "#21252b"
	codeSpanBg       = "#2c313a"
	errorFg          = "#ff6b6b"
	// The file list's selection, folders, gutter and zero counts.
	selectedBg = "#2e436e"
	selectedFg = "#ffffff"
	dirFg      = "#56a8f5"
	gutterFg   = "#393b40"
	zeroFg     = "#4a4d52"
	// The marks of the scrollbar, bright enough to find on the thumb.
	deletedFg  = "#e06c75"
	insertedFg = "#98c379"
	changedFg  = "#61afef"
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

// addPopupStyles adds the styles of the popup's text on its background.
func addPopupStyles(all map[style]lipgloss.Style, popup lipgloss.Style) {
	all[popupEmphasisStyle] = popup.Italic(true)
	all[popupStrongStyle] = popup.Bold(true)
	all[popupLinkStyle] = popup.Foreground(lipgloss.Color(changedFg)).Underline(true)
	all[popupDimStyle] = popup.Foreground(lipgloss.Color(dimFg))
	all[popupCodeSpanStyle] = popup.Background(lipgloss.Color(codeSpanBg))
	for k, fg := range kindColours() {
		all[popupCodeStyle(syntax.Kind(k))] = popup.Foreground(lipgloss.Color(fg))
	}
}

func lipglossStyles() map[style]lipgloss.Style {
	base := lipgloss.NewStyle()
	dim := base.Foreground(lipgloss.Color(dimFg))
	header := base.Bold(true).Background(lipgloss.Color(headerBg))
	errorText := base.Foreground(lipgloss.Color(errorFg))
	popup := base.Background(lipgloss.Color(popupBg))
	all := map[style]lipgloss.Style{
		headerStyle:     header,
		footerStyle:     dim,
		errorStyle:      errorText,
		separatorStyle:  dim,
		foldStyle:       dim.Faint(true),
		plainStandIn:    base.Faint(true),
		headerStandIn:   header.Faint(true),
		footerStandIn:   dim.Faint(true),
		errorStandIn:    errorText.Faint(true),
		popupStyle:      popup,
		popupFrameStyle: popup.Foreground(lipgloss.Color(dimFg)),
		popupStandIn:    popup.Faint(true),
		caretStyle:      base.Reverse(true),
	}
	addPopupStyles(all, popup)
	shades := [listShades]lipgloss.Style{
		plainShade:    base,
		selectedShade: base.Background(lipgloss.Color(selectedBg)),
		currentShade:  base.Bold(true),
	}
	parts := [listParts]string{
		listAddedPart: insertedFg, listDeletedPart: deletedFg, listChangedPart: changedFg, listDimPart: dimFg,
		listDirPart: dirFg, listGutterPart: gutterFg, listZeroPart: zeroFg,
	}
	for sh, shade := range shades {
		for p, fg := range parts {
			all[listStyle(listPart(p), listShade(sh))] = shade.Foreground(lipgloss.Color(fg))
		}
	}
	// The name of the selected file is white, as the design has it.
	all[listStyle(listNamePart, selectedShade)] = shades[selectedShade].Foreground(lipgloss.Color(selectedFg))
	marks := [tones]string{deletedTone: deletedFg, insertedTone: insertedFg, changedTone: changedFg}
	for t, fg := range marks {
		track := base.Foreground(lipgloss.Color(fg))
		all[scrollStyle(tone(t), false)] = track
		all[scrollStyle(tone(t), true)] = track.Background(lipgloss.Color(thumbBg))
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
