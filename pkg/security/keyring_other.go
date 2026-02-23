//go:build !windows

package security

// Stub-Implementierung für Linux und macOS.
//
// Auf diesen Plattformen ist der System-Keyring entweder headless nicht verfügbar
// (Linux Docker) oder erfordert CGo (macOS Keychain via Security.framework).
//
// FluxBot verwendet in diesem Fall ausschließlich den AES-256-GCM Vault.
// Wer auf macOS lokal entwickelt kann stattdessen den Vault direkt nutzen –
// eine CGo-freie macOS-Keychain-Implementierung ist für eine spätere Version geplant.
//
// keyringGet/keyringSet/keyringDelete/keyringEnumDynamic sind die plattformspezifischen
// Hilfsfunktionen die von keyring.go über den KeyringProvider aufgerufen werden.

// keyringGet gibt auf Non-Windows immer errKeyringNotFound zurück.
func keyringGet(_, _ string) (string, error) {
	return "", errKeyringUnsupported
}

// keyringSet ist auf Non-Windows ein No-Op.
func keyringSet(_, _, _ string) error {
	return errKeyringUnsupported
}

// keyringDelete ist auf Non-Windows ein No-Op.
func keyringDelete(_, _ string) error {
	return errKeyringUnsupported
}

// keyringEnumDynamic gibt auf Non-Windows eine leere Map zurück.
func keyringEnumDynamic(_ string) (map[string]string, error) {
	return map[string]string{}, nil
}

// keyringBackendName gibt den Plattform-Namen zurück.
func keyringBackendName() string {
	return "System-Keyring (nicht verfügbar – Vault aktiv)"
}
