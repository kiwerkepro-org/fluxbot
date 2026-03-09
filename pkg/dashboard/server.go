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

	"github.com/ki-werke/fluxbot/pkg/pairing"
	"github.com/ki-werke/fluxbot/pkg/security"
	"github.com/ki-werke/fluxbot/pkg/skills"
	"github.com/ki-werke/fluxbot/pkg/system"
)

//go:embed dashboard.html
//go:embed chat.html
var static embed.FS

// Server ist das FluxBot Web-Dashboard.
// Startet einen HTTP-Server auf dem konfigurierten Port.
type Server struct {
	configPath    string
	workspacePath string
	password      string
	passwordMu    sync.RWMutex    // schützt Hot-Reload des Passworts
	username      string
	usernameMu    sync.RWMutex    // schützt Hot-Reload des Benutzernamens
	hmacSecret    string
	hmacSecretMu  sync.RWMutex    // schützt Hot-Reload des HMAC-Secrets
	port          int
	startTime     time.Time
	version       string                  // Build-Version (per -ldflags "-X main.version=vX.Y.Z" gesetzt)
	getChannels   func() []string         // Callback: liefert aktive Kanäle zur Laufzeit
	logPath       string                  // Pfad zur Terminal-Log-Datei (fluxbot.log)
	vault         security.SecretProvider  // Secret-Speicher (Keyring / Vault / Chained)
	onReload      func()                  // Callback: wird nach Config-Änderung aufgerufen
	skillsLoader  *skills.Loader          // SkillsLoader für Skill-Verwaltung
	pairingStore  *pairing.Store          // Pairing-Store für DM-Pairing Mode (P9)
	sendToChannel func(channel, chatID, text string) error                          // Callback: Nachricht an Channel senden
	updater       *system.Updater                                                    // Auto-Update-System (P0)
	wsHandler     func(ctx context.Context, w http.ResponseWriter, r *http.Request) // P15: WebSocket-Handler
}

// New erstellt einen neuen Dashboard-Server.
// logPath: Pfad zur fluxbot.log – wenn leer, wird kein Terminal-Log angezeigt.
// vault: Secret-Speicher für API-Keys und Passwörter (AES-256-GCM).
// onReload: wird nach jeder Config- oder Secret-Änderung aufgerufen.
// hmacSecret: HMAC-Schlüssel für Dashboard-API-Request-Signierung (leer = deaktiviert).
// skillsLoader: SkillsLoader für Skill-Verwaltung (optional, kann nil sein).
// version: Build-Version ("dev" lokal, "vX.Y.Z" im Release via -ldflags).
func New(configPath, workspacePath, password, username string, port int, getChannels func() []string, logPath string, vault security.SecretProvider, onReload func(), hmacSecret string, skillsLoader *skills.Loader, version string, pairingStore *pairing.Store, sendToChannel func(channel, chatID, text string) error) *Server {
	if username == "" {
		username = "admin"
	}
	if version == "" {
		version = "dev"
	}
	return &Server{
		configPath:    configPath,
		workspacePath: workspacePath,
		password:      password,
		username:      username,
		hmacSecret:    hmacSecret,
		port:          port,
		startTime:     time.Now(),
		version:       version,
		getChannels:   getChannels,
		logPath:       logPath,
		vault:         vault,
		onReload:      onReload,
		skillsLoader:  skillsLoader,
		pairingStore:  pairingStore,
		sendToChannel: sendToChannel,
	}
}

// SetUpdater setzt den Auto-Updater nach der Initialisierung.
func (s *Server) SetUpdater(u *system.Updater) {
	s.updater = u
}

