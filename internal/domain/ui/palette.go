package ui

import (
	"errors"
	"fmt"
	"slices"
	"strconv"

	"seam/internal/domain/syntax"
)

// ErrInvalidColour is a colour value that is no colour.
var ErrInvalidColour = errors.New("invalid colour")

// Colour is a role a colour plays on the screen. The roles of syntax kinds
// follow the others, see KindColour; plain text and punctuation have one
// too, empty by default, so they keep the terminal's colour.
type Colour uint8

// The roles of the palette.
const (
	DeletedBg Colour = iota
	// The strong backgrounds mark the changed stretches within a line.
	DeletedStrongBg
	InsertedBg
	InsertedStrongBg
	ChangedBg
	ChangedStrongBg
	// AbsentBg is the side of a row that has no line.
	AbsentBg
	// DimFg is the colour of gutters, the footer and other secondary text.
	DimFg
	HeaderBg
	ThumbBg
	PopupBg
	CodeSpanBg
	ErrorFg
	// The file list's selection, folders, gutter and zero counts.
	SelectedBg
	SelectedFg
	DirFg
	GutterFg
	ZeroFg
	// The marks of the scrollbar and the counts of the file list.
	DeletedFg
	InsertedFg
	ChangedFg
)

const firstKindColour = ChangedFg + 1

// Colours is the number of roles, no role itself.
const Colours = ChangedFg + 1 + Colour(syntax.Kinds)

// KindColour is the role of text of kind k.
func KindColour(k syntax.Kind) Colour {
	return firstKindColour + Colour(k)
}

// String is the name of c in the configuration file.
func (c Colour) String() string {
	if c < Colours {
		return colourNames()[c]
	}
	return fmt.Sprintf("Colour(%d)", uint8(c))
}

// ParseColour is the role named name; ok is false for no role.
func ParseColour(name string) (c Colour, ok bool) {
	names := colourNames()
	i := slices.Index(names[:], name)
	return Colour(i), i >= 0
}

func colourNames() [Colours]string {
	kinds := kindNames()
	names := [Colours]string{
		DeletedBg: "deleted_bg", DeletedStrongBg: "deleted_strong_bg",
		InsertedBg: "inserted_bg", InsertedStrongBg: "inserted_strong_bg",
		ChangedBg: "changed_bg", ChangedStrongBg: "changed_strong_bg",
		AbsentBg: "absent_bg", DimFg: "dim_fg", HeaderBg: "header_bg", ThumbBg: "thumb_bg",
		PopupBg: "popup_bg", CodeSpanBg: "code_span_bg", ErrorFg: "error_fg",
		SelectedBg: "selected_bg", SelectedFg: "selected_fg", DirFg: "dir_fg",
		GutterFg: "gutter_fg", ZeroFg: "zero_fg",
		DeletedFg: "deleted_fg", InsertedFg: "inserted_fg", ChangedFg: "changed_fg",
	}
	copy(names[firstKindColour:], kinds[:])
	return names
}

func kindNames() [syntax.Kinds]string {
	return [syntax.Kinds]string{
		syntax.Plain: "plain", syntax.Keyword: "keyword", syntax.Type: "type",
		syntax.Function: "function", syntax.Builtin: "builtin", syntax.Constant: "constant",
		syntax.String: "string", syntax.Escape: "escape", syntax.Number: "number",
		syntax.Comment: "comment", syntax.Meta: "meta", syntax.Operator: "operator",
		syntax.Punctuation: "punctuation", syntax.Tag: "tag", syntax.Attribute: "attribute",
		syntax.Variable: "variable",
	}
}

// Palette is a colour for each role: "#rrggbb", an ANSI colour "0" to
// "255", or empty for the terminal's own.
type Palette struct {
	colours [Colours]string
}

// Colour is the colour of role c.
func (p Palette) Colour(c Colour) string {
	if c >= Colours {
		return ""
	}
	return p.colours[c]
}

// Set gives role c the colour value, if it is one.
func (p *Palette) Set(c Colour, value string) error {
	if c >= Colours {
		return fmt.Errorf("%w: no role %s", ErrInvalidColour, c)
	}
	if !isColour(value) {
		return fmt.Errorf("%w: %q, want \"#rrggbb\", \"0\" to \"255\" or \"\"", ErrInvalidColour, value)
	}
	p.colours[c] = value
	return nil
}

func isColour(v string) bool {
	if v == "" {
		return true
	}
	if v[0] == '#' {
		_, err := strconv.ParseUint(v[1:], 16, 32)
		return len(v) == 7 && err == nil
	}
	n, err := strconv.ParseUint(v, 10, 8)
	return err == nil && strconv.FormatUint(n, 10) == v
}

