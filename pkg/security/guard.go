package security

import (
	"log"
	"time"
)

// Guard kombiniert alle Sicherheitsprüfungen
type Guard struct {
	audit       *AuditLogger
	rateLimiter *RateLimiter
	blockOnHigh bool // High-Risk-Injections blockieren
}

// GuardConfig konfiguriert den Security Guard
type GuardConfig struct {
	WorkspacePath  string
	MaxMsgPerMin   int  // Standard: 30
	BlockInjection bool // High-Risk-Nachrichten blockieren?
}

// NewGuard erstellt einen neuen Security Guard
func NewGuard(cfg GuardConfig) *Guard {
	maxMsg := cfg.MaxMsgPerMin
	if maxMsg <= 0 {
		maxMsg = 30
	}
	return &Guard{
		audit:       NewAuditLogger(cfg.WorkspacePath),
		rateLimiter: NewRateLimiter(maxMsg, time.Minute),
		blockOnHigh: cfg.BlockInjection,
	}
}

// GetAuditLogger gibt Zugriff auf den AuditLogger (Session 31: für Agent-Integration)
func (g *Guard) GetAuditLogger() *AuditLogger {
	if g == nil {
		return nil
	}
	return g.audit
}

// CheckResult enthält das Ergebnis der Sicherheitsprüfung
type CheckResult struct {
	Allowed   bool
	Response  string // Antwort die an den Nutzer gesendet werden soll (wenn nicht allowed)
}

// Check prüft eine eingehende Nachricht auf Sicherheitsprobleme
func (g *Guard) Check(channelID, userID, msgType, text string) CheckResult {
	// 1. Rate Limiting
	if !g.rateLimiter.Allow(userID) {
		log.Printf("[Security] Rate-Limit erreicht: user=%s channel=%s", userID, channelID)
		g.audit.Log(AuditEntry{
			Timestamp:   time.Now(),
			ChannelID:   channelID,
			UserID:      userID,
			MessageType: msgType,
			Length:      len(text),
			Blocked:     true,
		})
		return CheckResult{
			Allowed:  false,
			Response: "⏱️ Du sendest zu schnell. Bitte warte einen Moment.",
		}
	}

	// 2. Injection-Check
	injResult := CheckInjection(text)

	// Audit-Log schreiben
	g.audit.Log(AuditEntry{
		Timestamp:   time.Now(),
		ChannelID:   channelID,
		UserID:      userID,
		MessageType: msgType,
		Length:      len(text),
		Injection:   injResult.Detected,
		InjReason:   injResult.Reason,
		Blocked:     injResult.Detected && injResult.Risk == RiskHigh && g.blockOnHigh,
	})

	if injResult.Detected {
		if injResult.Risk == RiskHigh && g.blockOnHigh {
			log.Printf("[Security] High-Risk-Injection GEBLOCKT: user=%s reason=%s", userID, injResult.Reason)
			return CheckResult{
				Allowed:  false,
				Response: "🛡️ Diese Anfrage kann ich nicht verarbeiten.",
			}
		}
		// Medium/Low: Loggen aber durchlassen
		log.Printf("[Security] Injection-Muster erkannt (nicht geblockt): user=%s risk=%d reason=%s",
			userID, injResult.Risk, injResult.Reason)
	}

	return CheckResult{Allowed: true}
}

// CleanOldLogs löscht alte Audit-Logs (DSGVO)
func (g *Guard) CleanOldLogs(retentionDays int) {
	g.audit.CleanOldLogs(retentionDays)
}
