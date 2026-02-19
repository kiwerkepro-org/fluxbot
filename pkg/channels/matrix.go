package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MatrixConfig enthält die Konfiguration für den Matrix-Kanal
type MatrixConfig struct {
	HomeServer string   // z.B. https://matrix.org
	UserID     string   // z.B. @fluxbot:matrix.org
	Token      string   // Access Token
	AllowFrom  []string // User-IDs die Nachrichten senden dürfen
}

// MatrixChannel implementiert den Matrix-Kanal via Client-Server API (Long-Polling /sync)
//
// Der Matrix-Client fragt regelmäßig den /sync-Endpunkt ab.
// Keine externen Abhängigkeiten nötig – reines HTTP.
type MatrixChannel struct {
	cfg       MatrixConfig
	bus       chan<- Message
	client    *http.Client
	nextBatch string
	botUserID string
	allow     map[string]bool
}

// NewMatrixChannel erstellt einen neuen Matrix-Kanal
func NewMatrixChannel(cfg MatrixConfig) *MatrixChannel {
	allow := make(map[string]bool)
	for _, a := range cfg.AllowFrom {
		allow[a] = true
	}
	return &MatrixChannel{
		cfg:    cfg,
		client: &http.Client{Timeout: 35 * time.Second}, // Timeout > sync timeout
		allow:  allow,
	}
}

func (m *MatrixChannel) Name() string { return "matrix" }

// Start startet den Long-Polling-Loop
func (m *MatrixChannel) Start(ctx context.Context, bus chan<- Message) error {
	m.bus = bus
	m.botUserID = m.cfg.UserID

	// Verbindung testen
	if err := m.whoami(); err != nil {
		return fmt.Errorf("matrix: Authentifizierung fehlgeschlagen: %w", err)
	}

	log.Printf("[Matrix] Eingeloggt als %s auf %s", m.botUserID, m.cfg.HomeServer)

	go m.syncLoop(ctx)
	return nil
}

func (m *MatrixChannel) Stop() {}

// Send sendet eine Matrix-Nachricht
func (m *MatrixChannel) Send(roomID string, text string) error {
	chunks := splitMessage(text, 50000) // Matrix hat höheres Limit
	for _, chunk := range chunks {
		if err := m.sendChunk(roomID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (m *MatrixChannel) sendChunk(roomID string, text string) error {
	txID := fmt.Sprintf("flux_%d", time.Now().UnixNano())
	endpoint := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		m.cfg.HomeServer, url.PathEscape(roomID), txID)

	payload := map[string]string{
		"msgtype": "m.text",
		"body":    text,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("PUT", endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("matrix: Senden fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("matrix: API Fehler %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// TypingIndicator sendet ein Typing-Event an Matrix
func (m *MatrixChannel) TypingIndicator(roomID string) {
	endpoint := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/typing/%s",
		m.cfg.HomeServer, url.PathEscape(roomID), url.PathEscape(m.botUserID))

	payload := map[string]interface{}{
		"typing":  true,
		"timeout": 5000,
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("PUT", endpoint, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+m.cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

// whoami prüft ob der Token gültig ist
func (m *MatrixChannel) whoami() error {
	endpoint := m.cfg.HomeServer + "/_matrix/client/v3/account/whoami"
	req, _ := http.NewRequest("GET", endpoint, nil)
	req.Header.Set("Authorization", "Bearer "+m.cfg.Token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// syncLoop fragt kontinuierlich /sync ab (Long-Polling)
func (m *MatrixChannel) syncLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := m.sync(ctx); err != nil {
				log.Printf("[Matrix] Sync-Fehler: %v", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}
		}
	}
}

// sync führt einen einzelnen /sync-Request durch
func (m *MatrixChannel) sync(ctx context.Context) error {
	params := url.Values{
		"access_token": {m.cfg.Token},
		"timeout":      {"30000"}, // 30 Sekunden Long-Polling
		"filter":       {`{"room":{"timeline":{"limit":10}}}`},
	}
	if m.nextBatch != "" {
		params.Set("since", m.nextBatch)
	}

	endpoint := m.cfg.HomeServer + "/_matrix/client/v3/sync?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.Token)

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("sync HTTP %d", resp.StatusCode)
	}

	var syncResp matrixSyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return err
	}

	m.nextBatch = syncResp.NextBatch
	m.processSync(syncResp)
	return nil
}

func (m *MatrixChannel) processSync(sync matrixSyncResponse) {
	for roomID, room := range sync.Rooms.Join {
		for _, event := range room.Timeline.Events {
			if event.Type != "m.room.message" {
				continue
			}
			// Eigene Nachrichten ignorieren
			if event.Sender == m.botUserID {
				continue
			}
			// Whitelist prüfen
			if len(m.allow) > 0 && !m.allow[event.Sender] {
				continue
			}

			msgType, _ := event.Content["msgtype"].(string)
			if msgType != "m.text" {
				continue
			}

			body, _ := event.Content["body"].(string)
			body = strings.TrimSpace(body)
			if body == "" {
				continue
			}

			msg := Message{
				ID:        event.EventID,
				ChannelID: "matrix",
				ChatID:    roomID,
				SenderID:  event.Sender,
				Type:      MessageTypeText,
				Text:      body,
			}

			log.Printf("[Matrix] Nachricht von %s in %s: %.50s", event.Sender, roomID, body)
			if m.bus != nil {
				m.bus <- msg
			}
		}
	}
}

// Matrix Sync Response Strukturen
type matrixSyncResponse struct {
	NextBatch string `json:"next_batch"`
	Rooms     struct {
		Join map[string]matrixRoom `json:"join"`
	} `json:"rooms"`
}

type matrixRoom struct {
	Timeline struct {
		Events []matrixEvent `json:"events"`
	} `json:"timeline"`
}

type matrixEvent struct {
	Type    string                 `json:"type"`
	EventID string                 `json:"event_id"`
	Sender  string                 `json:"sender"`
	Content map[string]interface{} `json:"content"`
}

// SendPhoto ist noch nicht implementiert für matrix
func (m *MatrixChannel) SendPhoto(_ string, _ string, _ string) error {
	return fmt.Errorf("matrix sendPhoto noch nicht implementiert")
}
