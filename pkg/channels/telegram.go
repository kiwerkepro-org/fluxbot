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

			// Sprachnachrichten-Handling mit integriertem Malware-Scan
			if update.Message.Voice != nil {
				msg.Type = MessageTypeVoice
				data, err := t.DownloadFile(update.Message.Voice.FileID)
				if err != nil {
					log.Printf("[Telegram] Download-Fehler: %v", err)
					continue
				}

				// --- SICHERHEITS-CHECK ---
				isSafe, err := security.ScanFile(data)
				if err != nil {
					log.Printf("[Security] Scan-Warnung (reicht durch): %v", err)
				}
				if !isSafe {
					log.Printf("[Security] 🚨 Blockiere bösartige Datei von User %s", senderID)
					t.Send(chatID, "⚠️ Sicherheits-Warnung: Die gesendete Datei wurde als potenzielle Malware eingestuft und blockiert.")
					continue
				}

				msg.VoiceData = data
			}

			input <- msg
		}
	}
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
