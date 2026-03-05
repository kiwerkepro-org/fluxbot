package dashboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	googlepkg "github.com/ki-werke/fluxbot/pkg/google"
	pairingpkg "github.com/ki-werke/fluxbot/pkg/pairing"
	"github.com/ki-werke/fluxbot/pkg/security"
)

// ── /api/status ───────────────────────────────────────────────────────────────

type statusResponse struct {
	Status   string   `json:"status"`
	Uptime   string   `json:"uptime"`
	Channels []string `json:"channels"`
	Version  string   `json:"version"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startTime)
	h := int(uptime.Hours())
	m := int(uptime.Minutes()) % 60
	sec := int(uptime.Seconds()) % 60

	var uptimeStr string
	if h > 0 {
		uptimeStr = fmt.Sprintf("%dh %dm %ds", h, m, sec)
	} else if m > 0 {
		uptimeStr = fmt.Sprintf("%dm %ds", m, sec)
	} else {
		uptimeStr = fmt.Sprintf("%ds", sec)
	}

	channels := []string{}
	if s.getChannels != nil {
		channels = s.getChannels()
	}

	writeJSON(w, statusResponse{
		Status:   "running",
		Uptime:   uptimeStr,
		Channels: channels,
		Version:  s.version,
	})
}

// ── /api/config ───────────────────────────────────────────────────────────────

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(s.configPath)
		if err != nil {
			httpError(w, "config.json konnte nicht gelesen werden", http.StatusInternalServerError)
			return
		}
		// JSON validieren und sauber zurückschicken
		var raw interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			httpError(w, "config.json ist ungültiges JSON", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(raw)

	case http.MethodPut:
		var raw interface{}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			httpError(w, "Ungültiges JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		// Backup der alten Config anlegen
		backupPath := s.configPath + ".bak"
		if data, err := os.ReadFile(s.configPath); err == nil {
			os.WriteFile(backupPath, data, 0600)
		}
		// Neue Config schreiben
		out, _ := json.MarshalIndent(raw, "", "  ")
		if err := os.WriteFile(s.configPath, out, 0600); err != nil {
			httpError(w, "config.json konnte nicht gespeichert werden: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// KEIN Reload hier – das Dashboard speichert immer config UND secrets (saveConfig() sendet beide
		// Requests). Der Reload passiert synchron nach dem POST /api/secrets, wenn die Vault-Werte
		// bereits geschrieben sind. Separater Reload hier würde alten Vault-Stand lesen → Race Condition.
		writeJSON(w, map[string]string{"status": "ok", "message": "config.json gespeichert."})

	default:
		http.Error(w, "Methode nicht erlaubt", http.StatusMethodNotAllowed)
	}
}

// ── /api/secrets ──────────────────────────────────────────────────────────────
//
// GET  → gibt alle gespeicherten Secrets zurück (Klartext, nur hinter Auth!)
// POST → schreibt Batch von Secrets in den Vault + löst Hot-Reload aus

func (s *Server) handleSecrets(w http.ResponseWriter, r *http.Request) {
	if s.vault == nil {
		httpError(w, "Vault nicht initialisiert", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		all, err := s.vault.GetAll()
		if err != nil {
			httpError(w, "Vault konnte nicht gelesen werden: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if all == nil {
			all = map[string]string{}
		}
		writeJSON(w, all)

	case http.MethodPost:
		var updates map[string]string
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			httpError(w, "Ungültiges JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.vault.SetBatch(updates); err != nil {
			httpError(w, "Vault konnte nicht geschrieben werden: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Hot-Reload: synchron (nicht go) – Vault ist jetzt geschrieben, Reload liest neue Werte.
		// Synchron = Antwort wird erst gesendet wenn Skills korrekt neu geladen sind.
		// Kein go = keine Race Condition mit dem parallelen PUT /api/config Reload.
		if s.onReload != nil {
			s.onReload()
		}
		writeJSON(w, map[string]string{"status": "ok", "message": "Secrets gespeichert."})

	default:
		http.Error(w, "Methode nicht erlaubt", http.StatusMethodNotAllowed)
	}
}

// ── /api/secrets/backend ──────────────────────────────────────────────────────

// handleSecretBackend gibt den aktiven Secret-Backend-Namen zurück.
// Wird vom Dashboard-Status-Tab angezeigt.
func (s *Server) handleSecretBackend(w http.ResponseWriter, r *http.Request) {
	backendName := "unbekannt"
	if s.vault != nil {
		backendName = s.vault.Backend()
	}
	isDocker := security.IsDockerEnvironment()
	writeJSON(w, map[string]interface{}{
		"backend":   backendName,
		"isDocker":  isDocker,
	})
}

// ── /api/soul ─────────────────────────────────────────────────────────────────

func (s *Server) handleSoul(w http.ResponseWriter, r *http.Request) {
	soulPath := filepath.Join(s.workspacePath, "SOUL.md")

	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(soulPath)
		if err != nil {
			// Datei existiert nicht – leeren String zurückgeben
			writeJSON(w, map[string]string{"content": ""})
			return
		}
		writeJSON(w, map[string]string{"content": string(data)})

	case http.MethodPut:
		var body struct {
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			httpError(w, "Ungültiges JSON", http.StatusBadRequest)
			return
		}
		if err := os.WriteFile(soulPath, []byte(body.Content), 0644); err != nil {
			httpError(w, "SOUL.md konnte nicht gespeichert werden: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok", "message": "SOUL.md gespeichert. Neustart erforderlich."})

	default:
		http.Error(w, "Methode nicht erlaubt", http.StatusMethodNotAllowed)
	}
}

// ── /api/logs ─────────────────────────────────────────────────────────────────

type logsResponse struct {
	Lines []string `json:"lines"`
	File  string   `json:"file"`
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	logsDir := filepath.Join(s.workspacePath, "logs")

	// Neueste Log-Datei finden
	entries, err := os.ReadDir(logsDir)
	if err != nil || len(entries) == 0 {
		writeJSON(w, logsResponse{Lines: []string{"Keine Log-Dateien gefunden."}, File: ""})
		return
	}

	// Nur audit-*.log Dateien, nach Name sortiert (= nach Datum)
	var logFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "audit-") {
			logFiles = append(logFiles, e.Name())
		}
	}
	if len(logFiles) == 0 {
		writeJSON(w, logsResponse{Lines: []string{"Keine Audit-Logs gefunden."}, File: ""})
		return
	}
	sort.Strings(logFiles)
	latestFile := logFiles[len(logFiles)-1]

	// Letzte 100 Zeilen lesen
	filePath := filepath.Join(logsDir, latestFile)
	lines, err := tailFile(filePath, 100)
	if err != nil {
		httpError(w, "Log konnte nicht gelesen werden: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, logsResponse{Lines: lines, File: latestFile})
}

// tailFile liest die letzten n Zeilen einer Datei
func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, scanner.Err()
}

// ── /api/logs/terminal ────────────────────────────────────────────────────────

// handleTerminalLogs liefert die letzten Zeilen der FluxBot-Terminal-Ausgabe (fluxbot.log).
func (s *Server) handleTerminalLogs(w http.ResponseWriter, r *http.Request) {
	if s.logPath == "" {
		writeJSON(w, logsResponse{Lines: []string{"Kein Terminal-Log konfiguriert."}, File: ""})
		return
	}
	lines, err := tailFile(s.logPath, 200)
	if err != nil {
		writeJSON(w, logsResponse{Lines: []string{"Terminal-Log noch nicht vorhanden – FluxBot wurde noch nicht gestartet."}, File: ""})
		return
	}
	writeJSON(w, logsResponse{Lines: lines, File: "fluxbot.log"})
}

// ── /api/vt/status  /api/vt/stats  /api/vt/history  /api/vt/clear ───────────

// handleVTStatus liefert VT-Status + Statistiken in einem kombinierten Response.
func (s *Server) handleVTStatus(w http.ResponseWriter, r *http.Request) {
	stats := security.GetStats()
	writeJSON(w, stats)
}

// handleVTHistory liefert die letzten Scan-Einträge (max 100).
func (s *Server) handleVTHistory(w http.ResponseWriter, r *http.Request) {
	history := security.GetHistory()
	if history == nil {
		history = []security.ScanEntry{}
	}
	writeJSON(w, history)
}

// handleVTClear löscht History und Statistiken (POST, HMAC-geschützt).
func (s *Server) handleVTClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Methode nicht erlaubt", http.StatusMethodNotAllowed)
		return
	}
	security.ClearHistory()
	writeJSON(w, map[string]string{"status": "ok", "message": "VT-History und Statistiken zurückgesetzt."})
}

// ── /api/google/auth-url ──────────────────────────────────────────────────────

// handleGoogleAuthURL gibt die OAuth2-Autorisierungs-URL zurück.
func (s *Server) handleGoogleAuthURL(w http.ResponseWriter, r *http.Request) {
	clientID, err := s.vault.Get("GOOGLE_CLIENT_ID")
	if err != nil || clientID == "" {
		writeJSON(w, map[string]string{"error": "GOOGLE_CLIENT_ID nicht im Vault."})
		return
	}
	redirectURI := "http://localhost:" + fmt.Sprintf("%d", s.port) + "/api/google/oauth-callback"
	scopes := strings.Join(googlepkg.AllScopes, " ")
	params := "client_id=" + clientID +
		"&redirect_uri=" + redirectURI +
		"&response_type=code" +
		"&scope=" + url.QueryEscape(scopes) +
		"&access_type=offline&prompt=consent"
	writeJSON(w, map[string]string{"url": "https://accounts.google.com/o/oauth2/v2/auth?" + params})
}

// ── /api/google/oauth-callback ────────────────────────────────────────────────

// handleGoogleOAuthCallback tauscht den Authorization-Code gegen Tokens.
func (s *Server) handleGoogleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		errMsg := r.URL.Query().Get("error")
		http.Error(w, "Kein Code empfangen. Google-Fehler: "+errMsg, http.StatusBadRequest)
		return
	}

	clientID, _ := s.vault.Get("GOOGLE_CLIENT_ID")
	clientSecret, _ := s.vault.Get("GOOGLE_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		http.Error(w, "GOOGLE_CLIENT_ID oder GOOGLE_CLIENT_SECRET fehlen im Vault.", http.StatusInternalServerError)
		return
	}

	redirectURI := "http://localhost:" + fmt.Sprintf("%d", s.port) + "/api/google/oauth-callback"

	// Token-Austausch via net/http
	data := "code=" + code +
		"&client_id=" + clientID +
		"&client_secret=" + clientSecret +
		"&redirect_uri=" + redirectURI +
		"&grant_type=authorization_code"

	resp, err := http.Post("https://oauth2.googleapis.com/token",
		"application/x-www-form-urlencoded",
		strings.NewReader(data))
	if err != nil {
		http.Error(w, "Token-Austausch fehlgeschlagen: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		http.Error(w, "Token-Decode fehlgeschlagen: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if tok.Error != "" {
		http.Error(w, "Google OAuth Fehler: "+tok.Error+" – "+tok.ErrorDesc, http.StatusBadRequest)
		return
	}
	if tok.RefreshToken == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<html><body style='font-family:sans-serif;padding:30px;background:#1a1a2e;color:#fff'><h2>⚠️ Kein Refresh Token erhalten</h2><p>Bitte widerrufe den App-Zugriff in deinen Google-Account-Einstellungen und versuche es erneut: <a href='https://myaccount.google.com/permissions' style='color:#5b8dee' target='_blank'>myaccount.google.com/permissions</a></p></body></html>")
		return
	}

	// Refresh Token im Vault speichern
	if err := s.vault.Set("GOOGLE_REFRESH_TOKEN", tok.RefreshToken); err != nil {
		http.Error(w, "Vault-Speichern fehlgeschlagen: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Hot-Reload auslösen
	if s.onReload != nil {
		s.onReload()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<html><body style="font-family:sans-serif;padding:30px;background:#1a1a2e;color:#fff">
<h2>✅ Google erfolgreich verbunden!</h2>
<p>Refresh Token wurde sicher im Vault gespeichert.</p>
<p>Du kannst dieses Fenster schließen und das Dashboard neu laden.</p>
<script>setTimeout(()=>{window.close()},3000)</script>
</body></html>`)
}

