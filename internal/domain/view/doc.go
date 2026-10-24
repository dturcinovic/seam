// Package view lays out the rows of a file diff as the lines of a view, the
// full file or the changed blocks with context and folds, and keeps a view's
// place across a change of view through its anchor row. It knows nothing of
// the terminal: no widths, colours or keys.
package view
