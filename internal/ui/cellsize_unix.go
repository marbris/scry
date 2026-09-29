//go:build unix

package ui

import (
	"os"

	"golang.org/x/sys/unix"
)

// cellAspect is how many times taller than wide one character cell is, as
// the terminal reports its size in pixels — so a picture's box fits the
// picture. 2 is the usual answer, and the guess where the terminal won't say.
func cellAspect() float64 {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Xpixel == 0 || ws.Ypixel == 0 || ws.Col == 0 || ws.Row == 0 {
		return 2
	}
	return (float64(ws.Ypixel) / float64(ws.Row)) / (float64(ws.Xpixel) / float64(ws.Col))
}
