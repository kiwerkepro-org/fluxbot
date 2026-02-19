package security

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditLogger schreibt alle Nachrichten in eine täglich rotierende Logdatei.
// DSGVO-konform: Keine Nachrichteninhalte, nur Metadaten.
type AuditLogger struct {
	mu      sync.Mutex
	logDir  string
	file    *os.File
	fileDay int
}

// AuditEntry beschreibt einen Audit-Logeintrag
type AuditEntry struct {
	Timestamp   time.Time
	ChannelID   string
	UserID      string
	MessageType string
	Length      int
	Injection   bool
	InjReason   string
	Blocked     bool
}

// NewAuditLogger erstellt einen neuen Audit-Logger
func NewAuditLogger(workspacePath string) *AuditLogger {
	logDir := filepath.Join(workspacePath, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("[Security] Fehler beim Erstellen des Log-Verzeichnisses: %v", err)
	}
	return &AuditLogger{logDir: logDir}
}

// Log schreibt einen Audit-Eintrag
func (a *AuditLogger) Log(entry AuditEntry) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	day := now.YearDay()

	// Tägliche Rotation
	if a.file == nil || a.fileDay != day {
		if a.file != nil {
			a.file.Close()
		}
		filename := filepath.Join(a.logDir, fmt.Sprintf("audit-%s.log", now.Format("2006-01-02")))
		f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("[Security] Fehler beim Öffnen der Audit-Logdatei: %v", err)
			return
		}
		a.file = f
		a.fileDay = day
	}

	// DSGVO: Nur Metadaten, keine Inhalte
	blocked := ""
	if entry.Blocked {
		blocked = " [GEBLOCKT]"
	}
	injInfo := ""
	if entry.Injection {
		injInfo = fmt.Sprintf(" [INJECTION: %s]", entry.InjReason)
	}

	line := fmt.Sprintf("[%s] channel=%s user=%s type=%s len=%d%s%s\n",
		entry.Timestamp.Format("2006-01-02 15:04:05"),
		entry.ChannelID,
		entry.UserID,
		entry.MessageType,
		entry.Length,
		injInfo,
		blocked,
	)

	if _, err := a.file.WriteString(line); err != nil {
		log.Printf("[Security] Fehler beim Schreiben des Audit-Logs: %v", err)
	}
}

// CleanOldLogs löscht Audit-Logs die älter als retentionDays Tage sind (DSGVO-Datenminimierung)
func (a *AuditLogger) CleanOldLogs(retentionDays int) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	entries, err := os.ReadDir(a.logDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(a.logDir, entry.Name())
			os.Remove(path)
			log.Printf("[Security] Alte Logdatei gelöscht (DSGVO): %s", entry.Name())
		}
	}
}
