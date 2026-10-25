package ui

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrInvalidKeys is a key map that breaks one of the rules NewKeymap checks.
var ErrInvalidKeys = errors.New("invalid keys")

// Action is what a key does.
type Action uint8

// The actions, grouped by the layer that takes them, see Layer.
const (
	// The motions move the view, or the file list while it has the focus.
	Down Action = iota
	Up
	HalfPageDown
	HalfPageUp
	PageDown
	PageUp
	Top
	Bottom

	Quit
	Reload
	ToggleView
	ToggleLayout
	ToggleStream
	NextChange
	PrevChange
	NextFile
	PrevFile
	ScrollLeft
	ScrollRight
	ToggleFiles
	StartCaret

	Open
	Back
	PrevFolder
	NextFolder

	LeaveCaret
	CaretDown
	CaretUp
	CaretLeft
	CaretRight
	WordForward
	WordBack
	WordEnd
	OtherSide
	Hover

	ClosePopup
	// Actions is the number of actions, no action itself.
	Actions
)

// Layer is a set of actions that take keys at once. The layers are asked
// from the top: an open popup, then the focused file list, then the caret,
// then the view, so a key of an upper layer hides the same key below it.
type Layer uint8

// The layers.
const (
	ViewLayer Layer = iota
	ListLayer
	CaretLayer
	PopupLayer
	Layers
)

// Takes reports whether layer l takes action a: the actions of a layer
// are a stretch of the constants above, and the file list takes the
// motions too.
func (l Layer) Takes(a Action) bool {
	switch {
	case a < Quit:
		return l == ViewLayer || l == ListLayer
	case a < Open:
		return l == ViewLayer
	case a < LeaveCaret:
		return l == ListLayer
	case a < ClosePopup:
		return l == CaretLayer
	}
	return l == PopupLayer && a < Actions
}

// String is the name of a in the configuration file.
func (a Action) String() string {
	if a < Actions {
		return actionNames()[a]
	}
	return fmt.Sprintf("Action(%d)", uint8(a))
}

// ParseAction is the action named name; ok is false for none.
func ParseAction(name string) (a Action, ok bool) {
	names := actionNames()
	i := slices.Index(names[:], name)
	return Action(i), i >= 0
}

func actionNames() [Actions]string {
	return [Actions]string{
		Down: "down", Up: "up", HalfPageDown: "half_page_down", HalfPageUp: "half_page_up",
		PageDown: "page_down", PageUp: "page_up", Top: "top", Bottom: "bottom",
		Quit: "quit", Reload: "reload", ToggleView: "toggle_view", ToggleLayout: "toggle_layout",
		ToggleStream: "toggle_stream", NextChange: "next_change", PrevChange: "prev_change", NextFile: "next_file", PrevFile: "prev_file",
		ScrollLeft: "scroll_left", ScrollRight: "scroll_right", ToggleFiles: "toggle_files", StartCaret: "start_caret",
		Open: "open", Back: "back", PrevFolder: "prev_folder", NextFolder: "next_folder",
		LeaveCaret: "leave_caret", CaretDown: "caret_down", CaretUp: "caret_up",
		CaretLeft: "caret_left", CaretRight: "caret_right",
		WordForward: "word_forward", WordBack: "word_back", WordEnd: "word_end",
		OtherSide: "other_side", Hover: "hover", ClosePopup: "close_popup",
	}
}

func defaultKeys() [Actions][]string {
	return [Actions][]string{
		Down: {"j", "down"}, Up: {"k", "up"}, HalfPageDown: {"ctrl+d"}, HalfPageUp: {"ctrl+u"},
		PageDown: {"pgdown", "space"}, PageUp: {"pgup"}, Top: {"g", "home"}, Bottom: {"G", "end"},
		Quit: {"q", "esc", "ctrl+c"}, Reload: {"r"}, ToggleView: {"f"}, ToggleLayout: {"u"},
		ToggleStream: {"s"}, NextChange: {"n"}, PrevChange: {"p"}, NextFile: {"tab", "]"}, PrevFile: {"shift+tab", "["},
		ScrollLeft: {"h", "left"}, ScrollRight: {"l", "right"}, ToggleFiles: {"b"}, StartCaret: {"i"},
		Open: {"enter"}, Back: {"esc"}, PrevFolder: {"["}, NextFolder: {"]"},
		LeaveCaret: {"i", "esc"}, CaretDown: {"j", "down"}, CaretUp: {"k", "up"},
		CaretLeft: {"h", "left"}, CaretRight: {"l", "right"},
		WordForward: {"w"}, WordBack: {"b"}, WordEnd: {"e"}, OtherSide: {"o"}, Hover: {"K"},
		ClosePopup: {"esc"},
	}
}

// Keymap is the keys of each action. The zero value has none; NewKeymap
// with no bindings is the default. It is shared, never changed, so a copy
// costs a pointer.
type Keymap struct {
	m *keymap
}

type keymap struct {
	keys   [Actions][]string
	layers [Layers]map[string]Action
}

