package channels

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ki-werke/fluxbot/pkg/security"
)

const (
	slackAPIBase    = "https://slack.com/api"
	slackWebhookPort = 3000
)

// SlackConfig enthält die Konfiguration für den Slack-Kanal
type SlackConfig struct {
	BotToken      string   // xoxb-... Bot OAuth Token
	AppToken      string   // Wird für Socket Mode verwendet (hier nicht genutzt)
	SigningSecret string   // Slack Signing Secret für HMAC-Verifizierung
	WebhookPort   int      // Port für Events API Webhook (default: 3000)
	AllowFrom     []string // User-IDs oder Channel-IDs die erlaubt sind
}

// SlackChannel implementiert den Slack-Kanal via Events API (HTTP Webhook)
//
// Setup in api.slack.com:
//  1. App erstellen → Features → Event Subscriptions aktivieren
//  2. Request URL: http://dein-server:3000/slack/events
//  3. Subscribe to bot events: message.im, message.channels, message.files
//  4. Bot Token Scopes: chat:write, im:history, channels:history, files:read
type SlackChannel struct {
	cfg    SlackConfig
	bus    chan<- Message
	server *http.Server
	allow  map[string]bool
}

// NewSlackChannel erstellt einen neuen Slack-Kanal
func NewSlackChannel(cfg SlackConfig) *SlackChannel {
	allow := make(map[string]bool)
	for _, a := range cfg.AllowFrom {
		allow[a] = true
	}
	if cfg.WebhookPort == 0 {
		cfg.WebhookPort = slackWebhookPort
	}
	return &SlackChannel{cfg: cfg, allow: allow}
}

func (s *SlackChannel) Name() string { return "slack" }

// Start startet den Events-API-Webhook-Server
func (s *SlackChannel) Start(_ context.Context, bus chan<- Message) error {
	s.bus = bus

	mux := http.NewServeMux()
	mux.HandleFunc("/slack/events", s.handleEvent)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.cfg.WebhookPort),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("[Slack] Events-API-Server startet auf Port %d", s.cfg.WebhookPort)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[Slack] Server-Fehler: %v", err)
		}
	}()

	log.Println("[Slack] Kanal gestartet")
	return nil
}

func (s *SlackChannel) Stop() {
	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.server.Shutdown(ctx)
	}
}

