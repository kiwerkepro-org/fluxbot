package channels

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Manager verwaltet alle aktiven Kanäle und orchestriert den Nachrichtenfluss.
// Er implementiert das Fan-In-Pattern: Alle Kanäle senden in einen gemeinsamen Bus.
type Manager struct {
	channels map[string]Channel
	bus      chan Message
	mu       sync.RWMutex
}

// NewManager erstellt einen neuen ChannelManager.
// busSize legt die Puffergröße des Nachrichtenbusses fest.
func NewManager(busSize int) *Manager {
	return &Manager{
		channels: make(map[string]Channel),
		bus:      make(chan Message, busSize),
	}
}

// Register registriert einen Kanal beim Manager.
// Muss vor Start() aufgerufen werden.
func (m *Manager) Register(ch Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[ch.Name()] = ch
	log.Printf("[Manager] Kanal registriert: %s", ch.Name())
}

// Start startet alle registrierten Kanäle als Goroutinen.
// Gibt einen Fehler zurück wenn keine Kanäle registriert sind.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.channels) == 0 {
		return fmt.Errorf("keine Kanäle registriert – mindestens einen Kanal in config.json aktivieren")
	}

	for name, ch := range m.channels {
		log.Printf("[Manager] Starte Kanal: %s", name)
		go func(ch Channel) {
			if err := ch.Start(ctx, m.bus); err != nil {
				log.Printf("[Manager] Kanal %s beendet mit Fehler: %v", ch.Name(), err)
			}
		}(ch)
	}
	return nil
}

// Stop beendet alle Kanäle graceful
func (m *Manager) Stop() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ch := range m.channels {
		ch.Stop()
	}
	log.Println("[Manager] Alle Kanäle gestoppt")
}

// Messages gibt den eingehenden Nachrichtenbus zurück (read-only).
// Der Agent-Loop liest hier alle eingehenden Nachrichten aller Kanäle.
func (m *Manager) Messages() <-chan Message {
	return m.bus
}

// Reply sendet eine Antwort über den richtigen Kanal.
// Verwendet ChannelID aus der ursprünglichen Nachricht um den Kanal zu finden.
func (m *Manager) Reply(msg Message, text string) error {
	m.mu.RLock()
	ch, ok := m.channels[msg.ChannelID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("kanal nicht gefunden: %s", msg.ChannelID)
	}
	return ch.Send(msg.ChatID, text)
}

// Typing zeigt den Tipp-Indikator im richtigen Kanal
func (m *Manager) Typing(msg Message) {
	m.mu.RLock()
	ch, ok := m.channels[msg.ChannelID]
	m.mu.RUnlock()

	if ok {
		ch.TypingIndicator(msg.ChatID)
	}
}

// ActiveChannels gibt die Namen aller aktiven Kanäle zurück
func (m *Manager) ActiveChannels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.channels))
	for name := range m.channels {
		names = append(names, name)
	}
	return names
}

// ReplyPhoto schickt ein Bild als Antwort auf eine Nachricht
func (m *Manager) ReplyPhoto(msg Message, imageURL, caption string) error {
	m.mu.RLock()
	ch, ok := m.channels[msg.ChannelID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("channel '%s' nicht gefunden", msg.ChannelID)
	}
	return ch.SendPhoto(msg.ChatID, imageURL, caption)
}

// ReplyVoice sendet eine Sprachnachricht als Antwort.
// Prüft per type-assert ob der Kanal VoiceChannel implementiert.
// Falls nicht (z.B. Discord, Slack), wird ein Fehler zurückgegeben – der Aufrufer
// kann dann auf Text-Antwort zurückfallen.
func (m *Manager) ReplyVoice(msg Message, audioData []byte) error {
	m.mu.RLock()
	ch, ok := m.channels[msg.ChannelID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("kanal '%s' nicht gefunden", msg.ChannelID)
	}
	vc, ok := ch.(VoiceChannel)
	if !ok {
		return fmt.Errorf("kanal '%s' unterstützt keine Sprachnachrichten", msg.ChannelID)
	}
	return vc.SendVoice(msg.ChatID, audioData)
}

// SendTo sendet eine Textnachricht direkt an einen bestimmten Kanal + Chat.
// Wird vom Cron-System genutzt um Reminders auszuliefern.
func (m *Manager) SendTo(channelID, chatID, text string) error {
	m.mu.RLock()
	ch, ok := m.channels[channelID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("channel '%s' nicht gefunden", channelID)
	}
	return ch.Send(chatID, text)
}
