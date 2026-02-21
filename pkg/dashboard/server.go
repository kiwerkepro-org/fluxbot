package dashboard

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/ki-werke/fluxbot/pkg/security"
)

//go:embed dashboard.html
var static embed.FS

// Server ist das FluxBot Web-Dashboard.
// Startet einen HTTP-Server auf dem konfigurierten Port.
type Server struct {
	configPath    string
	workspacePath string
	password      string
	passwordMu    sync.RWMutex    // schützt Hot-Reload des Passworts
	port          int
	startTime     time.Time
	getChannels   func() []string         // Callback: liefert aktive Kanäle zur Laufzeit
	logPath       string                  // Pfad zur Terminal-Log-Datei (fluxbot.log)
	vault         *security.VaultProvider // Secret-Speicher (AES-256-GCM)
	onReload      func()                  // Callback: wird nach Config-Änderung aufgerufen
}

// New erstellt einen neuen Dashboard-Server.
// logPath: Pfad zur fluxbot.log – wenn leer, wird kein Terminal-Log angezeigt.
// vault: Secret-Speicher für API-Keys und Passwörter (AES-256-GCM).
// onReload: wird nach jeder Config- oder Secret-Änderung aufgerufen.
func New(configPath, workspacePath, password string, port int, getChannels func() []string, logPath string, vault *security.VaultProvider, onReload func()) *Server {
	return &Server{
		configPath:    configPath,
		workspacePath: workspacePath,
		password:      password,
		port:          port,
		startTime:     time.Now(),
		getChannels:   getChannels,
		logPath:       logPath,
		vault:         vault,
		onReload:      onReload,
	}
}

// UpdatePassword aktualisiert das Dashboard-Passwort zur Laufzeit (Hot-Reload).
func (s *Server) UpdatePassword(pass string) {
	s.passwordMu.Lock()
	defer s.passwordMu.Unlock()
	if pass != "" {
		s.password = pass
		log.Println("[Dashboard] Passwort aktualisiert.")
	}
}

// Start startet den Dashboard-HTTP-Server.
// Blockiert bis ctx abgebrochen wird.
func (s *Server) Start(ctx context.Context) {
	mux := http.NewServeMux()

	// ── UI (eingebettetes HTML) ───────────────────────────────────────────────
	mux.HandleFunc("/", s.auth(s.handleUI))

	// ── API-Endpunkte ─────────────────────────────────────────────────────────
	mux.HandleFunc("/api/status", s.auth(s.handleStatus))
	mux.HandleFunc("/api/config", s.auth(s.handleConfig))
	mux.HandleFunc("/api/secrets", s.auth(s.handleSecrets))
	mux.HandleFunc("/api/soul", s.auth(s.handleSoul))
	mux.HandleFunc("/api/logs", s.auth(s.handleLogs))
	mux.HandleFunc("/api/logs/terminal", s.auth(s.handleTerminalLogs))

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx)
	}()

	log.Printf("[Dashboard] Gestartet → http://localhost:%d", s.port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("[Dashboard] Server-Fehler: %v", err)
	}
}

// auth ist HTTP Basic Auth Middleware.
// Wenn kein Passwort konfiguriert ist, wird kein Auth geprüft.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.passwordMu.RLock()
		pw := s.password
		s.passwordMu.RUnlock()
		if pw != "" {
			_, pass, ok := r.BasicAuth()
			if !ok || pass != pw {
				w.Header().Set("WWW-Authenticate", `Basic realm="FluxBot Dashboard"`)
				http.Error(w, "Zugriff verweigert", http.StatusUnauthorized)
				return
			}
		}
		// CORS für lokale Entwicklung
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next(w, r)
	}
}

// handleUI liefert das eingebettete dashboard.html
func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	content, err := fs.ReadFile(static, "dashboard.html")
	if err != nil {
		http.Error(w, "Dashboard HTML nicht gefunden", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
}