// ── /api/skills ────────────────────────────────────────────────────────────────

type skillResponse struct {
	Name           string `json:"name"`
	Path           string `json:"path"`
	NeedsResigning bool   `json:"needsResigning,omitempty"`
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	if s.skillsLoader == nil {
		writeJSON(w, map[string]interface{}{"skills": []skillResponse{}})
		return
	}

	skillList := s.skillsLoader.ListSkills()
	result := make([]skillResponse, 0, len(skillList))
	for _, skill := range skillList {
		// Filter: Nur .md Skills anzeigen, keine .sig oder andere Dateien
		if skill != nil && !strings.HasSuffix(skill.Name, ".sig") && !strings.HasPrefix(skill.Name, ".") {
			result = append(result, skillResponse{
				Name:           skill.Name,
				Path:           skill.Path,
				NeedsResigning: skill.NeedsResigning,
			})
		}
	}

	writeJSON(w, map[string]interface{}{"skills": result})
}

// ── /api/skills/sign ──────────────────────────────────────────────────────────────

type signSkillRequest struct {
	Skill string `json:"skill"`
}

func (s *Server) handleSkillsSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Nur POST erlaubt", http.StatusMethodNotAllowed)
		return
	}

	if s.skillsLoader == nil {
		httpError(w, "SkillsLoader nicht verfügbar", http.StatusServiceUnavailable)
		return
	}

	var req signSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, "Request-Parse fehlgeschlagen: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Skill == "" {
		httpError(w, "Skill-Name erforderlich", http.StatusBadRequest)
		return
	}

	// Skill neu signieren
	if err := s.skillsLoader.SignSkill(req.Skill); err != nil {
		httpError(w, "Signierung fehlgeschlagen: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Skill '%s' erfolgreich signiert", req.Skill),
	})
}

