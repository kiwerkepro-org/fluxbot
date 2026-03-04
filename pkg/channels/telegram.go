package channels

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/ki-werke/fluxbot/pkg/security"
)

type TelegramConfig struct {
	Token     string   `json:"token"`
	AllowFrom []string `json:"allow_from"`
}

type TelegramChannel struct {
	bot    *tgbotapi.BotAPI
	config TelegramConfig
}

func NewTelegramChannel(cfg TelegramConfig) *TelegramChannel {
	bot, err := tgbotapi.NewBotAPI(cfg.Token)
	if err != nil {
		log.Fatalf("[Telegram] Fehler beim Initialisieren: %v", err)
	}
	return &TelegramChannel{bot: bot, config: cfg}
}

func (t *TelegramChannel) Name() string { return "telegram" }

func (t *TelegramChannel) Start(ctx context.Context, input chan<- Message) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := t.bot.GetUpdatesChan(u)

	for {
		select {
		case <-ctx.Done():
			return nil
		case update := <-updates:
			if update.Message == nil {
				continue
			}

			// Auth-Check & IDs
			senderID := fmt.Sprintf("%d", update.Message.From.ID)
			chatID := fmt.Sprintf("%d", update.Message.Chat.ID)

			if !t.isAllowed(senderID) {
				continue
			}

			msg := Message{
				ID:        fmt.Sprintf("%d", update.Message.MessageID),
				ChannelID: "telegram",
				ChatID:    chatID,
				SenderID:  senderID,
				UserName:  update.Message.From.UserName,
				Type:      MessageTypeText,
				Text:      update.Message.Text,
				RawData:   update,
			}

			blocked := false

			// URL-Scan bei Textnachrichten
			if update.Message.Text != "" {
				isSafe, badURL, err := security.ScanURLsInText(update.Message.Text)
				if err != nil {
					log.Printf("[Telegram/Security] URL-Scan Warnung: %v", err)
				}
				if !isSafe {
					log.Printf("[Telegram/Security] 🚨 Bösartige URL von %s: %s", senderID, badURL)
					t.Send(chatID, security.VTURLBlockedMsg)
					continue
				}
			}

			// Sprachnachrichten
			if update.Message.Voice != nil {
				data, wasBlocked := t.downloadAndScanFile(chatID, senderID, update.Message.Voice.FileID, "Voice")
				if wasBlocked {
					blocked = true
				} else {
					msg.Type = MessageTypeVoice
					msg.VoiceData = data
				}
			}

			// Audiodateien (MP3, AAC, etc.)
			if !blocked && update.Message.Audio != nil {
				_, wasBlocked := t.downloadAndScanFile(chatID, senderID, update.Message.Audio.FileID, "Audio")
				if wasBlocked {
					blocked = true
				}
			}

			// Dokumente (PDF, Office, ZIP, EXE, etc.) - VT-Scan + Weitergabe
			if !blocked && update.Message.Document != nil {
				data, wasBlocked := t.downloadAndScanFile(chatID, senderID, update.Message.Document.FileID, "Dokument")
				if wasBlocked {
					blocked = true
				} else if data != nil {
					// Sichere Datei: Speichere lokal und leite weiter
					filename := update.Message.Document.FileName
					if filename == "" {
						filename = "document.pdf"
					}
					// Bestimme Dateiendung
					ext := ".pdf"
					if len(filename) > 4 {
						ext = filename[len(filename)-4:]
					}
					localPath, err := saveTempFile(data, ext)
					if err != nil {
						log.Printf("[Telegram] Fehler beim Speichern des Dokuments: %v", err)
						// Fallback: Agent erhält nur Log, kein Crash
						continue
					}
					msg.Type = MessageTypeVoice // Nutze Voice-Handler mit MediaPath
					msg.MediaPath = localPath
					log.Printf("[Telegram] Dokument akzeptiert: %s (%d bytes) von %s → MediaPath: %s", filename, len(data), senderID, localPath)
				}
			}

			// Fotos (größtes verfügbares Format scannen)
			if !blocked && update.Message.Photo != nil && len(update.Message.Photo) > 0 {
				largest := update.Message.Photo[len(update.Message.Photo)-1]
				data, wasBlocked := t.downloadAndScanFile(chatID, senderID, largest.FileID, "Foto")
				if wasBlocked {
					blocked = true
				} else if data != nil {
					// Sichere Datei: Speichere lokal und leite weiter
					localPath, err := saveTempFile(data, ".jpg")
					if err != nil {
						log.Printf("[Telegram] Fehler beim Speichern des Fotos: %v", err)
						continue
					}
					msg.Type = MessageTypeImage
					msg.MediaPath = localPath
					// Caption des Fotos als Nutzer-Prompt übernehmen
					if update.Message.Caption != "" {
						msg.Text = update.Message.Caption
					}
					log.Printf("[Telegram] Foto akzeptiert (%d bytes) von %s → MediaPath: %s | Caption: %q", len(data), senderID, localPath, msg.Text)
				}
			}

			// Videos
			if !blocked && update.Message.Video != nil {
				data, wasBlocked := t.downloadAndScanFile(chatID, senderID, update.Message.Video.FileID, "Video")
				if wasBlocked {
					blocked = true
				} else if data != nil {
					// Sichere Datei: Speichere lokal und leite weiter
					localPath, err := saveTempFile(data, ".mp4")
					if err != nil {
						log.Printf("[Telegram] Fehler beim Speichern des Videos: %v", err)
						continue
					}
					msg.Type = MessageTypeVoice
					msg.MediaPath = localPath
					log.Printf("[Telegram] Video akzeptiert (%d bytes) von %s → MediaPath: %s", len(data), senderID, localPath)
				}
			}

			// VideoNote (Rundvideo)
			if !blocked && update.Message.VideoNote != nil {
				data, wasBlocked := t.downloadAndScanFile(chatID, senderID, update.Message.VideoNote.FileID, "VideoNote")
				if wasBlocked {
					blocked = true
				} else if data != nil {
					// Sichere Datei: Speichere lokal und leite weiter
					localPath, err := saveTempFile(data, ".mp4")
					if err != nil {
						log.Printf("[Telegram] Fehler beim Speichern des VideoNote: %v", err)
						continue
					}
					msg.Type = MessageTypeVoice
					msg.MediaPath = localPath
					log.Printf("[Telegram] VideoNote akzeptiert (%d bytes) von %s → MediaPath: %s", len(data), senderID, localPath)
				}
			}

			if blocked {
				continue
			}

			input <- msg
		}
	}
}