// SetWSHandler setzt den WebSocket-Handler für den Web-Chat (P15).
// Wird in main.go nach Erstellung des WebChannel aufgerufen.
func (s *Server) SetWSHandler(h func(ctx context.Context, w http.ResponseWriter, r *http.Request)) {
	s.wsHandler = h
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

// UpdateUsername aktualisiert den Dashboard-Benutzernamen zur Laufzeit (Hot-Reload).
func (s *Server) UpdateUsername(user string) {
	s.usernameMu.Lock()
	defer s.usernameMu.Unlock()
	if user != "" {
		s.username = user
		log.Println("[Dashboard] Benutzername aktualisiert.")
	}
}

// Start startet den Dashboard-HTTP-Server.
// Blockiert bis ctx abgebrochen wird.
func (s *Server) Start(ctx context.Context) {
	mux := http.NewServeMux()

	// ── UI (eingebettetes HTML – öffentlich, Auth erfolgt via JS Login-Overlay) ──
	mux.HandleFunc("/", s.handleUI)

	// ── Auth-Endpunkte ───────────────────────────────────────────────────────
	mux.HandleFunc("/api/auth/check", s.auth(s.handleAuthCheck))       // Credentials prüfen
	mux.HandleFunc("/api/auth/recover", s.handleAuthRecover)           // Passwort-Wiederherstellung (nur localhost)

	// ── API-Endpunkte (lesend – kein HMAC erforderlich) ─────────────────────
	mux.HandleFunc("/api/status", s.auth(s.handleStatus))
	mux.HandleFunc("/api/secrets/backend", s.auth(s.handleSecretBackend))
	mux.HandleFunc("/api/logs", s.auth(s.handleLogs))
	mux.HandleFunc("/api/logs/terminal", s.auth(s.handleTerminalLogs))
	mux.HandleFunc("/api/hmac-token", s.auth(s.handleHMACToken))
	mux.HandleFunc("/api/vt/status", s.auth(s.handleVTStatus))
	mux.HandleFunc("/api/vt/history", s.auth(s.handleVTHistory))
	mux.HandleFunc("/api/skills", s.auth(s.handleSkills))
	mux.HandleFunc("/api/source", s.auth(s.handleSourceCode))  // Self-Extend: Quellcode-Reading

	// ── Google OAuth2 (kein HMAC nötig – auth reicht; Callback ist öffentlich) ──
	mux.HandleFunc("/api/google/auth-url", s.auth(s.handleGoogleAuthURL))
	mux.HandleFunc("/api/google/oauth-callback", s.handleGoogleOAuthCallback) // Kein Auth (Google-Redirect)

	// ── API-Endpunkte (schreibend – HMAC-Signierung erforderlich) ───────────
	mux.HandleFunc("/api/config", s.auth(s.hmacVerify(s.handleConfig)))
	mux.HandleFunc("/api/secrets", s.auth(s.hmacVerify(s.handleSecrets)))
	mux.HandleFunc("/api/soul", s.auth(s.hmacVerify(s.handleSoul)))
	mux.HandleFunc("/api/vt/clear", s.auth(s.hmacVerify(s.handleVTClear)))
	mux.HandleFunc("/api/skills/sign", s.auth(s.handleSkillsSign))         // Kein HMAC (nur Dashboard-Operation, keine kritischen Daten)
	mux.HandleFunc("/api/skills/reload", s.auth(s.handleSkillsReload))   // Kein HMAC (nur Reload, kein State-Change)

	// ── Pairing API (P9: DM-Pairing Mode) ─────────────────────────────────
	mux.HandleFunc("/api/pairing", s.auth(s.hmacVerify(s.handlePairing)))       // GET: Liste, POST: Approve/Block/Remove (HMAC)
	mux.HandleFunc("/api/pairing/stats", s.auth(s.handlePairingStats))          // GET: Statistiken

	// ── Security API (P11: Dangerous-Tools Whitelist) ───────────────────────
	mux.HandleFunc("/api/security/dangerous-tools", s.auth(s.handleDangerousToolsStats)) // GET: Stats

	// ── System API (P0: Auto-Update) ────────────────────────────────────────
	mux.HandleFunc("/api/system/version", s.auth(s.handleSystemVersion))                            // GET: Version-Info + Update-Status
	mux.HandleFunc("/api/system/check-update", s.auth(s.handleSystemCheckUpdate))                   // POST: Sofortiger Update-Check
	mux.HandleFunc("/api/system/install-update", s.auth(s.hmacVerify(s.handleSystemInstallUpdate))) // POST: Update installieren (HMAC)
	mux.HandleFunc("/api/system/restart", s.auth(s.hmacVerify(s.handleSystemRestart)))              // POST: Neustart (HMAC)

	// ── Web-Chat (P15: Standalone Chat App) ─────────────────────────────────
	mux.HandleFunc("/chat", s.auth(s.handleChatUI))             // Chat-Frontend (HTML)
	mux.HandleFunc("/ws", s.auth(s.handleChatWS))               // WebSocket-Endpoint
	mux.HandleFunc("/chat.webmanifest", s.handleChatManifest)   // PWA-Manifest
	mux.HandleFunc("/chat-sw.js", s.handleChatServiceWorker)    // Service Worker

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
// Prüft Benutzername + Passwort. Wenn kein Passwort konfiguriert ist, wird kein Auth geprüft.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.passwordMu.RLock()
		pw := s.password
		s.passwordMu.RUnlock()
		if pw != "" {
			s.usernameMu.RLock()
			expectedUser := s.username
			s.usernameMu.RUnlock()

			user, pass, ok := r.BasicAuth()
			if !ok || pass != pw || (expectedUser != "" && user != expectedUser) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "Ungültige Anmeldedaten"})
				return
			}
		}
		// CORS für lokale Entwicklung
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next(w, r)
	}
}

