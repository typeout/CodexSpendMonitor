package trayutil

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenURL asks the operating system to open url with the user's default browser.
func OpenURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening %s: %w", url, err)
	}
	return nil
}