// downloadAndScanFile lädt eine Datei herunter und scannt sie mit VirusTotal.
// Gibt (nil, true) zurück wenn die Datei geblockt wurde.
// Gibt (data, false) zurück wenn die Datei sicher ist.
func (t *TelegramChannel) downloadAndScanFile(chatID, senderID, fileID, fileType string) ([]byte, bool) {
	data, err := t.DownloadFile(fileID)
	if err != nil {
		log.Printf("[Telegram] Download-Fehler (%s): %v", fileType, err)
		return nil, false // Bei Download-Fehler: nicht blockieren, weiterleiten
	}

	isSafe, err := security.ScanFile(data)
	if err != nil {
		log.Printf("[Telegram/Security] Scan-Warnung (%s): %v", fileType, err)
		// Bei VT-API-Fehler: nicht blockieren
		return data, false
	}

	if !isSafe {
		log.Printf("[Telegram/Security] 🚨 Blockiere bösartige Datei (%s) von User %s", fileType, senderID)
		t.Send(chatID, security.VTFileBlockedMsg)
		return nil, true
	}

	log.Printf("[Telegram/Security] ✅ %s von User %s sicher", fileType, senderID)
	return data, false
}


func (t *TelegramChannel) Send(chatID, text string) error {
	id := int64(0)
	fmt.Sscanf(chatID, "%d", &id)
	m := tgbotapi.NewMessage(id, text)
	_, err := t.bot.Send(m)
	return err
}

func (t *TelegramChannel) DownloadFile(fileID string) ([]byte, error) {
	url, err := t.bot.GetFileDirectURL(fileID)
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (t *TelegramChannel) isAllowed(senderID string) bool {
	if len(t.config.AllowFrom) == 0 {
		return true
	}
	for _, id := range t.config.AllowFrom {
		if id == senderID {
			return true
		}
	}
	return false
}

func (t *TelegramChannel) Stop() {}

func (t *TelegramChannel) TypingIndicator(chatID string) {
	id := int64(0)
	fmt.Sscanf(chatID, "%d", &id)
	action := tgbotapi.NewChatAction(id, tgbotapi.ChatTyping)
	t.bot.Send(action)
}

func (t *TelegramChannel) SendPhoto(chatID string, imageURL string, caption string) error {
	id := int64(0)
	fmt.Sscanf(chatID, "%d", &id)
	photo := tgbotapi.NewPhoto(id, tgbotapi.FileURL(imageURL))
	photo.Caption = caption
	_, err := t.bot.Send(photo)
	return err
}

// SendVoice schickt eine Sprachnachricht (OGG/Opus-Bytes) an einen Telegram-Chat.
// Implementiert das VoiceChannel Interface.
// audioData: rohe OGG/Opus-Bytes (direkt von OpenAI TTS API geliefert)
func (t *TelegramChannel) SendVoice(chatID string, audioData []byte) error {
	id := int64(0)
	fmt.Sscanf(chatID, "%d", &id)
	voice := tgbotapi.NewVoice(id, tgbotapi.FileBytes{
		Name:  "voice.ogg",
		Bytes: audioData,
	})
	_, err := t.bot.Send(voice)
	return err
}
