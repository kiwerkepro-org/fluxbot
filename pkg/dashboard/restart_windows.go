//go:build windows

package dashboard

import (
	"os/exec"
	"path/filepath"
	"syscall"
)

const (
	createBreakawayFromJob = 0x01000000 // Kind-Prozess verlässt den Job-Object (Task Scheduler)
	createNoWindow         = 0x08000000 // Kein Konsolenfenster – Prozess stirbt nicht wenn Terminal geschlossen wird
)

// startDetached startet einen neuen Prozess, der vollständig vom aktuellen getrennt ist.
//   - CREATE_NEW_PROCESS_GROUP: eigene Prozessgruppe → kein CTRL+C vom Parent
//   - CREATE_BREAKAWAY_FROM_JOB: verlässt Task Scheduler Job-Object
//   - CREATE_NO_WINDOW: kein Konsolenfenster → überlebt das Schließen des Terminals
//   - cmd.Dir: explizit auf EXE-Verzeichnis gesetzt → korrektes Working Directory nach Restart
func startDetached(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createBreakawayFromJob | createNoWindow,
	}
	return cmd.Start()
}
