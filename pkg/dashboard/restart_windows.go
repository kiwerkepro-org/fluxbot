//go:build windows

package dashboard

import (
	"os/exec"
	"syscall"
)

// startDetached startet einen neuen Prozess, der vom aktuellen getrennt ist.
// Auf Windows: CREATE_BREAKAWAY_FROM_JOB verhindert, dass der Kind-Prozess
// beim Beenden des Elternprozesses (Task Scheduler Job Object) getötet wird.
func startDetached(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x01000000, // 0x01000000 = CREATE_BREAKAWAY_FROM_JOB
	}
	return cmd.Start()
}
