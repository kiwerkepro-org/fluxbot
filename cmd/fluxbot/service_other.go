//go:build !windows

package main

import (
	"context"
	"fmt"
)

// isWindowsService gibt auf Nicht-Windows-Systemen immer false zurück.
func isWindowsService() bool {
	return false
}

// runAsWindowsService ist auf Nicht-Windows-Systemen ein No-op.
// Auf Linux/macOS wird der Bot direkt via runBot() gestartet.
func runAsWindowsService(ctx context.Context, cancel context.CancelFunc, configPath string) {
	// Wird auf Linux/macOS nicht aufgerufen – nur Stub für Compilierung
	_ = ctx
	_ = cancel
	_ = configPath
}

// installService gibt auf Nicht-Windows-Systemen einen Hinweis aus.
// Für Linux-Systemd: deploy/linux/install.sh verwenden.
func installService(exePath, configPath string) error {
	return fmt.Errorf(
		"Windows-Dienst-Installation ist nur unter Windows verfügbar.\n" +
			"Für Linux-Systemd: deploy/linux/install.sh verwenden.\n" +
			"Für macOS: launchd-Konfiguration in deploy/macos/ (geplant)",
	)
}

// uninstallService gibt auf Nicht-Windows-Systemen einen Hinweis aus.
func uninstallService() error {
	return fmt.Errorf(
		"Windows-Dienst-Deinstallation ist nur unter Windows verfügbar.\n" +
			"Für Linux-Systemd: sudo systemctl disable --now fluxbot && sudo rm /etc/systemd/system/fluxbot.service",
	)
}