// Send sendet eine Nachricht über die Slack Web API
func (s *SlackChannel) Send(channelID string, text string) error {
	chunks := splitMessage(text, 3000) // Slack: max ~3000 Zeichen pro Block
	for _, chunk := range chunks {
		if err := s.sendChunk(channelID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (s *SlackChannel) sendChunk(channelID, text string) error {
	payload := map[string]string{
		"channel": channelID,
		"text":    text,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", slackAPIBase+"/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.BotToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("slack: API nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.OK {
		return fmt.Errorf("slack: API Fehler: %s", result.Error)
	}
	return nil
}

// TypingIndicator – Slack unterstützt kein Typing-Indikator via API
func (s *SlackChannel) TypingIndicator(_ string) {}

// handleEvent verarbeitet Slack Events API Anfragen
func (s *SlackChannel) handleEvent(rw http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(rw, "Bad Request", http.StatusBadRequest)
		return
	}

	// Slack Signatur prüfen
	if s.cfg.SigningSecret != "" && !s.verifySlackSignature(r, body) {
		log.Println("[Slack] Ungültige Signatur")
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var event slackEventPayload
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(rw, "Bad Request", http.StatusBadRequest)
		return
	}

	// URL-Verifizierung (Slack prüft die URL beim Setup)
	if event.Type == "url_verification" {
		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]string{"challenge": event.Challenge})
		return
	}

	// Schnell antworten (Slack erwartet <3 Sekunden)
	rw.WriteHeader(http.StatusOK)

	if event.Type != "event_callback" {
		return
	}

	e := event.Event
	// Bot-Nachrichten ignorieren
	if e.BotID != "" || e.SubType == "bot_message" {
		return
	}

	// Whitelist prüfen
	if len(s.allow) > 0 && !s.allow[e.User] && !s.allow[e.Channel] {
		log.Printf("[Slack] Nachricht von %s verworfen (nicht in Whitelist)", e.User)
		return
	}

	// ── File-Share: Dateien scannen ────────────────────────────────────────────
	if e.SubType == "file_share" && len(e.Files) > 0 {
		for _, f := range e.Files {
			if f.URLPrivateDownload == "" {
				log.Printf("[Slack] Datei '%s' hat keine Download-URL (möglicherweise noch nicht verfügbar)", f.Name)
				continue
			}

			data, err := s.downloadSlackFile(f.URLPrivateDownload)
			if err != nil {
				log.Printf("[Slack] File-Download Fehler für '%s': %v", f.Name, err)
				continue
			}

			isSafe, err := security.ScanFile(data)
			if err != nil {
				log.Printf("[Slack/Security] Scan-Warnung für '%s': %v", f.Name, err)
				// Bei VT-API-Fehler: nicht blockieren
			}
			if !isSafe {
				log.Printf("[Slack/Security] 🚨 Blockiere bösartige Datei von %s: %s", e.User, f.Name)
				s.Send(e.Channel, security.VTFileBlockedMsg)
				return
			}
			log.Printf("[Slack/Security] ✅ Datei sicher: %s", f.Name)
		}
	}

	// ── Textnachrichten + URL-Scan ─────────────────────────────────────────────
	if e.Type == "message" || e.Type == "app_mention" {
		text := strings.TrimSpace(removeSlackMention(e.Text))
		if text == "" {
			return
		}

		// URL-Scan
		isSafe, badURL, err := security.ScanURLsInText(text)
		if err != nil {
			log.Printf("[Slack/Security] URL-Scan Warnung: %v", err)
		}
		if !isSafe {
			log.Printf("[Slack/Security] 🚨 Bösartige URL von %s: %s", e.User, badURL)
			s.Send(e.Channel, security.VTURLBlockedMsg)
			return
		}

		msg := Message{
			ID:        e.TS,
			ChannelID: "slack",
			ChatID:    e.Channel,
			SenderID:  e.User,
			Type:      MessageTypeText,
			Text:      text,
		}

		log.Printf("[Slack] Nachricht von %s: %.50s", e.User, text)
		if s.bus != nil {
			s.bus <- msg
		}
	}
}

// downloadSlackFile lädt eine private Slack-Datei mit dem Bot-Token herunter.
// Slack-Dateien erfordern einen Authorization-Header (Bearer Token).
func (s *SlackChannel) downloadSlackFile(fileURL string) ([]byte, error) {
	req, err := http.NewRequest("GET", fileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.BotToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack: file-download fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("slack: file-download HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (s *SlackChannel) verifySlackSignature(r *http.Request, body []byte) bool {
	ts := r.Header.Get("X-Slack-Request-Timestamp")
	sig := r.Header.Get("X-Slack-Signature")
	baseStr := fmt.Sprintf("v0:%s:%s", ts, string(body))
	mac := hmac.New(sha256.New, []byte(s.cfg.SigningSecret))
	mac.Write([]byte(baseStr))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

func removeSlackMention(text string) string {
	// <@USERID> entfernen
	for strings.Contains(text, "<@") {
		start := strings.Index(text, "<@")
		end := strings.Index(text[start:], ">")
		if end < 0 {
			break
		}
		text = text[:start] + text[start+end+1:]
	}
	return text
}

// Slack Event Payload Strukturen
type slackEventPayload struct {
	Type      string     `json:"type"`
	Challenge string     `json:"challenge"`
	Event     slackEvent `json:"event"`
	TeamID    string     `json:"team_id"`
}

type slackEvent struct {
	Type    string      `json:"type"`
	SubType string      `json:"subtype"`
	Text    string      `json:"text"`
	User    string      `json:"user"`
	Channel string      `json:"channel"`
	TS      string      `json:"ts"`
	BotID   string      `json:"bot_id"`
	Files   []slackFile `json:"files"` // Für file_share Events
}

// slackFile repräsentiert eine in Slack hochgeladene Datei
type slackFile struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	URLPrivateDownload string `json:"url_private_download"` // Download-URL (benötigt Auth)
	Mimetype           string `json:"mimetype"`
	Size               int64  `json:"size"`
}

// SendPhoto ist noch nicht implementiert für slack
func (s *SlackChannel) SendPhoto(_ string, _ string, _ string) error {
	return fmt.Errorf("slack sendPhoto noch nicht implementiert")
}
