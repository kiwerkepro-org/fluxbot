package channels

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// DiscordConfig enthält die Konfiguration für den Discord-Kanal
type DiscordConfig struct {
	Token     string
	AllowFrom []string // Discord User-IDs die Nachrichten senden dürfen (leer = alle)
}

// DiscordChannel implementiert den Discord-Kanal via Gateway (WebSocket).
//
// Setup im Discord Developer Portal (discord.com/developers):
//  1. Neue Application erstellen → Bot → Token kopieren
//  2. Unter "Privileged Gateway Intents": MESSAGE CONTENT INTENT aktivieren
//  3. Bot einladen: OAuth2 → URL Generator → Scopes: bot → Permissions: Send Messages, Read Messages
type DiscordChannel struct {
	cfg      DiscordConfig
	session  *discordgo.Session
	bus      chan<- Message
	allow    map[string]bool
	stopCh   chan struct{}
	client   *http.Client
}

// NewDiscordChannel erstellt einen neuen Discord-Kanal
func NewDiscordChannel(cfg DiscordConfig) *DiscordChannel {
	allow := make(map[string]bool)
	for _, id := range cfg.AllowFrom {
		allow[id] = true
	}
	return &DiscordChannel{
		cfg:    cfg,
		allow:  allow,
		stopCh: make(chan struct{}),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (d *DiscordChannel) Name() string { return "discord" }

// Start verbindet sich mit dem Discord Gateway und empfängt Nachrichten.
func (d *DiscordChannel) Start(ctx context.Context, bus chan<- Message) error {
	d.bus = bus

	session, err := discordgo.New("Bot " + d.cfg.Token)
	if err != nil {
		return fmt.Errorf("[Discord] Session konnte nicht erstellt werden: %w", err)
	}
	d.session = session

	// Intents: Textnachrichten in Servern + Direktnachrichten
	session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentMessageContent

	// Nachrichtenhandler registrieren
	session.AddHandler(d.onMessage)

	// WebSocket-Verbindung öffnen
	if err := session.Open(); err != nil {
		return fmt.Errorf("[Discord] Gateway-Verbindung fehlgeschlagen: %w", err)
	}

	log.Printf("[Discord] Verbunden als: %s#%s", session.State.User.Username, session.State.User.Discriminator)

	// Warten bis Context abgebrochen oder Stop() aufgerufen wird
	select {
	case <-ctx.Done():
		log.Println("[Discord] Context abgebrochen – trenne Verbindung")
	case <-d.stopCh:
		log.Println("[Discord] Stop-Signal empfangen")
	}

	session.Close()
	return nil
}

// onMessage verarbeitet eingehende Discord-Nachrichten
func (d *DiscordChannel) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Eigene Nachrichten des Bots ignorieren
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Bot-Nachrichten ignorieren
	if m.Author.Bot {
		return
	}

	// Whitelist-Prüfung (leer = alle erlaubt)
	if len(d.allow) > 0 && !d.allow[m.Author.ID] {
		log.Printf("[Discord] User %s (%s) nicht in allowFrom – ignoriert", m.Author.Username, m.Author.ID)
		return
	}

	chatID := m.ChannelID
	senderID := m.Author.ID

	// ── Sprachnachrichten / Audio-Anhänge ────────────────────────────────────
	for _, att := range m.Attachments {
		if isDiscordAudio(att) {
			localPath, err := d.downloadFile(att.URL, ".ogg")
			if err != nil {
				log.Printf("[Discord] Audio-Download fehlgeschlagen: %v", err)
				localPath = ""
			}
			d.bus <- Message{
				ID:        m.ID,
				ChannelID: "discord",
				ChatID:    chatID,
				SenderID:  senderID,
				Type:      MessageTypeVoice,
				MediaPath: localPath,
			}
			return
		}
	}

	// ── Textnachrichten ───────────────────────────────────────────────────────
	text := strings.TrimSpace(m.Content)
	if text == "" {
		return
	}

	d.bus <- Message{
		ID:        m.ID,
		ChannelID: "discord",
		ChatID:    chatID,
		SenderID:  senderID,
		Type:      MessageTypeText,
		Text:      text,
	}
}

// Stop beendet die Discord-Verbindung
func (d *DiscordChannel) Stop() {
	select {
	case <-d.stopCh:
	default:
		close(d.stopCh)
	}
}

// Send schickt eine Textnachricht in einen Discord-Kanal.
// Discord-Limit: 2000 Zeichen.
func (d *DiscordChannel) Send(chatID string, text string) error {
	if d.session == nil {
		return fmt.Errorf("[Discord] Session nicht aktiv")
	}

	// Discord-Limit: 2000 Zeichen
	if len([]rune(text)) > 2000 {
		runes := []rune(text)
		text = string(runes[:1997]) + "..."
	}

	_, err := d.session.ChannelMessageSend(chatID, text)
	if err != nil {
		return fmt.Errorf("[Discord] Send fehlgeschlagen: %w", err)
	}
	return nil
}

// SendPhoto schickt ein Bild als Discord Embed.
// imageURL muss öffentlich erreichbar sein.
func (d *DiscordChannel) SendPhoto(chatID, imageURL, caption string) error {
	if d.session == nil {
		return fmt.Errorf("[Discord] Session nicht aktiv")
	}

	embed := &discordgo.MessageEmbed{
		Image: &discordgo.MessageEmbedImage{
			URL: imageURL,
		},
		Color: 0x5865F2, // Discord-Blau
	}
	if caption != "" {
		embed.Description = caption
	}

	_, err := d.session.ChannelMessageSendEmbed(chatID, embed)
	if err != nil {
		return fmt.Errorf("[Discord] SendPhoto fehlgeschlagen: %w", err)
	}
	return nil
}

// TypingIndicator zeigt den "tippt..."-Status im Discord-Kanal
func (d *DiscordChannel) TypingIndicator(chatID string) {
	if d.session != nil {
		d.session.ChannelTyping(chatID)
	}
}

// downloadFile lädt eine Datei von einer URL herunter und gibt den lokalen Pfad zurück.
func (d *DiscordChannel) downloadFile(url, ext string) (string, error) {
	resp, err := d.client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	tmpPath := fmt.Sprintf("/tmp/discord_voice_%d%s", time.Now().UnixNano(), ext)
	out, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("temp-datei konnte nicht erstellt werden: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("fehler beim Speichern: %w", err)
	}
	return tmpPath, nil
}

// isDiscordAudio prüft ob ein Anhang eine Audio-/Sprachdatei ist.
func isDiscordAudio(att *discordgo.MessageAttachment) bool {
	ct := strings.ToLower(att.ContentType)
	if strings.HasPrefix(ct, "audio/") {
		return true
	}
	// Fallback: Dateiendung prüfen
	name := strings.ToLower(att.Filename)
	return strings.HasSuffix(name, ".ogg") ||
		strings.HasSuffix(name, ".mp3") ||
		strings.HasSuffix(name, ".wav") ||
		strings.HasSuffix(name, ".m4a") ||
		strings.HasSuffix(name, ".webm")
}
