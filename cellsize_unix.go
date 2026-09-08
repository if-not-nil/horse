//go:build unix

package main

import "golang.org/x/sys/unix"

// termCellSize returns the terminal cell size in pixels via the TIOCGWINSZ ioctl.
// ok is false when the terminal doesn't report pixel dimensions.
func termCellSize(fd int) (cw, ch int, ok bool) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil || ws.Xpixel == 0 || ws.Ypixel == 0 || ws.Col == 0 || ws.Row == 0 {
		return 0, 0, false
	}
	return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row), true
}
