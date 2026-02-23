package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	vaultFileName  = ".secrets.vault"
	vaultKeyEnvVar = "FLUXBOT_VAULT_KEY"
)

// SecretProvider ist die Schnittstelle für sichere Secret-Speicherung.
// Implementierungen: VaultProvider (AES-256-GCM Datei), KeyringProvider (OS-Keyring),
// ChainedProvider (Keyring → Vault Fallback).
type SecretProvider interface {
	Get(key string) (string, error)
	Set(key string, value string) error
	Delete(key string) error
	GetAll() (map[string]string, error)
	SetBatch(updates map[string]string) error
	// MigrateFromConfig migriert Secrets aus einer Map in den Provider.
	// Schreibt nur Werte die noch nicht vorhanden sind (kein Überschreiben).
	// Gibt die Anzahl migrierter Einträge zurück.
	MigrateFromConfig(secrets map[string]string) (int, error)
	// Backend gibt den Anzeigenamen des aktiven Backends zurück.
	// Wird im Dashboard-Status angezeigt.
	Backend() string
}

// VaultProvider implementiert SecretProvider mit AES-256-GCM verschlüsselter Datei.
//
// Vault-Datei: {workspace}/.secrets.vault
// Schlüssel-Priorität:
//  1. Umgebungsvariable FLUXBOT_VAULT_KEY (32-Byte hex oder Passphrase)
//  2. Datei {workspace}/.vaultkey (auto-generiert beim ersten Start)
type VaultProvider struct {
	mu        sync.RWMutex
	vaultPath string
	key       []byte // 32-Byte AES-256 Schlüssel
}

// NewVaultProvider erstellt einen neuen VaultProvider.
// workspacePath: Verzeichnis für vault und key-Datei.
func NewVaultProvider(workspacePath string) (*VaultProvider, error) {
	vaultPath := filepath.Join(workspacePath, vaultFileName)
	keyFilePath := filepath.Join(workspacePath, ".vaultkey")

	key, err := loadOrCreateVaultKey(keyFilePath)
	if err != nil {
		return nil, fmt.Errorf("vault-schlüssel konnte nicht geladen werden: %w", err)
	}

	log.Printf("[Vault] ✅ Initialisiert – Backend: AES-256-GCM | %s", vaultPath)
	return &VaultProvider{
		vaultPath: vaultPath,
		key:       key,
	}, nil
}

// loadOrCreateVaultKey lädt oder generiert den AES-256 Vault-Schlüssel.
func loadOrCreateVaultKey(keyFilePath string) ([]byte, error) {
	// 1. Umgebungsvariable (für Docker / VPS)
	if envKey := os.Getenv(vaultKeyEnvVar); envKey != "" {
		decoded, err := hex.DecodeString(strings.TrimSpace(envKey))
		if err == nil && len(decoded) == 32 {
			log.Println("[Vault] Schlüssel aus Umgebungsvariable (hex) geladen.")
			return decoded, nil
		}
		// Als Passphrase: SHA-256 Hash
		h := sha256.Sum256([]byte(envKey))
		log.Println("[Vault] Schlüssel aus Umgebungsvariable (Passphrase) abgeleitet.")
		return h[:], nil
	}

	// 2. .vaultkey Datei
	if data, err := os.ReadFile(keyFilePath); err == nil {
		trimmed := strings.TrimSpace(string(data))
		if decoded, err := hex.DecodeString(trimmed); err == nil && len(decoded) == 32 {
			log.Printf("[Vault] Schlüssel aus %s geladen.", keyFilePath)
			return decoded, nil
		}
	}

	// 3. Neuen Schlüssel generieren und speichern
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("zufalls-schlüssel generierung fehlgeschlagen: %w", err)
	}
	keyHex := hex.EncodeToString(key)
	if err := os.WriteFile(keyFilePath, []byte(keyHex+"\n"), 0600); err != nil {
		return nil, fmt.Errorf("vault-schlüssel konnte nicht gespeichert werden: %w", err)
	}
	log.Printf("[Vault] ✅ Neuer Vault-Schlüssel generiert → %s", keyFilePath)
	log.Printf("[Vault]    Für Docker/VPS in docker-compose.yml eintragen:")
	log.Printf("[Vault]    FLUXBOT_VAULT_KEY=%s", keyHex)
	return key, nil
}

// ── Public API ────────────────────────────────────────────────────────────────

// Get liest einen Secret aus dem Vault.
func (vp *VaultProvider) Get(key string) (string, error) {
	vp.mu.RLock()
	defer vp.mu.RUnlock()
	data, err := vp.readVault()
	if err != nil {
		return "", err
	}
	return data[key], nil
}

// Set schreibt einen Secret in den Vault.
// Leerer Wert löscht den Key.
func (vp *VaultProvider) Set(key string, value string) error {
	vp.mu.Lock()
	defer vp.mu.Unlock()
	data, err := vp.readVault()
	if err != nil {
		data = make(map[string]string)
	}
	if value == "" {
		delete(data, key)
	} else {
		data[key] = value
	}
	return vp.writeVault(data)
}

// Delete entfernt einen Secret aus dem Vault.
func (vp *VaultProvider) Delete(key string) error {
	return vp.Set(key, "")
}

// GetAll gibt alle Secrets als Map zurück.
func (vp *VaultProvider) GetAll() (map[string]string, error) {
	vp.mu.RLock()
	defer vp.mu.RUnlock()
	return vp.readVault()
}

// SetBatch schreibt mehrere Secrets auf einmal (atomisch).
// Leere Werte werden ignoriert (vorhandene Werte bleiben erhalten).
func (vp *VaultProvider) SetBatch(updates map[string]string) error {
	vp.mu.Lock()
	defer vp.mu.Unlock()
	data, err := vp.readVault()
	if err != nil {
		data = make(map[string]string)
	}
	for key, value := range updates {
		if value != "" {
			data[key] = value
		}
	}
	return vp.writeVault(data)
}

// Backend gibt den Backend-Namen zurück.
func (vp *VaultProvider) Backend() string {
	return "AES-256-GCM Vault"
}

// MigrateFromConfig migriert Secrets aus einer Map in den Vault.
// Schreibt nur Werte die im Vault noch nicht vorhanden sind (kein Überschreiben).
// Gibt die Anzahl migrierter Einträge zurück.
func (vp *VaultProvider) MigrateFromConfig(secrets map[string]string) (int, error) {
	vp.mu.Lock()
	defer vp.mu.Unlock()
	data, err := vp.readVault()
	if err != nil {
		data = make(map[string]string)
	}
	migrated := 0
	for key, value := range secrets {
		if value != "" && data[key] == "" {
			data[key] = value
			migrated++
		}
	}
	if migrated > 0 {
		if err := vp.writeVault(data); err != nil {
			return 0, err
		}
	}
	return migrated, nil
}

// ── Interne Verschlüsselung (AES-256-GCM) ────────────────────────────────────

func (vp *VaultProvider) readVault() (map[string]string, error) {
	ciphertext, err := os.ReadFile(vp.vaultPath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]string), nil
		}
		return nil, err
	}

	block, err := aes.NewCipher(vp.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("vault-datei beschädigt (zu klein)")
	}
	nonce := ciphertext[:gcm.NonceSize()]
	ciphertext = ciphertext[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("vault konnte nicht entschlüsselt werden – falscher Schlüssel?")
	}

	var data map[string]string
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("vault JSON defekt: %w", err)
	}
	return data, nil
}

func (vp *VaultProvider) writeVault(data map[string]string) error {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(vp.key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return os.WriteFile(vp.vaultPath, ciphertext, 0600)
}
