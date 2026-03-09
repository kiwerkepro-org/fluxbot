//go:build windows

package main

import "syscall"

var (
	modKernel32     = syscall.NewLazyDLL("kernel32.dll")
	procFreeConsole = modKernel32.NewProc("FreeConsole")
)

// detachConsole löst den Prozess vom Eltern-Terminal (Console).
// Danach stirbt FluxBot nicht mehr, wenn das Terminal geschlossen wird.
// Ist kein Terminal vorhanden (Task Scheduler, Windows-Dienst), ist der Aufruf ein No-Op.
func detachConsole() {
	procFreeConsole.Call()
}
