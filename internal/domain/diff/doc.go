// Package diff compares two versions of a file line by line and aligns them
// side by side: equal lines pair up, a block of changed lines pairs up as far
// as both sides reach, and the rest of the block stands alone. Within a pair
// of changed lines it marks the words that differ. It knows nothing of git,
// colours or the terminal.
package diff
