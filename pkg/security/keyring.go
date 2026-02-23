package security

// Package security – Keyring-Abstraktionsschicht
//
// Unterstützt zwei Secret-Backends:
//   - KeyringProvider  → OS-nativer System-Keyring (Windows: Credential Manager,
//                         macOS: Keychain, Linux: libsecret)
//   - VaultProvider    → AES-256-GCM Datei-Vault (primär für Docker)
//
// Die Ladereihenfolge (höchste Priorität zuerst):
//   1. System-Keyring  (lokal – Windows / macOS)
//   2. AES-256-GCM Vault (Docker / alle Plattformen als Fallback)
//   3. Env-Variablen   (CI/CD, .env)
//
// IsDockerEnvironment() entscheidet automatisch welches Backend genutzt wird.
// ChainedProvider kombiniert Keyring + Vault für transparenten Zugriff.

import (
	"errors"
	"log"
	"os"
	"strings"
)

// errKeyringNotFound wird zurückgegeben wenn ein Key im Keyring nicht existiert.
// Implementierungen in keyring_windows.go / keyring_other.go verwenden diesen Fehler.
var errKeyringNotFound = errors.New("keyring: key not found")

// errKeyringUnsupported wird zurückgegeben wenn der System-Keyring nicht verfügbar ist.
var errKeyringUnsupported = errors.New("keyring: not supported on this platform")

// KeyringServiceName ist der Service-Name unter dem FluxBot Secrets im Keyring speichert.
// Windows: erscheint als "FluxBot/<key>" im Windows Credential Manager.
const KeyringServiceName = "FluxBot"

// ── Docker-Erkennung ──────────────────────────────────────────────────────────

// IsDockerEnvironment prüft ob FluxBot in einem Docker-Container läuft.
//
// Erkennungsmethoden (in Reihenfolge):
//  1. Datei /.dockerenv existiert (Standard bei Docker)
//  2. Umgebungsvariable FLUXBOT_DOCKER=true oder DOCKER_ENV=true
//  3. /proc/1/cgroup enthält "docker" oder "kubepods"
//
// Im Docker-Modus: System-Keyring nicht nutzbar (kein Display/Session-Bus) → Vault.
// Lokal (Windows/macOS): System-Keyring bevorzugt.
func IsDockerEnvironment() bool {
	// Methode 1: /.dockerenv (Docker-Standard)
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	// Methode 2: Explizite Umgebungsvariable (für Sonderfälle)
	if os.Getenv("FLUXBOT_DOCKER") == "true" || os.Getenv("DOCKER_ENV") == "true" {
		return true
	}
	// Methode 3: /proc/1/cgroup (Linux-Container, Kubernetes)
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") || strings.Contains(content, "kubepods") {
			return true
		}
	}
	return false
}

// ── KeyringProvider ───────────────────────────────────────────────────────────

// KeyringProvider implementiert SecretProvider über den OS-nativen System-Keyring.
//
// Plattform-Implementierungen:
//   - keyring_windows.go  → Windows Credential Manager (CredReadW, CredWriteW, CredDeleteW)
//   - keyring_other.go    → Stub für Linux/macOS (gibt errKeyringUnsupported zurück)
//
// Einschränkung: GetAll() kennt nur die statisch definierten Vault-Keys (allKnownKeys).
// Dynamische INTEG_*-Keys werden ebenfalls per Präfix-Suche eingelesen (keyring_windows.go).
type KeyringProvider struct{}

// NewKeyringProvider erstellt einen KeyringProvider und testet die Erreichbarkeit.
// Gibt einen Fehler zurück wenn der System-Keyring nicht verfügbar ist.
func NewKeyringProvider() (*KeyringProvider, error) {
	kp := &KeyringProvider{}
	// Erreichbarkeitstest: lese einen nicht-existierenden Schlüssel
	// → errKeyringNotFound ist OK, andere Fehler bedeuten "nicht verfügbar"
	_, err := kp.Get("__fluxbot_ping__")
	if err != nil && !errors.Is(err, errKeyringNotFound) {
		return nil, err
	}
	log.Printf("[Keyring] ✅ Initialisiert – Backend: %s", kp.Backend())
	return kp, nil
}

// Get liest einen Secret aus dem System-Keyring.
// Gibt "" zurück wenn der Key nicht existiert (kein Fehler).
func (kp *KeyringProvider) Get(key string) (string, error) {
	val, err := keyringGet(KeyringServiceName, key)
	if errors.Is(err, errKeyringNotFound) {
		return "", nil
	}
	return val, err
}

