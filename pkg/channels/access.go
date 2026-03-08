// Package channels – access.go
// Zentrale Zugriffskontroll-Logik für alle Kanäle (P10: Granulare DM-Policy).
package channels

import (
	"fmt"
	"log"

	"github.com/ki-werke/fluxbot/pkg/pairing"
)

const DefaultPairingMsg = "⏳ Pairing erforderlich.\n\nDu bist noch nicht berechtigt, diesen Bot zu nutzen.\nDeine User-ID wurde an den Admin gesendet.\n\nBitte warte auf Freigabe im Dashboard."

// AccessResult ist das Ergebnis einer Zugriffsprüfung.
type AccessResult int

const (
	AccessAllowed  AccessResult = iota // Nachricht darf verarbeitet werden
	AccessDenied                       // Nachricht wird still ignoriert
	AccessPending                      // Pairing-Request gesendet, Nachricht ignorieren
)

// AccessConfig enthält alle Daten für eine Zugriffsprüfung.
type AccessConfig struct {
	Channel        string         // "telegram", "discord", etc.
	SenderID       string         // User-ID des Senders
	UserName       string         // Anzeigename (für Logs)
	ChatID         string         // Chat-ID für Antworten
	IsDM           bool           // true = Direktnachricht, false = Gruppe/Server
	DMMode         string         // "open" | "allowlist" | "pairing"
	GroupMode      string         // "open" | "allowlist"
	AllowFrom      []string       // Statische Whitelist
	PairingStore   *pairing.Store // nil = Pairing deaktiviert
	PairingMessage string         // Custom-Nachricht (leer = Default)
	SendFn         func(string)   // Funktion zum Senden einer Nachricht an den User
}

// CheckAccess prüft ob ein User Nachrichten senden darf.
// Rückgabe: AccessAllowed, AccessDenied oder AccessPending.
func CheckAccess(cfg AccessConfig) AccessResult {
	// Stufe 1: Statische Whitelist hat immer Vorrang (in AllowFrom = immer erlaubt)
	if isInAllowlist(cfg.SenderID, cfg.AllowFrom) {
		return AccessAllowed
	}

	// Modus ermitteln: DM oder Gruppe
	mode := cfg.GroupMode
	if cfg.IsDM {
		mode = cfg.DMMode
	}
	if mode == "" {
		mode = "open" // Sicherer Default
	}

	switch mode {
	case "open":
		return AccessAllowed

	case "allowlist":
		// AllowFrom ist bereits geprüft (Stufe 1), also ist der User nicht drin
		if len(cfg.AllowFrom) == 0 {
			return AccessAllowed // Leere Allowlist = offen
		}
		log.Printf("[%s/Access] User %s (@%s) nicht in Allowlist – blockiert", cfg.Channel, cfg.SenderID, cfg.UserName)
		return AccessDenied

	case "pairing":
		if cfg.PairingStore == nil {
			log.Printf("[%s/Access] Pairing-Mode gesetzt aber kein Store – blockiert", cfg.Channel)
			return AccessDenied
		}
		if cfg.PairingStore.IsBlocked(cfg.Channel, cfg.SenderID) {
			log.Printf("[%s/Pairing] 🚫 Blockierter User: %s @%s", cfg.Channel, cfg.SenderID, cfg.UserName)
			return AccessDenied
		}
		if cfg.PairingStore.IsPaired(cfg.Channel, cfg.SenderID) {
			return AccessAllowed
		}
		// Unbekannter User → Pairing-Request
		isNew := cfg.PairingStore.RequestPairing(cfg.Channel, cfg.SenderID, cfg.UserName, cfg.ChatID)
		if isNew {
			log.Printf("[%s/Pairing] 📩 Neuer Pairing-Request: %s @%s", cfg.Channel, cfg.SenderID, cfg.UserName)
		}
		if cfg.SendFn != nil {
			msg := cfg.PairingMessage
			if msg == "" {
				msg = DefaultPairingMsg
			}
			cfg.SendFn(fmt.Sprintf("%s\n\n🆔 Deine User-ID: %s", msg, cfg.SenderID))
		}
		return AccessPending

	default:
		log.Printf("[%s/Access] Unbekannter Modus '%s' – offen", cfg.Channel, mode)
		return AccessAllowed
	}
}

func isInAllowlist(senderID string, allowFrom []string) bool {
	for _, id := range allowFrom {
		if id == senderID {
			return true
		}
	}
	return false
}
