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

	// ── Textnachrichten + URL-Scan ─────────────────────────────────────────
	case "text":
		text := strings.TrimSpace(wmsg.Text.Body)
		if text == "" {
			return
		}

		// URL-Scan
		isSafe, badURL, err := security.ScanURLsInText(text)
		if err != nil {
			log.Printf("[WhatsApp/Security] URL-Scan Warnung: %v", err)
		}
		if !isSafe {
			log.Printf("[WhatsApp/Security] 🚨 Bösartige URL von %s: %s", from, badURL)
			w.Send(from, security.VTURLBlockedMsg)
			return
		}

		msg.Type = MessageTypeText
		msg.Text = text

	// ── Audio / Sprachnachrichten ─────────────────────────────────────────
	case "audio", "voice":
		data, blocked := w.downloadAndScanMedia(wmsg.Audio.ID, "Audio", from)
		if blocked {
			return
		}
		msg.Type = MessageTypeVoice
		msg.VoiceData = data

	// ── Bilder ─────────────────────────────────────────────────────────────
	case "image":
		_, blocked := w.downloadAndScanMedia(wmsg.Image.ID, "Bild", from)
		if blocked {
			return
		}
		// Bild-Caption als Text weiterleiten (falls vorhanden)
		msg.Type = MessageTypeText
		msg.Text = wmsg.Image.Caption

	// ── Dokumente (PDF, Office, ZIP, etc.) ────────────────────────────────
	case "document":
		_, blocked := w.downloadAndScanMedia(wmsg.Document.ID, "Dokument", from)
		if blocked {
			return
		}
		msg.Type = MessageTypeText
		msg.Text = wmsg.Document.Caption

	// ── Videos ────────────────────────────────────────────────────────────
	case "video":
		_, blocked := w.downloadAndScanMedia(wmsg.Video.ID, "Video", from)
		if blocked {
			return
		}
		msg.Type = MessageTypeText
		msg.Text = wmsg.Video.Caption

	// ── Sticker ────────────────────────────────────────────────────────────
	case "sticker":
		_, blocked := w.downloadAndScanMedia(wmsg.Sticker.ID, "Sticker", from)
		if blocked {
			return
		}
		// Sticker haben keinen Text – Nachricht wird nicht weitergeleitet
		return

	default:
		log.Printf("[WhatsApp] Unbekannter Nachrichtentyp: %s", wmsg.Type)
		return
	}

	log.Printf("[WhatsApp] Nachricht von %s (Typ: %s): %.50s", from, wmsg.Type, msg.Text)

	if w.bus != nil {
		w.bus <- msg
	}
}

// downloadAndScanMedia lädt ein WhatsApp-Medium herunter und scannt es mit VT.
// Gibt (data, false) zurück wenn sicher, (nil, true) wenn geblockt.
func (w *WhatsAppChannel) downloadAndScanMedia(mediaID, mediaType, from string) ([]byte, bool) {
	data, err := w.downloadMedia(mediaID)
	if err != nil {
		log.Printf("[WhatsApp] %s-Download Fehler (von %s): %v", mediaType, from, err)
		return nil, false // Bei Download-Fehler: nicht blockieren
	}

	isSafe, err := security.ScanFile(data)
	if err != nil {
		log.Printf("[WhatsApp/Security] Scan-Warnung (%s): %v", mediaType, err)
		// Bei VT-API-Fehler: nicht blockieren
		return data, false
	}

	if !isSafe {
		log.Printf("[WhatsApp/Security] 🚨 Blockiere bösartige Datei (%s) von %s", mediaType, from)
		w.Send(from, security.VTFileBlockedMsg)
		return nil, true
	}

	log.Printf("[WhatsApp/Security] ✅ %s sicher (von %s)", mediaType, from)
	return data, false
}

// downloadMedia lädt eine WhatsApp-Mediendatei über die Meta Graph API herunter.
// Schritt 1: Media-Metadaten abrufen (URL)
// Schritt 2: Datei von der erhaltenen URL herunterladen
func (w *WhatsAppChannel) downloadMedia(mediaID string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Schritt 1: Media-URL abrufen
	metaEndpoint := fmt.Sprintf("%s/%s", whatsappGraphURL, mediaID)
	req, err := http.NewRequest("GET", metaEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+w.cfg.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: media-info fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("whatsapp: media-info HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var mediaInfo struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
		FileSize int64  `json:"file_size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&mediaInfo); err != nil {
		return nil, fmt.Errorf("whatsapp: media-info ungültig: %w", err)
	}
	if mediaInfo.URL == "" {
		return nil, fmt.Errorf("whatsapp: leere media-URL für ID %s", mediaID)
	}

	// Schritt 2: Datei herunterladen
	req2, err := http.NewRequest("GET", mediaInfo.URL, nil)
	if err != nil {
		return nil, err
	}
	req2.Header.Set("Authorization", "Bearer "+w.cfg.APIKey)

	resp2, err := client.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: media-download fehlgeschlagen: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode >= 400 {
		return nil, fmt.Errorf("whatsapp: media-download HTTP %d", resp2.StatusCode)
	}

	return io.ReadAll(resp2.Body)
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
	Object string          `json:"object"`
	Entry  []whatsAppEntry `json:"entry"`
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
	MessagingProduct string            `json:"messaging_product"`
	Contacts         []whatsAppContact `json:"contacts"`
	Messages         []whatsAppMessage `json:"messages"`
}

type whatsAppContact struct {
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
	WaID string `json:"wa_id"`
}

type whatsAppMessage struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Text      struct {
		Body string `json:"body"`
	} `json:"text"`
	Audio struct {
		ID string `json:"id"`
	} `json:"audio"`
	Image struct {
		ID      string `json:"id"`
		Caption string `json:"caption"`
	} `json:"image"`
	Document struct {
		ID       string `json:"id"`
		Caption  string `json:"caption"`
		Filename string `json:"filename"`
	} `json:"document"`
	Video struct {
		ID      string `json:"id"`
		Caption string `json:"caption"`
	} `json:"video"`
	Sticker struct {
		ID string `json:"id"`
	} `json:"sticker"`
}

// splitMessage teilt lange Nachrichten in Chunks auf.
// Da alle Channel-Dateien im gleichen Package "channels" sind, reicht eine Definition für alle.
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
