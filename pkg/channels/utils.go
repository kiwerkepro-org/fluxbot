package channels

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// saveTempFile speichert Binärdaten in einer temporären Datei und gibt den Pfad zurück.
// Wird für alle Download-Dateitypen verwendet: PDFs, Fotos, Videos, etc.
func saveTempFile(data []byte, ext string) (string, error) {
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("fluxbot_media_%d%s", time.Now().UnixNano(), ext))
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return "", fmt.Errorf("temp-datei konnte nicht erstellt werden: %w", err)
	}
	return tmpPath, nil
}
