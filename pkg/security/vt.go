package security

import (
	"crypto/sha256"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/VirusTotal/vt-go"
)

var (
	vtClient    *vt.Client
	scanEnabled bool
	// Cache für bereits gescannte Hashes
	scanCache  = make(map[string]bool)
	cacheMutex sync.RWMutex
	// Rate-Limiting für Public API (max 4/min)
	lastScan  time.Time
	scanMutex sync.Mutex
)

// InitVT initialisiert den VirusTotal-Client
func InitVT(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("VirusTotal API-Key fehlt")
	}
	vtClient = vt.NewClient(apiKey)
	scanEnabled = true
	log.Println("[Security-VT] VirusTotal Client initialisiert")
	return nil
}

// ScanFile ist die Hauptfunktion für Channel-Handler.
// Sie berechnet den Hash und prüft ihn bei VirusTotal.
func ScanFile(data []byte) (bool, error) {
	if !scanEnabled || vtClient == nil {
		return true, nil
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	return ScanFileHash(hash)
}

// ScanFileHash prüft einen SHA-256 Hash direkt
func ScanFileHash(hash string) (bool, error) {
	// 1. Cache-Check
	cacheMutex.RLock()
	isClean, found := scanCache[hash]
	cacheMutex.RUnlock()
	if found {
		log.Printf("[Security-VT] Hash im Cache: %s (Clean: %v)", hash, isClean)
		return isClean, nil
	}

	// 2. Rate-Limit Schutz
	scanMutex.Lock()
	elapsed := time.Since(lastScan)
	if elapsed < 15*time.Second {
		time.Sleep(15*time.Second - elapsed)
	}
	lastScan = time.Now()
	scanMutex.Unlock()

	log.Printf("[Security-VT] Scan-Anfrage für Hash: %s", hash)

	url := vt.URL("files/%s", hash)
	obj, err := vtClient.GetObject(url)
	if err != nil {
		// Bulletproof Fehler-Check: Wandelt den Fehler inkl. dynamischer Inhalte in einen String um
		errMsg := strings.ToLower(fmt.Sprintf("%v", err))

		// Datei unbekannt -> als sicher einstufen (da kein Treffer in der Datenbank)
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "notfounderror") {
			log.Printf("[Security-VT] Datei unbekannt (keine Bedrohung gelistet).")
			return true, nil
		}
		return false, fmt.Errorf("VT-API Fehler: %v", err)
	}

	// Analyse-Stats auslesen
	statsInterface, err := obj.Get("last_analysis_stats")
	if err != nil {
		return false, fmt.Errorf("Stats-Format ungültig: %v", err)
	}

	stats, ok := statsInterface.(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("Stats-Mapping fehlgeschlagen")
	}

	var malicious, suspicious float64
	if v, ok := stats["malicious"].(float64); ok {
		malicious = v
	}
	if v, ok := stats["suspicious"].(float64); ok {
		suspicious = v
	}

	isSafe := malicious == 0 && suspicious <= 2

	// 3. Cache aktualisieren
	cacheMutex.Lock()
	scanCache[hash] = isSafe
	cacheMutex.Unlock()

	if !isSafe {
		log.Printf("[Security-VT] 🚨 MALWARE ERKANNT! (M:%v S:%v)", malicious, suspicious)
	}
	return isSafe, nil
}