// Set schreibt einen Secret in den System-Keyring.
// Leerer Wert löscht den Key.
func (kp *KeyringProvider) Set(key, value string) error {
	if value == "" {
		return kp.Delete(key)
	}
	return keyringSet(KeyringServiceName, key, value)
}

// Delete entfernt einen Secret aus dem System-Keyring.
func (kp *KeyringProvider) Delete(key string) error {
	err := keyringDelete(KeyringServiceName, key)
	if errors.Is(err, errKeyringNotFound) {
		return nil
	}
	return err
}

// GetAll liest alle bekannten FluxBot-Secrets aus dem Keyring.
// Iteriert über allKnownKeys() + dynamisch gespeicherte INTEG_*-Keys.
func (kp *KeyringProvider) GetAll() (map[string]string, error) {
	result := make(map[string]string)
	for _, k := range allKnownKeys() {
		if v, err := kp.Get(k); err == nil && v != "" {
			result[k] = v
		}
	}
	// Dynamische INTEG_*-Keys via Enumeration (plattformspezifisch)
	if dynamic, err := keyringEnumDynamic(KeyringServiceName); err == nil {
		for k, v := range dynamic {
			if _, exists := result[k]; !exists {
				result[k] = v
			}
		}
	}
	return result, nil
}

// SetBatch schreibt mehrere Secrets auf einmal in den Keyring.
func (kp *KeyringProvider) SetBatch(updates map[string]string) error {
	for k, v := range updates {
		if err := kp.Set(k, v); err != nil {
			return err
		}
	}
	return nil
}

// MigrateFromConfig ist ein No-Op für den Keyring (Migration nur für Vault relevant).
func (kp *KeyringProvider) MigrateFromConfig(_ map[string]string) (int, error) {
	return 0, nil
}

// Backend gibt den Backend-Namen zurück (plattformspezifisch in keyring_windows.go).
func (kp *KeyringProvider) Backend() string {
	return keyringBackendName()
}

// ── ChainedProvider ───────────────────────────────────────────────────────────

// ChainedProvider kombiniert mehrere SecretProvider mit Fallback-Logik.
//
// Ladereihenfolge für Get/GetAll: primär zuerst, dann Fallbacks.
// Schreiben (Set/SetBatch/Delete): nur in primären Provider.
//
// Typische Konfiguration (lokal):
//
//	ChainedProvider{
//	    primary:   KeyringProvider   // Windows Credential Manager
//	    fallbacks: [VaultProvider]   // AES-256-GCM Vault als Fallback
//	}
type ChainedProvider struct {
	primary   SecretProvider
	fallbacks []SecretProvider
}

// NewChainedProvider erstellt einen ChainedProvider.
// primary: wird bevorzugt für alle Lese- und Schreiboperationen.
// fallbacks: werden nacheinander befragt wenn primary einen leeren Wert liefert.
func NewChainedProvider(primary SecretProvider, fallbacks ...SecretProvider) *ChainedProvider {
	return &ChainedProvider{
		primary:   primary,
		fallbacks: fallbacks,
	}
}

// Get liest einen Secret – primär zuerst, dann Fallbacks.
func (cp *ChainedProvider) Get(key string) (string, error) {
	if v, err := cp.primary.Get(key); err == nil && v != "" {
		return v, nil
	}
	for _, fb := range cp.fallbacks {
		if v, err := fb.Get(key); err == nil && v != "" {
			return v, nil
		}
	}
	return "", nil
}

// Set schreibt nur in den primären Provider.
func (cp *ChainedProvider) Set(key, value string) error {
	return cp.primary.Set(key, value)
}

// Delete entfernt aus dem primären Provider.
func (cp *ChainedProvider) Delete(key string) error {
	return cp.primary.Delete(key)
}

// GetAll merged alle Provider – primäre Werte haben Vorrang.
func (cp *ChainedProvider) GetAll() (map[string]string, error) {
	// Fallbacks zuerst befragen (niedrigste Priorität)
	result := make(map[string]string)
	for i := len(cp.fallbacks) - 1; i >= 0; i-- {
		if all, err := cp.fallbacks[i].GetAll(); err == nil {
			for k, v := range all {
				if v != "" {
					result[k] = v
				}
			}
		}
	}
	// Primärer Provider überschreibt (höchste Priorität)
	if all, err := cp.primary.GetAll(); err == nil {
		for k, v := range all {
			if v != "" {
				result[k] = v
			}
		}
	}
	return result, nil
}

