package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// copyToClipboard copia text al portapapeles del sistema usando la utilidad
// nativa de cada SO, sin dependencias externas.
func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default: // linux/bsd
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else {
			return fmt.Errorf("no encontré wl-copy ni xclip para copiar al portapapeles")
		}
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
