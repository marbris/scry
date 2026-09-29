// Package browser opens a link in the system's web browser, for the few
// things a terminal can't show: a deck on Moxfield, a card's picture on a
// terminal that can't draw one.
package browser

import (
	"os/exec"
	"runtime"
)

// Open hands a link to whatever opens links here, and doesn't wait for the
// browser: it runs alongside, not in front.
func Open(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Reaped in the background, so an xdg-open that exits leaves no zombie.
	go cmd.Wait()
	return nil
}
