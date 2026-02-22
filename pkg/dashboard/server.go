package dashboard

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
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
	hmacSecret    string
	hmacSecretMu  sync.RWMutex    // schützt Hot-Reload des HMAC-Secrets
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
// hmacSecret: HMAC-Schlüssel für Dashboard-API-Request-Signierung (leer = deaktiviert).
func New(configPath, workspacePath, password string, port int, getChannels func() []string, logPath string, vault *security.VaultProvider, onReload func(), hmacSecret string) *Server {
	return &Server{
		configPath:    configPath,
		workspacePath: workspacePath,
		password:      password,
		hmacSecret:    hmacSecret,
		port:          port,
		startTime:     time.Now(),
		getChannels:   getChannels,
		logPath:       logPath,
		vault:         vault,
		onReload:      onReload,
	}
}

// UpdateHMACSecret aktualisiert das HMAC-Secret zur Laufzeit (Hot-Reload).
func (s *Server) UpdateHMACSecret(secret string) {
	s.hmacSecretMu.Lock()
	defer s.hmacSecretMu.Unlock()
	if secret != "" {
		s.hmacSecret = secret
		log.Println("[Dashboard] HMAC-Secret aktualisiert.")
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

	// ── API-Endpunkte (lesend – kein HMAC erforderlich) ─────────────────────
	mux.HandleFunc("/api/status", s.auth(s.handleStatus))
	mux.HandleFunc("/api/logs", s.auth(s.handleLogs))
	mux.HandleFunc("/api/logs/terminal", s.auth(s.handleTerminalLogs))
	mux.HandleFunc("/api/hmac-token", s.auth(s.handleHMACToken))
	mux.HandleFunc("/api/vt/status", s.auth(s.handleVTStatus))
	mux.HandleFunc("/api/vt/history", s.auth(s.handleVTHistory))

	// ── API-Endpunkte (schreibend – HMAC-Signierung erforderlich) ───────────
	mux.HandleFunc("/api/config", s.auth(s.hmacVerify(s.handleConfig)))
	mux.HandleFunc("/api/secrets", s.auth(s.hmacVerify(s.handleSecrets)))
	mux.HandleFunc("/api/soul", s.auth(s.hmacVerify(s.handleSoul)))
	mux.HandleFunc("/api/vt/clear", s.auth(s.hmacVerify(s.handleVTClear)))

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

// hmacVerify ist HMAC-SHA256 Middleware für schreibende API-Requests (POST, PUT, DELETE).
// GET-Requests werden ohne HMAC-Prüfung durchgelassen.
// Wenn kein HMAC-Secret konfiguriert ist, wird die Prüfung übersprungen (Abwärtskompatibilität).
// Payload: HMAC-SHA256("{timestamp}.{body}", secret) als Hex-String im Header X-Signature.
// Replay-Schutz: X-Timestamp muss Unix-Sekunden sein und darf max. 5 Minuten abweichen.
func (s *Server) hmacVerify(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// GET-Requests haben keinen Body – keine Signierung nötig
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next(w, r)
			return
		}

		s.hmacSecretMu.RLock()
		secret := s.hmacSecret
		s.hmacSecretMu.RUnlock()

		// Kein Secret konfiguriert → Middleware transparent (Abwärtskompatibilität)
		if secret == "" {
			next(w, r)
			return
		}

		// Timestamp aus Header lesen und Replay-Schutz prüfen
		tsStr := r.Header.Get("X-Timestamp")
		if tsStr == "" {
			http.Error(w, "X-Timestamp fehlt", http.StatusUnauthorized)
			return
		}
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			http.Error(w, "X-Timestamp ungültig", http.StatusUnauthorized)
			return
		}
		diff := math.Abs(float64(time.Now().Unix() - ts))
		if diff > 300 { // 5 Minuten
			http.Error(w, "X-Timestamp abgelaufen (Replay-Schutz)", http.StatusUnauthorized)
			return
		}

		// Signatur aus Header lesen
		sig := r.Header.Get("X-Signature")
		if sig == "" {
			http.Error(w, "X-Signature fehlt", http.StatusUnauthorized)
			return
		}

		// Body lesen (und für Handler wiederherstellen)
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Body konnte nicht gelesen werden", http.StatusBadRequest)
			return
		}

		// HMAC-Payload: "{timestamp}.{body}"
		payload := fmt.Sprintf("%s.%s", tsStr, string(bodyBytes))
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(payload))
		expected := hex.EncodeToString(mac.Sum(nil))

		if !hmac.Equal([]byte(sig), []byte(expected)) {
			log.Printf("[Dashboard] ⚠️ HMAC-Verifikation fehlgeschlagen: %s %s", r.Method, r.URL.Path)
			http.Error(w, "Ungültige Signatur", http.StatusUnauthorized)
			return
		}

		// Body für den eigentlichen Handler wiederherstellen
		r.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
		next(w, r)
	}
}

// handleHMACToken liefert das HMAC-Secret an das authentifizierte Frontend.
// Das Secret wird nur übertragen wenn HMAC aktiviert ist.
func (s *Server) handleHMACToken(w http.ResponseWriter, r *http.Request) {
	s.hmacSecretMu.RLock()
	secret := s.hmacSecret
	s.hmacSecretMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled": secret != "",
		"secret":  secret,
	})
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
