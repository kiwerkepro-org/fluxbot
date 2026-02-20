package skills

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// Sign berechnet den HMAC-SHA256 für den gegebenen Inhalt und Secret.
func Sign(content, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(content))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify prüft ob die Signatur zum Inhalt und Secret passt.
func Verify(content, sig, secret string) bool {
	expected := Sign(content, secret)
	return hmac.Equal([]byte(expected), []byte(strings.TrimSpace(sig)))
}

// SignFile signiert eine Skill-Datei und schreibt die .sig-Datei daneben.
func SignFile(path, secret string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("skill lesen: %w", err)
	}
	sig := Sign(string(data), secret)
	return os.WriteFile(path+".sig", []byte(sig), 0644)
}

// VerifyFile prüft ob die .sig-Datei zur Skill-Datei passt.
// Rückgabe: (true, nil) = OK | (false, nil) = keine Signatur | (false, err) = Fehler
func VerifyFile(path, secret string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	sigData, err := os.ReadFile(path + ".sig")
	if os.IsNotExist(err) {
		return false, nil // unsigniert – kein Fehler
	}
	if err != nil {
		return false, err
	}
	return Verify(string(data), string(sigData), secret), nil
}
