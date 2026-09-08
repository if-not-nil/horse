//go:build !unix

package main

// termCellSize can't query pixel dimensions on this platform (e.g. windows),
// so the caller falls back to its ~1:2 guess.
func termCellSize(fd int) (cw, ch int, ok bool) {
	return 0, 0, false
}
