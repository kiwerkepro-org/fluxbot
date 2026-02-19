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
)

const (
	whatsappGraphURL = "https://graph.facebook.com/v19.0"
)

// WhatsAppConfig enthält die Konfiguration für den WhatsApp-Kanal
type WhatsAppConfig struct {
	Provider      string
	PhoneNumber   string
	PhoneNumberID string
	APIKey        string
	WebhookSecret string
	WebhookPort   int
	AllowFrom     []string
}

// WhatsAppChannel implementiert den WhatsApp Meta Business Cloud API Kanal
type WhatsAppChannel struct {
	cfg    WhatsAppConfig
	bus    chan<- Message
	server *http.Server
	allow  map[string]bool
}

// NewWhatsAppChannel erstellt einen neuen WhatsApp-Kanal
func NewWhatsAppChannel(cfg WhatsAppConfig) *WhatsAppChannel {
	allow := make(map[string]bool)
	for _, a := range cfg.AllowFrom {
		allow[normalizePhone(a)] = true
	}
	return &WhatsAppChannel{cfg: cfg, allow: allow}
}

func (w *WhatsAppChannel) Name() string { return "whatsapp" }

// Start startet den Webhook-HTTP-Server
func (w *WhatsAppChannel) Start(_ context.Context, bus chan<- Message) error {
	w.bus = bus
	port := w.cfg.WebhookPort
	if port == 0 {
		port = 8443
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", w.handleWebhook)

	w.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("[WhatsApp] Webhook-Server startet auf Port %d", port)
		if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[WhatsApp] Webhook-Server Fehler: %v", err)
		}
	}()

	log.Printf("[WhatsApp] Kanal gestartet | PhoneNumberID: %s", w.cfg.PhoneNumberID)
	return nil
}

// Stop beendet den Webhook-Server
func (w *WhatsAppChannel) Stop() {
	if w.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		w.server.Shutdown(ctx)
	}
}

