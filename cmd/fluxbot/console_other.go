//go:build !windows

package main

// detachConsole ist auf Nicht-Windows-Systemen ein No-Op.
func detachConsole() {}