// ErrUnknownTheme is a theme name that names no built-in palette.
var ErrUnknownTheme = errors.New("unknown theme")

// ThemePalette is the built-in palette called name: "dark" or "light".
func ThemePalette(name string) (Palette, error) {
	switch name {
	case "dark":
		return Dark(), nil
	case "light":
		return Light(), nil
	}
	return Palette{}, fmt.Errorf("%w: %q, want \"dark\" or \"light\"", ErrUnknownTheme, name)
}

// Dark is the palette for a dark background, the default.
func Dark() Palette {
	return Palette{colours: [Colours]string{
		DeletedBg: "#3f2326", DeletedStrongBg: "#7a2e35",
		InsertedBg: "#213a28", InsertedStrongBg: "#2f6b3e",
		ChangedBg: "#1f2f47", ChangedStrongBg: "#2f4f80",
		AbsentBg: "#262626", DimFg: "#7a7a7a", HeaderBg: "#303030", ThumbBg: "#4a4a4a",
		PopupBg: "#21252b", CodeSpanBg: "#2c313a", ErrorFg: "#ff6b6b",
		// The name of the selected file is white, as the design has it.
		SelectedBg: "#2e436e", SelectedFg: "#ffffff", DirFg: "#56a8f5",
		GutterFg: "#393b40", ZeroFg: "#4a4d52",
		// Bright enough to find on the scrollbar's thumb.
		DeletedFg: "#e06c75", InsertedFg: "#98c379", ChangedFg: "#61afef",
		firstKindColour + Colour(syntax.Keyword):   "#c678dd",
		firstKindColour + Colour(syntax.Type):      "#e5c07b",
		firstKindColour + Colour(syntax.Function):  "#61afef",
		firstKindColour + Colour(syntax.Builtin):   "#56b6c2",
		firstKindColour + Colour(syntax.Constant):  "#d19a66",
		firstKindColour + Colour(syntax.String):    "#98c379",
		firstKindColour + Colour(syntax.Escape):    "#56b6c2",
		firstKindColour + Colour(syntax.Number):    "#d19a66",
		firstKindColour + Colour(syntax.Comment):   "#7f848e",
		firstKindColour + Colour(syntax.Meta):      "#e06c75",
		firstKindColour + Colour(syntax.Operator):  "#56b6c2",
		firstKindColour + Colour(syntax.Tag):       "#e06c75",
		firstKindColour + Colour(syntax.Attribute): "#d19a66",
		firstKindColour + Colour(syntax.Variable):  "#e06c75",
	}}
}

// Light is the palette for a light background.
func Light() Palette {
	return Palette{colours: [Colours]string{
		DeletedBg: "#ffebe9", DeletedStrongBg: "#ffb8b0",
		InsertedBg: "#e6ffec", InsertedStrongBg: "#a6f0b8",
		ChangedBg: "#e3f0ff", ChangedStrongBg: "#b4d6ff",
		AbsentBg: "#f0f0f0", DimFg: "#8a8a8a", HeaderBg: "#e4e4e4", ThumbBg: "#c8c8c8",
		PopupBg: "#f3f4f6", CodeSpanBg: "#e4e6ea", ErrorFg: "#cf222e",
		SelectedBg: "#cfe0ff", SelectedFg: "#000000", DirFg: "#0550ae",
		GutterFg: "#d0d0d0", ZeroFg: "#b0b0b0",
		DeletedFg: "#cf222e", InsertedFg: "#1a7f37", ChangedFg: "#0969da",
		firstKindColour + Colour(syntax.Keyword):   "#a626a4",
		firstKindColour + Colour(syntax.Type):      "#986801",
		firstKindColour + Colour(syntax.Function):  "#4078f2",
		firstKindColour + Colour(syntax.Builtin):   "#0184bc",
		firstKindColour + Colour(syntax.Constant):  "#986801",
		firstKindColour + Colour(syntax.String):    "#50a14f",
		firstKindColour + Colour(syntax.Escape):    "#0184bc",
		firstKindColour + Colour(syntax.Number):    "#986801",
		firstKindColour + Colour(syntax.Comment):   "#a0a1a7",
		firstKindColour + Colour(syntax.Meta):      "#e45649",
		firstKindColour + Colour(syntax.Operator):  "#0184bc",
		firstKindColour + Colour(syntax.Tag):       "#e45649",
		firstKindColour + Colour(syntax.Attribute): "#986801",
		firstKindColour + Colour(syntax.Variable):  "#e45649",
	}}
}
