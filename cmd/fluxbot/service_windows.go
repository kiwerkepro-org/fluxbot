//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName        = "FluxBot"
	serviceDisplayName = "FluxBot – Multi-Channel AI Agent"
	serviceDescription = "FluxBot KI-Assistent von KI-WERKE (github.com/ki-werke)"
)

// isWindowsService gibt zurück, ob FluxBot vom Windows Service Control Manager gestartet wurde.
func isWindowsService() bool {
	inService, _ := svc.IsWindowsService()
	return inService
}

// fluxSvc implementiert den Windows Service Handler.
type fluxSvc struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (s *fluxSvc) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for {
		select {
		case <-s.ctx.Done():
			changes <- svc.Status{State: svc.StopPending}
			return false, 0
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				s.cancel()
				return false, 0
			}
		}
	}
}

// runAsWindowsService startet FluxBot als Windows-Dienst.
func runAsWindowsService(ctx context.Context, cancel context.CancelFunc, configPath string) {
	// ── Working Directory auf das Verzeichnis der EXE setzen ─────────────────
	// Windows-Dienste starten mit CWD = C:\Windows\System32.
	// Relative Pfade (z.B. "workspace": "./workspace") funktionieren dann nicht.
	// Fix: CWD auf das Verzeichnis der EXE setzen, damit alle relativen Pfade stimmen.
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		if err := os.Chdir(exeDir); err != nil {
			log.Printf("[Service] Warnung: Working Directory konnte nicht gesetzt werden: %v", err)
		} else {
			log.Printf("[Service] Working Directory: %s", exeDir)
		}
	}

	elog, err := eventlog.Open(serviceName)
	if err != nil {
		// Fallback: kein Eventlog – trotzdem starten
		log.Printf("[Service] Eventlog nicht verfügbar: %v", err)
	} else {
		defer elog.Close()
		elog.Info(1, "FluxBot Service startet")
	}

	// Bot in Goroutine starten
	go func() {
		printBanner()
		runBot(ctx, configPath)
		cancel() // Bot hat sich beendet → Service stoppen
	}()

	// Windows SCM-Handler laufen lassen
	handler := &fluxSvc{ctx: ctx, cancel: cancel}
	if err := svc.Run(serviceName, handler); err != nil {
		log.Printf("[Service] svc.Run Fehler: %v", err)
	}

	if elog != nil {
		elog.Info(1, "FluxBot Service beendet")
	}
}

// installService registriert FluxBot als Windows-Dienst.
// Muss als Administrator ausgeführt werden.
func installService(exePath, configPath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("Service Manager nicht erreichbar (Administrator-Rechte erforderlich): %w", err)
	}
	defer m.Disconnect()

	// Prüfen ob Dienst bereits existiert
	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("Dienst '%s' ist bereits installiert.\nZum Neuinstallieren: fluxbot.exe --service uninstall", serviceName)
	}

	// Dienst erstellen
	s, err = m.CreateService(serviceName, exePath, mgr.Config{
		DisplayName:      serviceDisplayName,
		Description:      serviceDescription,
		StartType:        mgr.StartAutomatic,
		DelayedAutoStart: true, // Startet erst nach anderen Diensten (Netzwerk etc.)
	}, "--config", configPath, "--service", "run")
	if err != nil {
		return fmt.Errorf("Dienst konnte nicht erstellt werden: %w", err)
	}
	defer s.Close()

	// Windows Eventlog einrichten
	if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		s.Delete()
		return fmt.Errorf("Eventlog-Einrichtung fehlgeschlagen: %w", err)
	}

	fmt.Printf("✅ FluxBot als Windows-Dienst installiert.\n")
	fmt.Printf("   Starten:     sc start %s\n", serviceName)
	fmt.Printf("   Status:      sc query %s\n", serviceName)
	fmt.Printf("   Deinstall:   fluxbot.exe --service uninstall\n")
	fmt.Printf("   Config:      %s\n", configPath)
	return nil
}

// uninstallService entfernt den FluxBot Windows-Dienst.
func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("Service Manager nicht erreichbar (Administrator-Rechte erforderlich): %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("Dienst '%s' nicht gefunden: %w", serviceName, err)
	}
	defer s.Close()

	// Dienst stoppen falls er läuft
	status, err := s.Query()
	if err == nil && status.State == svc.Running {
		if _, err := s.Control(svc.Stop); err != nil {
			log.Printf("[Service] Warnung: Dienst konnte nicht gestoppt werden: %v", err)
		}
		time.Sleep(2 * time.Second)
	}

	if err := s.Delete(); err != nil {
		return fmt.Errorf("Dienst konnte nicht entfernt werden: %w", err)
	}

	if err := eventlog.Remove(serviceName); err != nil {
		log.Printf("[Service] Warnung: Eventlog konnte nicht entfernt werden: %v", err)
	}

	fmt.Printf("✅ FluxBot Windows-Dienst '%s' deinstalliert.\n", serviceName)
	return nil
}

