package channels

import "context"

// MessageType unterscheidet verschiedene Nachrichtenarten
type MessageType string

const (
	MessageTypeText  MessageType = "text"
	MessageTypeVoice MessageType = "voice"
	MessageTypeImage MessageType = "image"
)

// Message ist die normalisierte Nachricht, die durch alle Channels fließt
type Message struct {
	ID        string      // Eindeutige Nachrichten-ID
	ChannelID string      // Name des Channels (z.B. "telegram", "discord")
	ChatID    string      // Chat/Raum-ID (für Antworten)
	SenderID  string      // Absender-ID
	UserName  string      // NEU: Absender-Name (für Agent/Security)
	Type      MessageType // Nachrichtentyp
	Text      string      // Textinhalt
	VoiceData []byte      // NEU: Sprachdaten im RAM (für Scanner/Transkription)
	MediaPath string      // Lokaler Pfad zu heruntergeladenen Medien
	RawData   interface{} // Channel-spezifische Rohdaten
}

// Channel ist das Interface für alle Kommunikationskanäle
type Channel interface {
	// Name gibt den eindeutigen Kanalnamen zurück
	Name() string

	// Start startet den Channel und schreibt eingehende Nachrichten in den Bus
	Start(ctx context.Context, bus chan<- Message) error

	// Stop beendet den Channel
	Stop()

	// Send schickt eine Textnachricht an einen Chat
	Send(chatID string, text string) error

	// SendPhoto schickt ein Bild an einen Chat
	// imageURL: Direkte URL zum Bild (muss öffentlich erreichbar sein)
	// caption: optionaler Bildtext (leer = kein Caption)
	SendPhoto(chatID string, imageURL string, caption string) error

	// TypingIndicator zeigt an dass der Bot tippt
	TypingIndicator(chatID string)
}

// VoiceChannel erweitert Channel um Sprachausgabe.
// Nur Kanäle die Voice-Nachrichten unterstützen (z.B. Telegram) implementieren dieses Interface.
// Manager.ReplyVoice() prüft per type-assert ob der Kanal VoiceChannel implementiert.
type VoiceChannel interface {
	Channel

	// SendVoice schickt eine Sprachnachricht (OGG/Opus-Bytes) an einen Chat.
	// audioData: rohe OGG/Opus-Bytes (direkt von OpenAI TTS lieferbar)
	SendVoice(chatID string, audioData []byte) error
}