// NewKeymap lays bindings over the default keys: each replaces the keys of
// its action, an empty one leaves the action without. A bound key leaves
// the defaults of the actions that share a layer with its own, so a new
// default never breaks a configuration. Every key must be one the TUI can
// receive, a layer must not take a key for two bound actions, and quitting
// must keep a key.
func NewKeymap(bindings map[Action][]string) (Keymap, error) {
	for a := range bindings {
		if a >= Actions {
			return Keymap{}, fmt.Errorf("%w: no action %s", ErrInvalidKeys, a)
		}
	}
	k := &keymap{keys: defaultKeys()}
	// In the order of the actions, so an error names the same one on every
	// run.
	for a := range Actions {
		keys, ok := bindings[a]
		if !ok {
			continue
		}
		for _, key := range keys {
			if err := checkKey(key); err != nil {
				return Keymap{}, fmt.Errorf("%w: %s: %w", ErrInvalidKeys, a, err)
			}
		}
		k.keys[a] = slices.Clone(keys)
	}
	k.yield(bindings)
	if len(k.keys[Quit]) == 0 {
		return Keymap{}, fmt.Errorf("%w: quit needs a key", ErrInvalidKeys)
	}
	if err := k.conflict(); err != nil {
		return Keymap{}, err
	}
	k.index()
	return Keymap{m: k}, nil
}

// yield drops from the default keys of the actions not bound those that a
// bound action sharing a layer with them takes.
func (k *keymap) yield(bindings map[Action][]string) {
	for a := range Actions {
		if _, bound := bindings[a]; bound {
			continue
		}
		k.keys[a] = slices.DeleteFunc(k.keys[a], func(key string) bool {
			for b, keys := range bindings {
				if sharesLayer(a, b) && slices.Contains(keys, key) {
					return true
				}
			}
			return false
		})
	}
}

func sharesLayer(a, b Action) bool {
	for l := range Layers {
		if l.Takes(a) && l.Takes(b) {
			return true
		}
	}
	return false
}

// conflict is a key a layer would take for two actions; the actions are
// walked in order, so the error names the same pair on every run.
func (k *keymap) conflict() error {
	for l := range Layers {
		seen := make(map[string]Action)
		for a := range Actions {
			if !l.Takes(a) {
				continue
			}
			for _, key := range k.keys[a] {
				if other, ok := seen[key]; ok && other != a {
					return fmt.Errorf("%w: %q is a key of both %s and %s", ErrInvalidKeys, key, other, a)
				}
				seen[key] = a
			}
		}
	}
	return nil
}

// index builds the layers' lookups of keys without a conflict.
func (k *keymap) index() {
	for l := range Layers {
		k.layers[l] = make(map[string]Action)
		for a := range Actions {
			if l.Takes(a) {
				for _, key := range k.keys[a] {
					k.layers[l][key] = a
				}
			}
		}
	}
}

// DefaultKeymap is the keys seam has without a configuration. The tests
// check that the defaults have no conflict, as NewKeymap would.
func DefaultKeymap() Keymap {
	k := &keymap{keys: defaultKeys()}
	k.index()
	return Keymap{m: k}
}

// Keys are the keys of a, first the one to show.
func (k Keymap) Keys(a Action) []string {
	if k.m == nil || a >= Actions {
		return nil
	}
	return slices.Clone(k.m.keys[a])
}

// Action is what key does in layer l; ok is false for nothing.
func (k Keymap) Action(l Layer, key string) (a Action, ok bool) {
	if k.m == nil || l >= Layers {
		return 0, false
	}
	a, ok = k.m.layers[l][key]
	return a, ok
}

// modifiers are in the order the TUI writes them before a key.
func modifiers() []string {
	return []string{"ctrl", "alt", "shift", "meta", "hyper", "super"}
}

// namedKeys are the keys the TUI calls by name rather than by their text.
func namedKeys() []string {
	return []string{
		"enter", "tab", "backspace", "esc", "space", "up", "down", "left", "right",
		"begin", "find", "insert", "delete", "select", "pgup", "pgdown", "home", "end",
	}
}

// checkKey checks key against how the TUI writes the keys it receives:
// modifiers in their fixed order, then a key's name or its one character.
// A shifted character arrives as itself, as G rather than shift+g.
func checkKey(key string) error {
	parts := strings.Split(key, "+")
	// "+" itself, alone or after modifiers, splits into two empty parts.
	if strings.HasSuffix(key, "++") || key == "+" {
		parts = append(parts[:len(parts)-2], "+")
	}
	base, mods := parts[len(parts)-1], parts[:len(parts)-1]
	last := -1
	for _, m := range mods {
		i := slices.Index(modifiers(), m)
		if i <= last {
			return fmt.Errorf("%q: want modifiers among %s in this order", key, strings.Join(modifiers(), ", "))
		}
		last = i
	}
	switch {
	case slices.Contains(namedKeys(), base), isFunctionKey(base):
		return nil
	case utf8.RuneCountInString(base) != 1:
		return fmt.Errorf("%q: want one character or a key's name", key)
	case unicode.IsSpace([]rune(base)[0]) || !unicode.IsPrint([]rune(base)[0]):
		return fmt.Errorf("%q: want a printable character; a space is \"space\"", key)
	case len(mods) == 1 && mods[0] == "shift":
		return fmt.Errorf("%q: a shifted character is the character itself, such as G", key)
	case len(mods) > 0 && unicode.IsUpper([]rune(base)[0]):
		// With another modifier the TUI names the letter in lower case.
		return fmt.Errorf("%q: with a modifier, a capital is shift and the small letter, such as ctrl+shift+g", key)
	}
	return nil
}

// isFunctionKey is f1 to f63.
func isFunctionKey(s string) bool {
	digits, ok := strings.CutPrefix(s, "f")
	n, err := strconv.Atoi(digits)
	return ok && err == nil && n >= 1 && n <= 63 && strconv.Itoa(n) == digits
}
