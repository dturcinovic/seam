package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/dturcinovic/seam/internal/domain/syntax"
	"github.com/dturcinovic/seam/internal/domain/ui"
)

// style names what a stretch of the screen shows; the theme gives it its
// colours. The pane styles come in one block per tone, see paneStyle.
type style uint16

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
	firstListStyle      = firstScrollStyle + style(2*lineTones)
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
	// The hover tones are the four above on the line under the mouse; a
	// side without a line is never under it.
	equalHoverTone
	deletedHoverTone
	insertedHoverTone
	changedHoverTone
	tones
)

// lineTones are the tones of a row's side as the diff gives them.
const lineTones = absentTone + 1

// hovered is t on the line under the mouse; a hover tone stays itself.
func (t tone) hovered() tone {
	if t < absentTone {
		return equalHoverTone + t
	}
	return t
}

func (t tone) isHover() bool { return t >= equalHoverTone }

// role is a part of a pane line in its tone. Text comes in one role per
// syntax kind, see textStyle.
type role uint8

const (
	gutterRole role = iota
	// standInRole is a replaced grapheme, inlineStandInRole one within a
	// changed stretch; both are dimmed rather than coloured by kind.
	standInRole
	inlineStandInRole
	// noteRole is a note's mark in the gutter.
	noteRole
	textRole
	// inlineRole is a changed stretch within a changed line.
	inlineRole = textRole + role(syntax.Kinds)
	roles      = inlineRole + role(syntax.Kinds)
)

const styles = int(firstPopupCodeStyle) + int(syntax.Kinds)

// A style must fit its type; more kinds or tones fail the build here.
const _ = uint16(styles - 1)

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

// newTheme derives the sequences of palette p from Lip Gloss styles once:
// Lip Gloss renders a whole string per call, while a line switches style
// per stretch of cells and writes them itself.
func newTheme(p ui.Palette) *theme {
	var t theme
	for st, ls := range lipglossStyles(colours(p)) {
		// The placeholder is no part of any SGR sequence.
		t.open[st], t.close[st], _ = strings.Cut(ls.Render("x"), "x")
	}
	return &t
}

// palette is a palette's colours as Lip Gloss takes them.
type palette func(ui.Colour) color.Color

// colours is p as Lip Gloss takes it; it reads an empty colour as none,
// which keeps the terminal's own.
func colours(p ui.Palette) palette {
	return func(c ui.Colour) color.Color { return lipgloss.Color(p.Colour(c)) }
}

func (c palette) kind(k syntax.Kind) color.Color { return c(ui.KindColour(k)) }

// addPopupStyles adds the styles of the popup's text on its background.
func addPopupStyles(all map[style]lipgloss.Style, popup lipgloss.Style, c palette) {
	all[popupEmphasisStyle] = popup.Italic(true)
	all[popupStrongStyle] = popup.Bold(true)
	all[popupLinkStyle] = popup.Foreground(c(ui.ChangedFg)).Underline(true)
	all[popupDimStyle] = popup.Foreground(c(ui.DimFg))
	all[popupCodeSpanStyle] = popup.Background(c(ui.CodeSpanBg))
	for k := range syntax.Kinds {
		all[popupCodeStyle(k)] = popup.Foreground(c.kind(k))
	}
}

func lipglossStyles(c palette) map[style]lipgloss.Style {
	base := lipgloss.NewStyle()
	dim := base.Foreground(c(ui.DimFg))
	header := base.Bold(true).Background(c(ui.HeaderBg))
	errorText := base.Foreground(c(ui.ErrorFg))
	popup := base.Background(c(ui.PopupBg))
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
		popupFrameStyle: popup.Foreground(c(ui.DimFg)),
		popupStandIn:    popup.Faint(true),
		caretStyle:      base.Reverse(true),
	}
	addPopupStyles(all, popup, c)
	addListStyles(all, base, c)
	marks := [lineTones]color.Color{
		equalTone: lipgloss.NoColor{}, deletedTone: c(ui.DeletedFg), insertedTone: c(ui.InsertedFg),
		changedTone: c(ui.ChangedFg), absentTone: lipgloss.NoColor{},
	}
	for t, fg := range marks {
		track := base.Foreground(fg)
		all[scrollStyle(tone(t), false)] = track
		all[scrollStyle(tone(t), true)] = track.Background(c(ui.ThumbBg))
	}
	addPaneStyles(all, base, c)
	return all
}

// addPaneStyles adds the styles of the panes in each tone. Equal lines keep
// the terminal's background; under the mouse a line takes a lighter one and
// its number the terminal's foreground, while its changed words keep
// theirs.
func addPaneStyles(all map[style]lipgloss.Style, base lipgloss.Style, c palette) {
	backgrounds := [tones][2]color.Color{
		equalTone:         {lipgloss.NoColor{}, lipgloss.NoColor{}},
		deletedTone:       {c(ui.DeletedBg), c(ui.DeletedStrongBg)},
		insertedTone:      {c(ui.InsertedBg), c(ui.InsertedStrongBg)},
		changedTone:       {c(ui.ChangedBg), c(ui.ChangedStrongBg)},
		absentTone:        {c(ui.AbsentBg), c(ui.AbsentBg)},
		equalHoverTone:    {c(ui.HoverBg), c(ui.HoverBg)},
		deletedHoverTone:  {c(ui.DeletedHoverBg), c(ui.DeletedStrongBg)},
		insertedHoverTone: {c(ui.InsertedHoverBg), c(ui.InsertedStrongBg)},
		changedHoverTone:  {c(ui.ChangedHoverBg), c(ui.ChangedStrongBg)},
	}
	for t, bg := range backgrounds {
		normal, strong := base.Background(bg[0]), base.Background(bg[1])
		gutter := normal.Foreground(c(ui.DimFg))
		if tone(t).isHover() {
			gutter = normal
		}
		all[paneStyle(tone(t), gutterRole)] = gutter
		all[paneStyle(tone(t), noteRole)] = normal.Foreground(c(ui.DirFg)).Bold(true)
		all[paneStyle(tone(t), standInRole)] = normal.Faint(true)
		all[paneStyle(tone(t), inlineStandInRole)] = strong.Faint(true)
		for k := range syntax.Kinds {
			all[textStyle(tone(t), k, false)] = normal.Foreground(c.kind(k))
			all[textStyle(tone(t), k, true)] = strong.Foreground(c.kind(k))
		}
	}
}

// addListStyles adds the styles of the file list in each shade.
func addListStyles(all map[style]lipgloss.Style, base lipgloss.Style, c palette) {
	shades := [listShades]lipgloss.Style{
		plainShade:    base,
		selectedShade: base.Background(c(ui.SelectedBg)),
		currentShade:  base.Bold(true),
	}
	parts := [listParts]color.Color{
		listNamePart: lipgloss.NoColor{}, listAddedPart: c(ui.InsertedFg), listDeletedPart: c(ui.DeletedFg),
		listChangedPart: c(ui.ChangedFg), listDimPart: c(ui.DimFg),
		listDirPart: c(ui.DirFg), listGutterPart: c(ui.GutterFg), listZeroPart: c(ui.ZeroFg),
	}
	for sh, shade := range shades {
		for p, fg := range parts {
			all[listStyle(listPart(p), listShade(sh))] = shade.Foreground(fg)
		}
	}
	all[listStyle(listNamePart, selectedShade)] = shades[selectedShade].Foreground(c(ui.SelectedFg))
}
