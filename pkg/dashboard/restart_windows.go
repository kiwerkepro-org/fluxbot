//go:build windows

package dashboard

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"
)

const (
	createBreakawayFromJob = 0x01000000
	createNoWindow         = 0x08000000
)

// startDetached startet FluxBot neu über PowerShell Start-Process.
//
// Ablauf:
//  1. PowerShell startet (hidden, kein Fenster, getrennt vom aktuellen Prozess)
//  2. PowerShell wartet 1.5s (alter FluxBot-Prozess hat Zeit zu sterben + Port freizugeben)
//  3. Start-Process startet neues fluxbot.exe mit korrektem WorkingDirectory + WindowStyle Hidden
//
// Vorteile gegenüber direktem cmd.Start():
//   - Kein Port-Konflikt: neuer Prozess startet erst nach dem Tod des alten
//   - Start-Process -WindowStyle Hidden: kein Konsolenfenster, terminal-unabhängig
//   - Bewährter Windows-Mechanismus (identisch mit install.ps1)
func startDetached(exe string, args []string) error {
	dir := filepath.Dir(exe)
	psCmd := fmt.Sprintf(
		`Start-Sleep -Milliseconds 1500; Start-Process -FilePath '%s' -WorkingDirectory '%s' -WindowStyle Hidden`,
		exe, dir,
	)
	cmd := exec.Command("powershell.exe",
		"-NonInteractive",
		"-WindowStyle", "Hidden",
		"-Command", psCmd,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | createBreakawayFromJob | createNoWindow,
	}
	return cmd.Start()
}