// ── /api/skills/reload ────────────────────────────────────────────────────────────

func (s *Server) handleSkillsReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, "Nur POST erlaubt", http.StatusMethodNotAllowed)
		return
	}

	if s.skillsLoader == nil {
		httpError(w, "SkillsLoader nicht verfügbar", http.StatusServiceUnavailable)
		return
	}

	// Alle Skills neu laden
	s.skillsLoader.Reload()

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "✅ Alle Skills erfolgreich neu geladen",
	})
}

// ── Pairing API (P9: DM-Pairing Mode) ─────────────────────────────────────────

// handlePairing verarbeitet GET (Liste) und POST (Approve/Block/Remove) Requests.
func (s *Server) handlePairing(w http.ResponseWriter, r *http.Request) {
	if s.pairingStore == nil {
		httpError(w, "Pairing-Store nicht initialisiert", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Filter-Parameter: ?status=pending|approved|blocked (leer = alle)
		statusFilter := r.URL.Query().Get("status")
		var entries interface{}
		switch statusFilter {
		case "pending", "approved", "blocked":
			entries = s.pairingStore.List(pairingpkg.PairStatus(statusFilter))
		default:
			entries = s.pairingStore.List("")
		}
		writeJSON(w, entries)

	case http.MethodPost:
		// Aktion: approve, block, remove, note
		var req struct {
			Action  string `json:"action"`  // "approve", "block", "remove", "note"
			Channel string `json:"channel"` // "telegram", "discord", etc.
			UserID  string `json:"userId"`  // User-ID
			Note    string `json:"note"`    // Admin-Notiz (nur für "note")
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpError(w, "Ungültiger Request", http.StatusBadRequest)
			return
		}
		if req.Channel == "" || req.UserID == "" {
			httpError(w, "channel und userId erforderlich", http.StatusBadRequest)
			return
		}

		var err error
		var msg string
		switch req.Action {
		case "approve":
			err = s.pairingStore.Approve(req.Channel, req.UserID)
			msg = "✅ User gepairt"
			// Benachrichtigung an den User senden
			if err == nil && s.sendToChannel != nil {
				entry := s.pairingStore.GetEntry(req.Channel, req.UserID)
				if entry != nil && entry.ChatID != "" {
					go s.sendToChannel(req.Channel, entry.ChatID,
						"✅ Pairing bestätigt! Du kannst jetzt mit mir chatten.")
				}
			}
		case "block":
			err = s.pairingStore.Block(req.Channel, req.UserID)
			msg = "🚫 User blockiert"
		case "remove":
			err = s.pairingStore.Remove(req.Channel, req.UserID)
			msg = "🗑️ Eintrag entfernt"
		case "note":
			err = s.pairingStore.SetNote(req.Channel, req.UserID, req.Note)
			msg = "📝 Notiz gespeichert"
		default:
			httpError(w, fmt.Sprintf("Unbekannte Aktion: %s", req.Action), http.StatusBadRequest)
			return
		}

		if err != nil {
			httpError(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]string{"message": msg})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handlePairingStats gibt Pairing-Statistiken zurück.
func (s *Server) handlePairingStats(w http.ResponseWriter, r *http.Request) {
	if s.pairingStore == nil {
		httpError(w, "Pairing-Store nicht initialisiert", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, s.pairingStore.Stats())
}

// ── Hilfsfunktionen ───────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
