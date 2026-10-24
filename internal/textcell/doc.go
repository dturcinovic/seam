// Package textcell maps text to terminal cells: grapheme boundaries, cell
// widths, tab stops and visible replacements for control characters, each
// cell traced back to its original bytes. Rendering and hit testing both use
// it, so they cannot disagree. It knows no styles and no colours.
package textcell
