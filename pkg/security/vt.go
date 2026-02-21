package security

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/VirusTotal/vt-go"
)

var (
	vtClient    *vt.Client
	scanEnabled bool
	// Cache für bereits gescannte Hashes (Vermeidung von Doppel-Scans)
	scanCache  = make(map[string]bool)
	cacheMutex sync.RWMutex
	// Einfaches Rate-Limiting für Public API (4 Requests pro Minute)
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

// ScanFileHash prüft einen SHA-256 Hash bei VirusTotal
func ScanFileHash(hash string) (bool, error) {
	if !scanEnabled || vtClient == nil {
		return true, nil // Wenn nicht aktiv, lassen wir die Datei durch
	}

	// 1. Im Cache nachsehen
	cacheMutex.RLock()
	isClean, found := scanCache[hash]
	cacheMutex.RUnlock()
	if found {
		log.Printf("[Security-VT] Hash im lokalen Cache gefunden: %s (Clean: %v)", hash, isClean)
		return isClean, nil
	}

	// 2. Rate-Limiting (Pause einlegen, wenn der letzte Scan zu kurz her ist)
	scanMutex.Lock()
	elapsed := time.Since(lastScan)
	if elapsed < 15*time.Second { // Max 4 pro Minute -> alle 15 Sek einer
		time.Sleep(15*time.Second - elapsed)
	}
	lastScan = time.Now()
	scanMutex.Unlock()

	log.Printf("[Security-VT] Frage VirusTotal für Hash ab: %s", hash)

	// API Abfrage
	url := vt.URL("files/%s", hash)
	obj, err := vtClient.Get(context.Background(), url)
	if err != nil {
		// Wenn der Hash unbekannt ist (404), stufen wir ihn als "unbekannt/sicher" ein
		// oder lassen ihn zur Analyse hochladen (Zukunft-Feature)
		if err.Error() == "NotFoundError" || err.Error() == "not_found" {
			log.Printf("[Security-VT] Datei bei VT unbekannt (neu).")
			return true, nil
		}
		return false, fmt.Errorf("VT-API Fehler: %v", err)
	}

	// Ergebnisse auswerten
	stats, err := obj.GetMap("last_analysis_stats")
	if err != nil {
		return false, fmt.Errorf("konnte Analyse-Stats nicht lesen: %v", err)
	}

	malicious := stats["malicious"].(float64)
	suspicious := stats["suspicious"].(float64)

	isSafe := malicious == 0 && suspicious <= 2 // Kleine Toleranz für Fehlalarme (max 2 suspicious)

	// 3. Ergebnis in den Cache schreiben
	cacheMutex.Lock()
	scanCache[hash] = isSafe
	cacheMutex.Unlock()

	if !isSafe {
		log.Printf("[Security-VT] 🚨 WARNUNG: Datei als bösartig eingestuft! (Malicious: %v, Suspicious: %v)", malicious, suspicious)
	} else {
		log.Printf("[Security-VT] Datei ist sauber. (Malicious: 0, Suspicious: %v)", suspicious)
	}

	return isSafe, nil
}
