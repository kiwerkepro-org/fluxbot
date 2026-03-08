// Package system stellt System-Dienste bereit: Auto-Update-Prüfung und -Installation.
package system

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	githubReleaseAPI = "https://api.github.com/repos/kiwerkepro-org/fluxbot/releases/latest"
	checkInterval    = 6 * time.Hour
)

// UpdateInfo enthält Informationen über verfügbare Updates.
type UpdateInfo struct {
	CurrentVersion string    `json:"current_version"`
	LatestVersion  string    `json:"latest_version"`
	UpdateAvailable bool     `json:"update_available"`
	DownloadURL    string    `json:"download_url"`
	ReleaseNotes   string    `json:"release_notes"`
	PublishedAt    time.Time `json:"published_at"`
	CheckedAt      time.Time `json:"checked_at"`
}

// githubRelease ist die Antwort der GitHub Releases API.
type githubRelease struct {
	TagName     string    `json:"tag_name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// Updater verwaltet Update-Checks und -Installationen.
type Updater struct {
	currentVersion string
	lastCheck      *UpdateInfo
	mu             sync.RWMutex
	httpClient     *http.Client
}

// New erstellt einen neuen Updater.
func New(currentVersion string) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// StartBackgroundCheck startet automatische Update-Prüfungen alle 6 Stunden.
func (u *Updater) StartBackgroundCheck(ctx context.Context) {
	go func() {
		// Ersten Check beim Start nach 30s verzögern
		select {
		case <-time.After(30 * time.Second):
		case <-ctx.Done():
			return
		}

		u.checkNow()

		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				u.checkNow()
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (u *Updater) checkNow() {
	info, err := u.fetchLatestRelease()
	if err != nil {
		log.Printf("[Updater] Update-Check fehlgeschlagen: %v", err)
		return
	}
	u.mu.Lock()
	u.lastCheck = info
	u.mu.Unlock()
	if info.UpdateAvailable {
		log.Printf("[Updater] Neue Version verfügbar: %s → %s", info.CurrentVersion, info.LatestVersion)
	}
}

// CheckUpdate führt einen sofortigen Update-Check durch und liefert das Ergebnis.
func (u *Updater) CheckUpdate() (*UpdateInfo, error) {
	info, err := u.fetchLatestRelease()
	if err != nil {
		return nil, err
	}
	u.mu.Lock()
	u.lastCheck = info
	u.mu.Unlock()
	return info, nil
}

// LastCheckResult liefert das Ergebnis des letzten Checks (ohne GitHub-Anfrage).
func (u *Updater) LastCheckResult() *UpdateInfo {
	u.mu.RLock()
	defer u.mu.RUnlock()
	if u.lastCheck == nil {
		return &UpdateInfo{
			CurrentVersion:  u.currentVersion,
			LatestVersion:   "",
			UpdateAvailable: false,
			CheckedAt:       time.Time{},
		}
	}
	return u.lastCheck
}

// fetchLatestRelease fragt die GitHub Releases API ab.
func (u *Updater) fetchLatestRelease() (*UpdateInfo, error) {
	req, err := http.NewRequest("GET", githubReleaseAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "FluxBot-Updater/1.0")

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP-Anfrage fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Private Repo oder kein Release vorhanden
		return &UpdateInfo{
			CurrentVersion:  u.currentVersion,
			LatestVersion:   u.currentVersion,
			UpdateAvailable: false,
			CheckedAt:       time.Now(),
		}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API Status: %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("JSON-Parsing fehlgeschlagen: %w", err)
	}

	latestVersion := rel.TagName
	updateAvailable := isNewerVersion(u.currentVersion, latestVersion)

	// Asset-URL für die aktuelle Plattform ermitteln
	assetName := platformAssetName()
	downloadURL := ""
	for _, asset := range rel.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	return &UpdateInfo{
		CurrentVersion:  u.currentVersion,
		LatestVersion:   latestVersion,
		UpdateAvailable: updateAvailable,
		DownloadURL:     downloadURL,
		ReleaseNotes:    rel.Body,
		PublishedAt:     rel.PublishedAt,
		CheckedAt:       time.Now(),
	}, nil
}

// InstallUpdate lädt die neue Binary herunter und ersetzt die aktuelle.
// Der Neustart muss vom Caller (z.B. Systemd/Task Scheduler) durchgeführt werden.
// Gibt den Pfad der heruntergeladenen Binary zurück.
func (u *Updater) InstallUpdate(downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("kein Download-URL für diese Plattform verfügbar")
	}

	// Aktuellen Executable-Pfad ermitteln
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("Executable-Pfad nicht ermittelbar: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("Symlink-Auflösung fehlgeschlagen: %w", err)
	}

	log.Printf("[Updater] Lade neue Version herunter von: %s", downloadURL)

	// Neue Binary in temporäre Datei laden
	tmpPath := exePath + ".update"
	if err := u.downloadFile(downloadURL, tmpPath); err != nil {
		return fmt.Errorf("Download fehlgeschlagen: %w", err)
	}

	// Ausführbar machen (Linux/macOS)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0755); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("chmod fehlgeschlagen: %w", err)
		}
	}

	// Alte Binary sichern
	backupPath := exePath + ".bak"
	os.Remove(backupPath) // Altes Backup löschen, ignoriere Fehler
	if err := os.Rename(exePath, backupPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("Backup der alten Binary fehlgeschlagen: %w", err)
	}

	// Neue Binary an den richtigen Platz verschieben
	if err := os.Rename(tmpPath, exePath); err != nil {
		// Rollback: alte Binary wiederherstellen
		os.Rename(backupPath, exePath)
		return fmt.Errorf("Installation fehlgeschlagen: %w", err)
	}

	log.Printf("[Updater] Update erfolgreich installiert → %s", exePath)
	log.Printf("[Updater] Backup der alten Version: %s", backupPath)
	log.Printf("[Updater] Bitte FluxBot neu starten um das Update zu aktivieren.")
	return nil
}

// downloadFile lädt eine Datei von einer URL herunter und speichert sie.
func (u *Updater) downloadFile(url, destPath string) error {
	resp, err := u.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Download-Status: %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// platformAssetName gibt den erwarteten GitHub-Asset-Namen für die aktuelle Plattform zurück.
// Konvention: fluxbot-windows-amd64.exe, fluxbot-linux-amd64, fluxbot-darwin-arm64, etc.
func platformAssetName() string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// arm64 → arm64, amd64 → amd64 (direkte Übernahme)
	name := fmt.Sprintf("fluxbot-%s-%s", os, arch)
	if os == "windows" {
		name += ".exe"
	}
	return name
}

// isNewerVersion prüft ob latestVersion neuer ist als currentVersion.
// Erwartet Semver-Format: "v1.2.3"
func isNewerVersion(current, latest string) bool {
	if current == "" || latest == "" {
		return false
	}
	if current == "dev" {
		return false // Entwicklungsversionen werden nicht upgedated
	}
	// Normalisieren: "v1.2.3" → [1, 2, 3]
	cur := parseVersion(current)
	lat := parseVersion(latest)

	for i := 0; i < 3; i++ {
		if lat[i] > cur[i] {
			return true
		}
		if lat[i] < cur[i] {
			return false
		}
	}
	return false
}

// parseVersion extrahiert [major, minor, patch] aus "v1.2.3".
func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	var result [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n := 0
		fmt.Sscanf(parts[i], "%d", &n)
		result[i] = n
	}
	return result
}