// handleAuthCheck gibt 200 zurück wenn die Credentials korrekt sind (für JS Login-Check).
func (s *Server) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// handleAuthRecover gibt Benutzername + Passwort zurück – NUR von localhost erreichbar.
// Dient der Passwort-Wiederherstellung ohne Kommandozeile.
func (s *Server) handleAuthRecover(w http.ResponseWriter, r *http.Request) {
	// Nur localhost darf diese Route aufrufen
	ip := r.RemoteAddr
	if !strings.HasPrefix(ip, "127.0.0.1:") && !strings.HasPrefix(ip, "[::1]:") && ip != "127.0.0.1" && ip != "::1" {
		http.Error(w, "Nur von localhost erreichbar", http.StatusForbidden)
		return
	}
	s.passwordMu.RLock()
	pw := s.password
	s.passwordMu.RUnlock()
	s.usernameMu.RLock()
	user := s.username
	s.usernameMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"username": user,
		"password": pw,
	})
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
	w.Header().Set("Cache-Control", "no-store")
	w.Write(content)
}

// handleChatUI liefert das eingebettete chat.html (P15).
func (s *Server) handleChatUI(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(static, "chat.html")
	if err != nil {
		http.Error(w, "Chat HTML nicht gefunden", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(content)
}

// handleChatWS leitet WebSocket-Verbindungen an den WebChannel weiter (P15).
func (s *Server) handleChatWS(w http.ResponseWriter, r *http.Request) {
	if s.wsHandler == nil {
		http.Error(w, "Web-Chat nicht aktiv", http.StatusServiceUnavailable)
		return
	}
	s.wsHandler(r.Context(), w, r)
}

// handleChatManifest liefert das PWA-Manifest (P15).
func (s *Server) handleChatManifest(w http.ResponseWriter, r *http.Request) {
	manifest := map[string]interface{}{
		"name":             "FluxBot Chat",
		"short_name":       "Fluxi",
		"description":      "Dein KI-Assistent – direkt im Browser",
		"start_url":        "/chat",
		"scope":            "/chat",
		"display":          "standalone",
		"orientation":      "portrait-primary",
		"theme_color":      "#2563eb",
		"background_color": "#ffffff",
		"lang":             "de",
		"icons": []map[string]string{
			{
				"src":   "data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><rect width='100' height='100' rx='20' fill='%232563eb'/><text y='.9em' font-size='80' x='10'>🤖</text></svg>",
				"sizes": "192x192",
				"type":  "image/svg+xml",
			},
		},
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "max-age=3600")
	json.NewEncoder(w).Encode(manifest)
}

// handleChatServiceWorker liefert den PWA Service Worker (P15).
func (s *Server) handleChatServiceWorker(w http.ResponseWriter, r *http.Request) {
	sw := `
const CACHE = 'fluxbot-chat-v1';
self.addEventListener('install', e => e.waitUntil(caches.open(CACHE)));
self.addEventListener('fetch', e => {
  if (e.request.url.includes('/ws')) return;
  e.respondWith(fetch(e.request).catch(() => caches.match(e.request)));
});
`
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, sw)
}
