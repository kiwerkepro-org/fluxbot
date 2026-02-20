package dashboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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
		Version:  "1.0.0",
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
		// Reload-Callback aufrufen (z.B. Image-Generators neu laden)
		if s.onReload != nil {
			go s.onReload()
		}
		writeJSON(w, map[string]string{"status": "ok", "message": "config.json gespeichert."})

	default:
		http.Error(w, "Methode nicht erlaubt", http.StatusMethodNotAllowed)
	}
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
