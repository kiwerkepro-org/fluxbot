package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// TelegramChannel implementiert das Channel-Interface für Telegram.
// Nutzt Long-Polling (kein Webhook erforderlich).
type TelegramChannel struct {
	token     string
	allowFrom map[string]bool
	client    *http.Client
	offset    int
	stopCh    chan struct{}
}

// TelegramConfig enthält die Konfiguration für den Telegram-Kanal (aus config.json)
type TelegramConfig struct {
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom"`
}

// NewTelegramChannel erstellt einen neuen Telegram-Kanal
func NewTelegramChannel(cfg TelegramConfig) *TelegramChannel {
	allowMap := make(map[string]bool)
	for _, id := range cfg.AllowFrom {
		allowMap[id] = true
	}
	return &TelegramChannel{
		token:     cfg.Token,
		allowFrom: allowMap,
		client:    &http.Client{Timeout: 90 * time.Second},
		stopCh:    make(chan struct{}),
	}
}

func (t *TelegramChannel) Name() string { return "telegram" }

// Start startet den Telegram Long-Polling-Loop.
func (t *TelegramChannel) Start(ctx context.Context, bus chan<- Message) error {
	log.Println("[Telegram] Long-Polling gestartet")

	for {
		select {
		case <-ctx.Done():
			log.Println("[Telegram] Context abgebrochen – stoppe")
			return nil
		case <-t.stopCh:
			log.Println("[Telegram] Stop-Signal empfangen")
			return nil
		default:
		}

		updates, err := t.getUpdates()
		if err != nil {
			log.Printf("[Telegram] Fehler beim Abrufen der Updates: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, u := range updates {
			t.offset = u.UpdateID + 1
			senderID := fmt.Sprintf("%d", u.Message.From.ID)

			// Whitelist-Prüfung
			if len(t.allowFrom) > 0 && !t.allowFrom[senderID] {
				log.Printf("[Telegram] Nachricht von nicht erlaubtem User ignoriert: %s", senderID)
				continue
			}

			chatID := fmt.Sprintf("%d", u.Message.Chat.ID)

			// Sprachnachricht – Datei herunterladen und lokalen Pfad übergeben
			if u.Message.Voice.FileID != "" {
				localPath, err := t.downloadVoiceFile(u.Message.Voice.FileID)
				if err != nil {
					log.Printf("[Telegram] Fehler beim Herunterladen der Sprachnachricht: %v", err)
					// Trotzdem weiterleiten – Agent zeigt Fehlermeldung
					bus <- Message{
						ID:        fmt.Sprintf("tg_%d", u.UpdateID),
						ChannelID: "telegram",
						ChatID:    chatID,
						SenderID:  senderID,
						Type:      MessageTypeVoice,
						MediaPath: "", // leer = Download fehlgeschlagen
					}
				} else {
					bus <- Message{
						ID:        fmt.Sprintf("tg_%d", u.UpdateID),
						ChannelID: "telegram",
						ChatID:    chatID,
						SenderID:  senderID,
						Type:      MessageTypeVoice,
						MediaPath: localPath,
					}
				}
				continue
			}

			// Textnachricht
			if u.Message.Text != "" {
				bus <- Message{
					ID:        fmt.Sprintf("tg_%d", u.UpdateID),
					ChannelID: "telegram",
					ChatID:    chatID,
					SenderID:  senderID,
					Type:      MessageTypeText,
					Text:      u.Message.Text,
				}
			}
		}

		if len(updates) == 0 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// Stop sendet das Stop-Signal an den Polling-Loop
func (t *TelegramChannel) Stop() {
	select {
	case <-t.stopCh:
	default:
		close(t.stopCh)
	}
}

// Send sendet eine Textnachricht an eine Chat-ID.
// Kürzt automatisch auf Telegrams 4096-Zeichen-Limit.
func (t *TelegramChannel) Send(chatID string, text string) error {
	if len([]rune(text)) > 4096 {
		runes := []rune(text)
		text = string(runes[:4093]) + "..."
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	body, _ := json.Marshal(map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	})

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("telegram send error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram API Fehler %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// TypingIndicator sendet den "tippt…"-Status
func (t *TelegramChannel) TypingIndicator(chatID string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendChatAction", t.token)
	body, _ := json.Marshal(map[string]interface{}{
		"chat_id": chatID,
		"action":  "typing",
	})
	http.Post(url, "application/json", bytes.NewBuffer(body))
}

// downloadVoiceFile lädt eine Sprachnachricht von Telegram herunter.
// Gibt den lokalen Dateipfad zurück. Aufrufer ist für das Löschen verantwortlich.
// Portiert aus alter main.go: downloadVoiceMessage()
func (t *TelegramChannel) downloadVoiceFile(fileID string) (string, error) {
	// Schritt 1: FilePath von Telegram API holen
	fileInfoURL := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", t.token, fileID)
	resp, err := t.client.Get(fileInfoURL)
	if err != nil {
		return "", fmt.Errorf("getFile fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var fileInfo struct {
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &fileInfo); err != nil || fileInfo.Result.FilePath == "" {
		return "", fmt.Errorf("file_path nicht gefunden in API-Antwort")
	}

	// Schritt 2: Datei herunterladen
	downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", t.token, fileInfo.Result.FilePath)
	resp2, err := t.client.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("download fehlgeschlagen: %w", err)
	}
	defer resp2.Body.Close()

	// In temporäre Datei speichern
	tmpPath := fmt.Sprintf("/tmp/voice_%d.ogg", time.Now().UnixNano())
	out, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("temp-datei konnte nicht erstellt werden: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp2.Body); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("fehler beim Speichern der Audiodatei: %w", err)
	}

	return tmpPath, nil
}

// SendPhoto schickt ein Bild via Telegram sendPhoto API.
// imageURL muss öffentlich erreichbar sein (z.B. CDN-URL von fal.ai oder DALL-E).
func (t *TelegramChannel) SendPhoto(chatID, imageURL, caption string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", t.token)
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"photo":      imageURL,
		"parse_mode": "Markdown",
	}
	if caption != "" {
		payload["caption"] = caption
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram sendPhoto: request-Fehler: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram sendPhoto: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram sendPhoto Fehler %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// getUpdates holt neue Updates von der Telegram API (Long-Polling)
func (t *TelegramChannel) getUpdates() ([]telegramUpdate, error) {
	url := fmt.Sprintf(
		"https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30",
		t.token, t.offset,
	)
	resp, err := t.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		OK     bool             `json:"ok"`
		Result []telegramUpdate `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Result, nil
}

// --- Interne Telegram-Typen ---

type telegramUpdate struct {
	UpdateID int `json:"update_id"`
	Message  struct {
		Chat struct {
			ID int `json:"id"`
		} `json:"chat"`
		From struct {
			ID int `json:"id"`
		} `json:"from"`
		Text  string `json:"text"`
		Voice struct {
			FileID   string `json:"file_id"`
			Duration int    `json:"duration"`
		} `json:"voice"`
	} `json:"message"`
}


