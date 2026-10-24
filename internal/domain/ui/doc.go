// Package ui holds what a user may set about the viewer: the palette, the
// keys of its actions, the context of the hunk view, the delay of a hover
// and when the sides stand side by side. It checks each setting and knows
// the names the configuration file gives them. It draws nothing and reads
// no key: the TUI does both, and the key names it checks are those the TUI
// receives.
package ui
