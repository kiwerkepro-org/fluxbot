package security

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/VirusTotal/vt-go"
)

// ScanEntry ist ein einzelner Eintrag in der Scan-History.
type ScanEntry struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`   // "file" | "url"
	Target  string    `json:"target"` // Hash (gekürzt) oder URL
	Safe    bool      `json:"safe"`
	Cached  bool      `json:"cached"`
}

// VTStats enthält aggregierte Scan-Statistiken.
type VTStats struct {
	Enabled      bool `json:"enabled"`
	TotalFiles   int  `json:"total_files"`
	TotalURLs    int  `json:"total_urls"`
	BlockedFiles int  `json:"blocked_files"`
	BlockedURLs  int  `json:"blocked_urls"`
	CacheHits    int  `json:"cache_hits"`
	HistoryLen   int  `json:"history_len"`
}

var (
	vtClient    *vt.Client
	scanEnabled bool
	// Cache für bereits gescannte Hashes
	scanCache  = make(map[string]bool)
	cacheMutex sync.RWMutex
	// Rate-Limiting für Public API (max 4/min → 15 Sekunden Abstand)
	lastScan  time.Time
	scanMutex sync.Mutex
	// Scan-History (in-memory, max 100 Einträge)
	scanHistory  []ScanEntry
	historyMutex sync.RWMutex
	// Statistik-Zähler
	statsFiles        int
	statsURLs         int
	statsBlockedFiles int
	statsBlockedURLs  int
	statsCacheHits    int
	statsMutex        sync.Mutex
)

const maxHistory = 100

// Einheitliche Benutzer-Warnmeldungen – alle Kanäle nutzen diese Konstanten.
const (
	VTFileBlockedMsg = "⚠️ Sicherheits-Warnung: Die gesendete Datei wurde als potenzielle Malware eingestuft und blockiert."
	VTURLBlockedMsg  = "⚠️ Sicherheits-Warnung: Der gesendete Link wurde als gefährlich eingestuft und blockiert."
)

// maxScanSize: Dateien größer als 32 MB werden nicht in den Speicher geladen.
// VirusTotal akzeptiert ohnehin nur Dateien bis 650 MB, für RAM-Schutz setzen wir 32 MB.
const maxScanSize = 32 * 1024 * 1024

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

// IsEnabled gibt zurück ob VT aktiv ist (für Dashboard-Status).
func IsEnabled() bool {
	return scanEnabled && vtClient != nil
}

// ScanFile ist die Hauptfunktion für Channel-Handler.
// Sie berechnet den SHA-256 Hash und prüft ihn bei VirusTotal.
// Gibt true zurück wenn die Datei sicher ist.
func ScanFile(data []byte) (bool, error) {
	if !scanEnabled || vtClient == nil {
		return true, nil
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	return ScanFileHash(hash)
}

// ScanFileHash prüft einen SHA-256 Hash direkt (ohne Datei herunterzuladen).
func ScanFileHash(hash string) (bool, error) {
	if !scanEnabled || vtClient == nil {
		return true, nil
	}

	// 1. Cache-Check
	cacheMutex.RLock()
	isClean, found := scanCache[hash]
	cacheMutex.RUnlock()
	if found {
		log.Printf("[Security-VT] Hash im Cache: %s (Clean: %v)", hash[:16]+"...", isClean)
		recordScan("file", hash[:16]+"...", isClean, true)
		return isClean, nil
	}

	// 2. Rate-Limit Schutz (Public API: max 4 Anfragen/Minute)
	scanMutex.Lock()
	elapsed := time.Since(lastScan)
	if elapsed < 15*time.Second {
		time.Sleep(15*time.Second - elapsed)
	}
	lastScan = time.Now()
	scanMutex.Unlock()

	log.Printf("[Security-VT] Datei-Scan für Hash: %s...", hash[:16])

	url := vt.URL("files/%s", hash)
	obj, err := vtClient.GetObject(url)
	if err != nil {
		errMsg := strings.ToLower(fmt.Sprintf("%v", err))
		// Datei unbekannt → kein Eintrag in VT-Datenbank → als sicher einstufen
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "notfounderror") {
			log.Printf("[Security-VT] Datei unbekannt (keine Bedrohung gelistet).")
			cacheMutex.Lock()
			scanCache[hash] = true
			cacheMutex.Unlock()
			return true, nil
		}
		return false, fmt.Errorf("VT-API Fehler: %v", err)
	}

	isSafe, err := extractSafetyFromStats(obj)
	if err != nil {
		return false, err
	}

	// 3. Cache aktualisieren
	cacheMutex.Lock()
	scanCache[hash] = isSafe
	cacheMutex.Unlock()

	if !isSafe {
		log.Printf("[Security-VT] 🚨 MALWARE ERKANNT! (Hash: %s...)", hash[:16])
	}
	recordScan("file", hash[:16]+"...", isSafe, false)
	return isSafe, nil
}

// ScanURL prüft eine URL bei VirusTotal auf Malware.
// Gibt true zurück wenn die URL als sicher gilt (oder VT nicht aktiv ist).
func ScanURL(rawURL string) (bool, error) {
	if !scanEnabled || vtClient == nil {
		return true, nil
	}

	// Rate-Limit Schutz (geteilt mit Datei-Scans)
	scanMutex.Lock()
	elapsed := time.Since(lastScan)
	if elapsed < 15*time.Second {
		time.Sleep(15*time.Second - elapsed)
	}
	lastScan = time.Now()
	scanMutex.Unlock()

	log.Printf("[Security-VT] URL-Scan: %s", rawURL)

	urlPath := vt.URL("urls/%s", vtURLIdentifier(rawURL))
	obj, err := vtClient.GetObject(urlPath)
	if err != nil {
		errMsg := strings.ToLower(fmt.Sprintf("%v", err))
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "notfounderror") {
			log.Printf("[Security-VT] URL unbekannt (keine Bedrohung gelistet): %s", rawURL)
			return true, nil
		}
		return false, fmt.Errorf("VT URL-API Fehler: %v", err)
	}

	isSafe, err := extractSafetyFromStats(obj)
	if err != nil {
		return false, err
	}

	if !isSafe {
		log.Printf("[Security-VT] 🚨 MALWARE-URL ERKANNT! URL: %s", rawURL)
	}
	// URL für die History kürzen (max 80 Zeichen)
	displayURL := rawURL
	if len(displayURL) > 80 {
		displayURL = displayURL[:77] + "..."
	}
	recordScan("url", displayURL, isSafe, false)
	return isSafe, nil
}

// ScanURLsInText extrahiert alle HTTP/HTTPS-URLs aus einem Text und scannt sie.
// Gibt (true, "", nil) zurück wenn alle URLs sicher sind.
// Gibt (false, badURL, nil) zurück wenn eine URL als gefährlich eingestuft wird.
func ScanURLsInText(text string) (bool, string, error) {
	urls := ExtractURLs(text)
	for _, u := range urls {
		isSafe, err := ScanURL(u)
		if err != nil {
			log.Printf("[Security-VT] URL-Scan Warnung für %s: %v", u, err)
			continue // Bei API-Fehler nicht blockieren
		}
		if !isSafe {
			return false, u, nil
		}
	}
	return true, "", nil
}

// ExtractURLs extrahiert HTTP/HTTPS-URLs aus einem Text (nach Leerzeichen getrennt).
func ExtractURLs(text string) []string {
	words := strings.Fields(text)
	var urls []string
	for _, word := range words {
		// Trailing-Interpunktion entfernen (z.B. "https://example.com," → "https://example.com")
		word = strings.TrimRight(word, ".,;:!?)")
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			urls = append(urls, word)
		}
	}
	return urls
}

// vtURLIdentifier berechnet den VirusTotal URL-Identifier.
// Format: base64url(sha256(url)) ohne Padding – entspricht VT API v3.
func vtURLIdentifier(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// recordScan fügt einen Eintrag zur Scan-History hinzu und aktualisiert die Statistiken.
func recordScan(scanType, target string, safe, cached bool) {
	statsMutex.Lock()
	if cached {
		statsCacheHits++
	} else if scanType == "file" {
		statsFiles++
		if !safe {
			statsBlockedFiles++
		}
	} else if scanType == "url" {
		statsURLs++
		if !safe {
			statsBlockedURLs++
		}
	}
	statsMutex.Unlock()

	entry := ScanEntry{
		Time:   time.Now(),
		Type:   scanType,
		Target: target,
		Safe:   safe,
		Cached: cached,
	}
	historyMutex.Lock()
	// Neuester Eintrag vorne
	scanHistory = append([]ScanEntry{entry}, scanHistory...)
	if len(scanHistory) > maxHistory {
		scanHistory = scanHistory[:maxHistory]
	}
	historyMutex.Unlock()
}

// GetStats gibt die aktuellen Scan-Statistiken zurück (thread-safe).
func GetStats() VTStats {
	statsMutex.Lock()
	s := VTStats{
		Enabled:      IsEnabled(),
		TotalFiles:   statsFiles,
		TotalURLs:    statsURLs,
		BlockedFiles: statsBlockedFiles,
		BlockedURLs:  statsBlockedURLs,
		CacheHits:    statsCacheHits,
	}
	statsMutex.Unlock()
	historyMutex.RLock()
	s.HistoryLen = len(scanHistory)
	historyMutex.RUnlock()
	return s
}

// GetHistory gibt eine Kopie der Scan-History zurück (thread-safe).
func GetHistory() []ScanEntry {
	historyMutex.RLock()
	defer historyMutex.RUnlock()
	result := make([]ScanEntry, len(scanHistory))
	copy(result, scanHistory)
	return result
}

// ClearHistory leert die Scan-History und setzt alle Statistik-Zähler zurück.
func ClearHistory() {
	historyMutex.Lock()
	scanHistory = nil
	historyMutex.Unlock()
	statsMutex.Lock()
	statsFiles = 0
	statsURLs = 0
	statsBlockedFiles = 0
	statsBlockedURLs = 0
	statsCacheHits = 0
	statsMutex.Unlock()
	log.Println("[Security-VT] History und Statistiken zurückgesetzt.")
}

// extractSafetyFromStats liest die last_analysis_stats aus einem VT-Objekt
// und gibt true zurück wenn keine Bedrohung erkannt wurde.
func extractSafetyFromStats(obj *vt.Object) (bool, error) {
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

	// Schwellenwert: 0 malicious, max 2 suspicious (False-Positive-Schutz)
	isSafe := malicious == 0 && suspicious <= 2
	if !isSafe {
		log.Printf("[Security-VT] Bedrohung erkannt (Malicious: %.0f, Suspicious: %.0f)", malicious, suspicious)
	}
	return isSafe, nil
}