// Send sendet eine WhatsApp-Nachricht über die Meta Cloud API
func (w *WhatsAppChannel) Send(to string, text string) error {
	// Lange Nachrichten aufteilen (WhatsApp: max 4096 Zeichen)
	chunks := splitMessage(text, 4096)
	for _, chunk := range chunks {
		if err := w.sendChunk(to, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (w *WhatsAppChannel) sendChunk(to, text string) error {
	payload := map[string]interface{}{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text":              map[string]string{"body": text},
	}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/%s/messages", whatsappGraphURL, w.cfg.PhoneNumberID)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("whatsapp: request-Fehler: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+w.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("whatsapp: API nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("whatsapp: API Fehler %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// TypingIndicator – WhatsApp Business API unterstützt kein Typing-Indikator via API
func (w *WhatsAppChannel) TypingIndicator(_ string) {}

// handleWebhook verarbeitet GET (Verifikation) und POST (Nachrichten) Requests
func (w *WhatsAppChannel) handleWebhook(rw http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.handleVerification(rw, r)
	case http.MethodPost:
		w.handleIncoming(rw, r)
	default:
		http.Error(rw, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleVerification verarbeitet den Meta Webhook-Verifizierungs-Challenge
func (w *WhatsAppChannel) handleVerification(rw http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if mode == "subscribe" && token == w.cfg.WebhookSecret {
		log.Println("[WhatsApp] Webhook-Verifizierung erfolgreich")
		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte(challenge))
		return
	}

	log.Printf("[WhatsApp] Webhook-Verifizierung fehlgeschlagen: mode=%s", mode)
	http.Error(rw, "Forbidden", http.StatusForbidden)
}

// handleIncoming verarbeitet eingehende Nachrichten von Meta
func (w *WhatsAppChannel) handleIncoming(rw http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // max 1 MB
	if err != nil {
		http.Error(rw, "Bad Request", http.StatusBadRequest)
		return
	}

	// HMAC-Signatur prüfen
	if !w.verifySignature(r.Header.Get("X-Hub-Signature-256"), body) {
		log.Println("[WhatsApp] HMAC-Signatur ungültig – Nachricht verworfen")
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Immer 200 OK antworten (Meta erwartet schnelle Antwort)
	rw.WriteHeader(http.StatusOK)

	// Payload parsen
	var payload whatsAppWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("[WhatsApp] Payload-Fehler: %v", err)
		return
	}

	// Nachrichten extrahieren
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Value.Messages == nil {
				continue
			}
			for _, wmsg := range change.Value.Messages {
				w.processIncomingMessage(wmsg, change.Value.Contacts)
			}
		}
	}
}

func (w *WhatsAppChannel) processIncomingMessage(wmsg whatsAppMessage, contacts []whatsAppContact) {
	// Absender-Name ermitteln
	from := wmsg.From
	for _, c := range contacts {
		if c.WaID == from {
			break
		}
	}

	normalized := normalizePhone(from)

	// Whitelist prüfen
	if len(w.allow) > 0 && !w.allow[normalized] {
		log.Printf("[WhatsApp] Nachricht von nicht erlaubter Nummer verworfen: %s", from)
		return
	}

	var msg Message
	msg.ID = wmsg.ID
	msg.ChannelID = "whatsapp"
	msg.ChatID = from
	msg.SenderID = from

	switch wmsg.Type {
	case "text":
		msg.Type = MessageTypeText
		msg.Text = wmsg.Text.Body
	case "audio", "voice":
		msg.Type = MessageTypeVoice
		// Audio-Download wird über Media-API implementiert
		log.Printf("[WhatsApp] Voice-Nachricht empfangen (Media-ID: %s) – Download noch nicht implementiert", wmsg.Audio.ID)
		return
	default:
		log.Printf("[WhatsApp] Unbekannter Nachrichtentyp: %s", wmsg.Type)
		return
	}

	log.Printf("[WhatsApp] Nachricht von %s: %.50s", from, msg.Text)

	if w.bus != nil {
		w.bus <- msg
	}
}

// verifySignature prüft die HMAC-SHA256-Signatur von Meta
func (w *WhatsAppChannel) verifySignature(signature string, body []byte) bool {
	if w.cfg.WebhookSecret == "" {
		return true // Kein Secret konfiguriert → nicht prüfen
	}

	signature = strings.TrimPrefix(signature, "sha256=")
	mac := hmac.New(sha256.New, []byte(w.cfg.WebhookSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}

// normalizePhone normalisiert Telefonnummern für den Vergleich
func normalizePhone(phone string) string {
	phone = strings.TrimPrefix(phone, "+")
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	return phone
}

// ── Webhook-Payload-Strukturen ─────────────────────────────────────────────

type whatsAppWebhookPayload struct {
	Object string            `json:"object"`
	Entry  []whatsAppEntry   `json:"entry"`
}

type whatsAppEntry struct {
	ID      string           `json:"id"`
	Changes []whatsAppChange `json:"changes"`
}

type whatsAppChange struct {
	Value whatsAppValue `json:"value"`
	Field string        `json:"field"`
}

type whatsAppValue struct {
	MessagingProduct string           `json:"messaging_product"`
	Contacts         []whatsAppContact `json:"contacts"`
	Messages         []whatsAppMessage `json:"messages"`
}

type whatsAppContact struct {
	Profile struct{ Name string `json:"name"` } `json:"profile"`
	WaID    string `json:"wa_id"`
}

type whatsAppMessage struct {
	From      string            `json:"from"`
	ID        string            `json:"id"`
	Timestamp string            `json:"timestamp"`
	Type      string            `json:"type"`
	Text      struct{ Body string `json:"body"` }   `json:"text"`
	Audio     struct{ ID   string `json:"id"` }     `json:"audio"`
}

// splitMessage teilt lange Nachrichten in Chunks auf
func splitMessage(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}
	var chunks []string
	runes := []rune(text)
	for len(runes) > 0 {
		end := maxLen
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[:end]))
		runes = runes[end:]
	}
	return chunks
}

// SendPhoto ist noch nicht implementiert für whatsapp
func (w *WhatsAppChannel) SendPhoto(_ string, _ string, _ string) error {
	return fmt.Errorf("whatsapp sendPhoto noch nicht implementiert")
}
