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
	"github.com/ki-werke/fluxbot/pkg/security"
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
	cfg     DiscordConfig
	session *discordgo.Session
	bus     chan<- Message
	allow   map[string]bool
	stopCh  chan struct{}
	client  *http.Client
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

	// ── Anhänge scannen (alle Typen: Audio, Bild, Dokument, Video, ...) ──────
	for _, att := range m.Attachments {
		data, err := d.downloadToMemory(att.URL)
		if err != nil {
			log.Printf("[Discord] Download-Fehler für '%s': %v", att.Filename, err)
			continue
		}

		isSafe, err := security.ScanFile(data)
		if err != nil {
			log.Printf("[Discord/Security] Scan-Warnung für '%s': %v", att.Filename, err)
			// Bei VT-API-Fehler: nicht blockieren
		}
		if !isSafe {
			log.Printf("[Discord/Security] 🚨 Blockiere bösartige Datei von User %s: %s", senderID, att.Filename)
			d.Send(chatID, security.VTFileBlockedMsg)
			return
		}

		log.Printf("[Discord/Security] ✅ Datei sicher: %s", att.Filename)

		// Audio-Anhänge als Voice-Nachricht weiterleiten
		if isDiscordAudio(att) {
			localPath, err := saveTempFileFromData(data, ".ogg")
			if err != nil {
				log.Printf("[Discord] Temp-Datei konnte nicht erstellt werden: %v", err)
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
		// Andere Dateitypen: sicher befunden, Nachricht fließt als Text durch (s.u.)
	}

	// ── URL-Scan bei Textnachrichten ──────────────────────────────────────────
	text := strings.TrimSpace(m.Content)
	if text != "" {
		isSafe, badURL, err := security.ScanURLsInText(text)
		if err != nil {
			log.Printf("[Discord/Security] URL-Scan Warnung: %v", err)
		}
		if !isSafe {
			log.Printf("[Discord/Security] 🚨 Bösartige URL von User %s: %s", senderID, badURL)
			d.Send(chatID, security.VTURLBlockedMsg)
			return
		}
	}

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

// downloadToMemory lädt eine Datei von einer URL in den RAM.
func (d *DiscordChannel) downloadToMemory(url string) ([]byte, error) {
	resp, err := d.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// downloadFile lädt eine Datei von einer URL herunter und gibt den lokalen Pfad zurück.
// Wird noch für Abwärtskompatibilität behalten.
func (d *DiscordChannel) downloadFile(url, ext string) (string, error) {
	data, err := d.downloadToMemory(url)
	if err != nil {
		return "", err
	}
	return saveTempFileFromData(data, ext)
}

// saveTempFileFromData speichert Bytes in einer temporären Datei und gibt den Pfad zurück.
func saveTempFileFromData(data []byte, ext string) (string, error) {
	tmpPath := fmt.Sprintf("/tmp/discord_media_%d%s", time.Now().UnixNano(), ext)
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return "", fmt.Errorf("temp-datei konnte nicht erstellt werden: %w", err)
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