// SetBatch schreibt nur in den primären Provider.
func (cp *ChainedProvider) SetBatch(updates map[string]string) error {
	return cp.primary.SetBatch(updates)
}

// MigrateFromConfig delegiert an den primären Provider.
func (cp *ChainedProvider) MigrateFromConfig(secrets map[string]string) (int, error) {
	if m, ok := cp.primary.(interface {
		MigrateFromConfig(map[string]string) (int, error)
	}); ok {
		return m.MigrateFromConfig(secrets)
	}
	return 0, nil
}

// Backend gibt den Backend-String des primären Providers zurück.
func (cp *ChainedProvider) Backend() string {
	return cp.primary.Backend()
}

// ── Factory-Funktion ──────────────────────────────────────────────────────────

// NewSecretProvider wählt automatisch den richtigen Provider basierend auf Umgebung.
//
// Docker  → VaultProvider (AES-256-GCM Datei)
// Lokal   → ChainedProvider(KeyringProvider → VaultProvider)
//
// Gibt zusätzlich den Backend-Namen als String zurück (für Logging + Dashboard).
func NewSecretProvider(workspacePath string) (SecretProvider, error) {
	// Vault ist immer vorhanden (Docker + lokal als Fallback)
	vault, err := NewVaultProvider(workspacePath)
	if err != nil {
		return nil, err
	}

	if IsDockerEnvironment() {
		log.Println("[Secrets] Modus: Docker → AES-256-GCM Vault")
		return vault, nil
	}

	// Lokal: Keyring versuchen
	kr, err := NewKeyringProvider()
	if err != nil {
		log.Printf("[Secrets] ⚠️  System-Keyring nicht verfügbar (%v) → Vault-Fallback", err)
		return vault, nil
	}

	log.Println("[Secrets] Modus: Lokal → Keyring (primär) + Vault (Fallback)")
	return NewChainedProvider(kr, vault), nil
}

// ── Bekannte Vault-Keys ───────────────────────────────────────────────────────

// allKnownKeys gibt alle statisch bekannten FluxBot-Secret-Keys zurück.
// Wird von KeyringProvider.GetAll() verwendet da der Keyring keine Auflistung unterstützt.
func allKnownKeys() []string {
	return []string{
		// Kanäle
		"TELEGRAM_TOKEN", "DISCORD_TOKEN",
		"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN", "SLACK_SIGNING_SECRET",
		"MATRIX_TOKEN",
		"WHATSAPP_API_KEY", "WHATSAPP_WEBHOOK_SECRET",
		// AI Provider
		"PROVIDER_OPENROUTER", "PROVIDER_ANTHROPIC", "PROVIDER_OPENAI",
		"PROVIDER_GOOGLE", "PROVIDER_XAI", "PROVIDER_GROQ",
		"PROVIDER_MISTRAL", "PROVIDER_TOGETHER", "PROVIDER_DEEPSEEK",
		"PROVIDER_PERPLEXITY", "PROVIDER_COHERE", "PROVIDER_FIREWORKS",
		"PROVIDER_NOVITA", "PROVIDER_DEEPINFRA", "PROVIDER_CEREBRAS",
		"PROVIDER_OLLAMA", "PROVIDER_CUSTOM",
		"OLLAMA_BASE_URL",
		// Voice & Medien
		"VOICE_API_KEY",
		"IMG_OPENROUTER", "IMG_FAL", "IMG_OPENAI", "IMG_STABILITY",
		"IMG_TOGETHER", "IMG_REPLICATE",
		"VID_RUNWAY", "VID_KLING", "VID_LUMA", "VID_PIKA",
		"VID_HAILUO", "VID_SORA", "VID_VEO",
		// Google Workspace
		"GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET", "GOOGLE_REFRESH_TOKEN",
		// Cal.com
		"CALCOM_BASE_URL", "CALCOM_API_KEY", "CALCOM_OWNER_EMAIL", "CALCOM_EVENT_TYPE_ID",
		// SMTP
		"INTEG_SMTP_HOST", "INTEG_SMTP_PORT", "INTEG_SMTP_USER",
		"INTEG_SMTP_PASSWORD", "INTEG_SMTP_FROM",
		// System
		"SKILL_SECRET", "VIRUSTOTAL_API_KEY",
		"DASHBOARD_PASSWORD", "HMAC_SECRET",
	}
}
