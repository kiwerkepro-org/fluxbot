package logger

import (
	"fmt"
	"log"
	"os"
	"time"
)

// Level definiert den Log-Level
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Logger ist ein einfacher strukturierter Logger für FluxBot
type Logger struct {
	level  Level
	logger *log.Logger
}

// New erstellt einen neuen Logger
func New(debug bool) *Logger {
	level := LevelInfo
	if debug {
		level = LevelDebug
	}
	return &Logger{
		level:  level,
		logger: log.New(os.Stdout, "", 0),
	}
}

// Debug gibt eine Debug-Nachricht aus (nur wenn debug=true)
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level <= LevelDebug {
		l.print("DEBUG", format, args...)
	}
}

// Info gibt eine Info-Nachricht aus
func (l *Logger) Info(format string, args ...interface{}) {
	if l.level <= LevelInfo {
		l.print("INFO ", format, args...)
	}
}

// Warn gibt eine Warnung aus
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level <= LevelWarn {
		l.print("WARN ", format, args...)
	}
}

// Error gibt einen Fehler aus
func (l *Logger) Error(format string, args ...interface{}) {
	if l.level <= LevelError {
		l.print("ERROR", format, args...)
	}
}

func (l *Logger) print(level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.logger.Printf("[%s] [%s] %s", time.Now().Format("15:04:05"), level, msg)
}
