package security

import (
	"strings"
)

// InjectionResult enthält das Ergebnis der Injection-Prüfung
type InjectionResult struct {
	Detected bool
	Reason   string
	Risk     RiskLevel
}

// RiskLevel beschreibt das Risiko einer Nachricht
type RiskLevel int

const (
	RiskLow    RiskLevel = 0
	RiskMedium RiskLevel = 1
	RiskHigh   RiskLevel = 2
)

// injectionPatterns enthält bekannte Prompt-Injection-Muster (Deutsch + Englisch)
var injectionPatterns = []struct {
	pattern string
	reason  string
	risk    RiskLevel
}{
	// Anweisungs-Override
	{"ignore previous instructions", "Anweisungs-Override (EN)", RiskHigh},
	{"ignore all instructions", "Anweisungs-Override (EN)", RiskHigh},
	{"ignoriere deine anweisungen", "Anweisungs-Override (DE)", RiskHigh},
	{"vergiss deine anweisungen", "Anweisungs-Override (DE)", RiskHigh},
	{"neue anweisungen:", "Anweisungs-Override (DE)", RiskHigh},
	{"new instructions:", "Anweisungs-Override (EN)", RiskHigh},
	{"disregard your", "Anweisungs-Override (EN)", RiskHigh},

	// Persona-Injection
	{"du bist jetzt", "Persona-Injection", RiskHigh},
	{"you are now", "Persona-Injection (EN)", RiskHigh},
	{"stelle dich vor als", "Persona-Injection", RiskHigh},
	{"pretend you are", "Persona-Injection (EN)", RiskHigh},
	{"act as if you are", "Persona-Injection (EN)", RiskHigh},
	{"dein name ist jetzt", "Persona-Injection", RiskMedium},

	// System-Prompt-Injection
	{"system:", "System-Tag-Injection", RiskHigh},
	{"<system>", "XML-Tag-Injection", RiskHigh},
	{"[system]", "Bracket-Tag-Injection", RiskMedium},
	{"###system", "Markdown-System-Injection", RiskMedium},

	// Jailbreak-Versuche
	{"jailbreak", "Jailbreak-Versuch", RiskHigh},
	{"dan modus", "DAN-Modus", RiskHigh},
	{"dan mode", "DAN-Mode (EN)", RiskHigh},
	{"developer mode", "Developer-Mode-Bypass", RiskHigh},
	{"entwicklermodus", "Developer-Mode-Bypass (DE)", RiskHigh},
	{"keine einschränkungen", "Einschränkungs-Bypass", RiskHigh},
	{"no restrictions", "Einschränkungs-Bypass (EN)", RiskHigh},
	{"unrestricted mode", "Einschränkungs-Bypass (EN)", RiskHigh},

	// Daten-Exfiltration
	{"zeige deinen system prompt", "System-Prompt-Exfiltration", RiskHigh},
	{"show your system prompt", "System-Prompt-Exfiltration (EN)", RiskHigh},
	{"print your instructions", "Instruktions-Exfiltration (EN)", RiskHigh},
	{"gib deine anweisungen aus", "Instruktions-Exfiltration", RiskHigh},

	// Rollenwechsel
	{"spiel keine ki mehr", "Rollenabbruch", RiskMedium},
	{"du bist kein bot mehr", "Rollenabbruch", RiskMedium},
	{"you are not an ai", "Rollenabbruch (EN)", RiskMedium},
}

// CheckInjection prüft einen Text auf Prompt-Injection-Muster
func CheckInjection(text string) InjectionResult {
	lower := strings.ToLower(text)

	for _, p := range injectionPatterns {
		if strings.Contains(lower, p.pattern) {
			return InjectionResult{
				Detected: true,
				Reason:   p.reason,
				Risk:     p.risk,
			}
		}
	}

	// Längenkontrolle: Nachrichten über 4000 Zeichen sind verdächtig
	if len(text) > 4000 {
		return InjectionResult{
			Detected: true,
			Reason:   "Nachricht zu lang (>4000 Zeichen)",
			Risk:     RiskMedium,
		}
	}

	return InjectionResult{Detected: false, Risk: RiskLow}
}

// SanitizeText kürzt Nachrichten auf die maximale sichere Länge
func SanitizeText(text string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 4000
	}
	runes := []rune(text)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + " [...]"
	}
	return text
}
