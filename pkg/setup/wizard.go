package setup

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

//go:embed wizard.html
var wizardHTML embed.FS

// isDockerEnv gibt true zurück wenn FluxBot innerhalb eines Docker-Containers läuft.
// Erkennungsmerkmale: /.dockerenv-Datei oder FLUXBOT_DOCKER=1 Umgebungsvariable.
func isDockerEnv() bool {
	if os.Getenv("FLUXBOT_DOCKER") == "1" {
		return true
	}
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// RunWizard startet den Browser-basierten Einrichtungsassistenten.
// Blockiert solange bis der User die Einrichtung abgeschlossen hat.
// Schreibt danach die config.json und kehrt zurück.
//
// Verhaltensunterschiede:
//   - Nativ (.exe / direktes Binary):
//     → zufälliger freier Port, lauscht auf 127.0.0.1, öffnet Browser automatisch
//   - Docker:
//     → fester Port 8090 (entspricht dem gemappten Port), lauscht auf 0.0.0.0,
//     → kein automatischer Browser-Start, URL wird prominent geloggt
func RunWizard(configPath string) error {
	docker := isDockerEnv()

	var listenAddr string
	var port int

	if docker {
		// In Docker: fester Port 8090, alle Interfaces (damit der Host-Browser
		// über den gemappten Port erreichbar ist)
		port = 8090
		listenAddr = fmt.Sprintf("0.0.0.0:%d", port)
	} else {
		// Nativ: zufälliger freier Port, nur Loopback
		p, err := freePort()
		if err != nil {
			return fmt.Errorf("kein freier Port verfügbar: %w", err)
		}
		port = p
		listenAddr = fmt.Sprintf("127.0.0.1:%d", port)
	}

	done := make(chan error, 1)
	mux := http.NewServeMux()

	// ── UI ───────────────────────────────────────────────────────────────────
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		content, err := fs.ReadFile(wizardHTML, "wizard.html")
		if err != nil {
			http.Error(w, "Wizard HTML nicht gefunden", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(content)
	})

	// ── API: Konfiguration speichern ─────────────────────────────────────────
	mux.HandleFunc("/api/finish", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		var cfg interface{}
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "Ungültige Daten: "+err.Error(), http.StatusBadRequest)
			return
		}

		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			http.Error(w, "JSON-Fehler: "+err.Error(), http.StatusInternalServerError)
			go func() { time.Sleep(500 * time.Millisecond); done <- err }()
			return
		}

		// Verzeichnis anlegen falls nicht vorhanden (z.B. erster Start)
		if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
			http.Error(w, "Konnte Verzeichnis nicht anlegen: "+err.Error(), http.StatusInternalServerError)
			go func() { time.Sleep(500 * time.Millisecond); done <- err }()
			return
		}

		if err := os.WriteFile(configPath, data, 0600); err != nil {
			http.Error(w, "Konnte config.json nicht schreiben: "+err.Error(), http.StatusInternalServerError)
			go func() { time.Sleep(500 * time.Millisecond); done <- err }()
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))

		// Kurz warten damit der Browser die Antwort empfängt, dann beenden
		go func() {
			time.Sleep(800 * time.Millisecond)
			done <- nil
		}()
	})

	srv := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
		// Kein ReadTimeout/WriteTimeout – der User soll sich beim Ausfüllen
		// so viel Zeit lassen wie er braucht. Ein kurzes Timeout würde die
		// Keep-Alive-Verbindung schließen und beim POST "Failed to fetch" auslösen.
		IdleTimeout: 30 * time.Minute,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[Setup] Server-Fehler: %v", err)
		}
	}()

	// Kurz warten damit der Server hochfährt
	time.Sleep(300 * time.Millisecond)

	url := fmt.Sprintf("http://localhost:%d", port)

	if docker {
		// Docker: kein Auto-Browser, aber sehr deutliche Log-Ausgabe
		log.Println("[Setup] ╔══════════════════════════════════════════════════╗")
		log.Println("[Setup] ║          FluxBot – Einrichtungsassistent         ║")
		log.Println("[Setup] ╠══════════════════════════════════════════════════╣")
		log.Println("[Setup] ║                                                  ║")
		log.Printf("[Setup] ║  👉  %-44s║", url+"  ")
		log.Println("[Setup] ║                                                  ║")
		log.Println("[Setup] ║  Öffne diese URL in deinem Browser um            ║")
		log.Println("[Setup] ║  FluxBot einzurichten.                           ║")
		log.Println("[Setup] ║                                                  ║")
		log.Println("[Setup] ╚══════════════════════════════════════════════════╝")
	} else {
		log.Println("[Setup] ╔══════════════════════════════════════════╗")
		log.Printf("[Setup] ║  Einrichtungsassistent gestartet         ║")
		log.Printf("[Setup] ║  %-41s║", url+" ")
		log.Println("[Setup] ║  Browser öffnet sich automatisch...      ║")
		log.Println("[Setup] ╚══════════════════════════════════════════╝")
		openBrowser(url)
	}

	// Blockieren bis Setup abgeschlossen oder Fehler
	result := <-done

	shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)

	if result == nil {
		log.Println("[Setup] ✅ Konfiguration gespeichert – FluxBot wird gestartet...")
	}
	return result
}

// freePort findet einen freien TCP-Port auf localhost.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// openBrowser öffnet die URL im Standard-Browser des Betriebssystems.
func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}

	if err := exec.Command(cmd, args...).Start(); err != nil {
		log.Printf("[Setup] Browser konnte nicht automatisch geöffnet werden.")
		log.Printf("[Setup] → Bitte manuell öffnen: %s", url)
	}
}
