package dashboard

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"
)

//go:embed dashboard.html
var static embed.FS

// Server ist das FluxBot Web-Dashboard.
// Startet einen HTTP-Server auf dem konfigurierten Port.
type Server struct {
	configPath    string
	workspacePath string
	password      string
	port          int
	startTime     time.Time
	getChannels   func() []string // Callback: liefert aktive Kanäle zur Laufzeit
}

// New erstellt einen neuen Dashboard-Server.
// getChannels ist ein Callback der die aktuell aktiven Channel-Namen zurückgibt.
func New(configPath, workspacePath, password string, port int, getChannels func() []string) *Server {
	return &Server{
		configPath:    configPath,
		workspacePath: workspacePath,
		password:      password,
		port:          port,
		startTime:     time.Now(),
		getChannels:   getChannels,
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
	mux.HandleFunc("/api/soul", s.auth(s.handleSoul))
	mux.HandleFunc("/api/logs", s.auth(s.handleLogs))

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
		if s.password != "" {
			_, pass, ok := r.BasicAuth()
			if !ok || pass != s.password {
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
