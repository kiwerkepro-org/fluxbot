//go:build !windows

package dashboard

import "os/exec"

// startDetached startet einen neuen Prozess (Linux/macOS – keine Job Objects).
func startDetached(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	return cmd.Start()
}
