package pairing

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PairStatus beschreibt den Status eines Pairing-Eintrags.
type PairStatus string

const (
	StatusPending  PairStatus = "pending"  // wartet auf Dashboard-Bestätigung
	StatusApproved PairStatus = "approved" // durch Dashboard-Admin genehmigt
	StatusBlocked  PairStatus = "blocked"  // explizit blockiert
)

// PairEntry ist ein einzelner Pairing-Eintrag.
type PairEntry struct {
	UserID    string     `json:"userId"`              // Telegram/Discord User-ID (z.B. "123456789")
	UserName  string     `json:"userName,omitempty"`   // @username (optional, zur Anzeige)
	Channel   string     `json:"channel"`              // "telegram", "discord", etc.
	Status    PairStatus `json:"status"`               // pending, approved, blocked
	ChatID    string     `json:"chatId,omitempty"`     // Chat-ID für Antwort-Nachrichten
	CreatedAt time.Time  `json:"createdAt"`            // Zeitpunkt des ersten Kontakts
	ApprovedAt *time.Time `json:"approvedAt,omitempty"` // Zeitpunkt der Genehmigung
	LastSeen  time.Time  `json:"lastSeen"`             // Letzter Kontaktversuch
	Note      string     `json:"note,omitempty"`       // Admin-Notiz (Dashboard)
}

// Store verwaltet Pairing-Einträge (Thread-Safe, JSON-persistiert).
type Store struct {
	mu       sync.RWMutex
	entries  map[string]*PairEntry // Key: "channel:userId" (z.B. "telegram:123456789")
	filePath string
}

// New erstellt einen neuen Pairing-Store.
// storePath: Pfad zur JSON-Datei (z.B. workspace/pairing.json).
func New(storePath string) *Store {
	s := &Store{
		entries:  make(map[string]*PairEntry),
		filePath: storePath,
	}
	s.load()
	return s
}

// key erzeugt den Map-Schlüssel aus Channel + UserID.
func key(channel, userID string) string {
	return channel + ":" + userID
}

// IsPaired prüft ob ein User gepairt (approved) ist.
func (s *Store) IsPaired(channel, userID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[key(channel, userID)]
	return ok && e.Status == StatusApproved
}

// IsBlocked prüft ob ein User explizit blockiert ist.
func (s *Store) IsBlocked(channel, userID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[key(channel, userID)]
	return ok && e.Status == StatusBlocked
}

// RequestPairing registriert einen neuen Pairing-Request (Status: pending).
// Gibt true zurück wenn der Request neu ist, false wenn bereits vorhanden.
func (s *Store) RequestPairing(channel, userID, userName, chatID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(channel, userID)
	if e, ok := s.entries[k]; ok {
		// Update LastSeen + UserName (falls geändert)
		e.LastSeen = time.Now()
		if userName != "" {
			e.UserName = userName
		}
		if chatID != "" {
			e.ChatID = chatID
		}
		s.save()
		return false // bereits bekannt
	}

	// Neuer Eintrag
	now := time.Now()
	s.entries[k] = &PairEntry{
		UserID:    userID,
		UserName:  userName,
		Channel:   channel,
		Status:    StatusPending,
		ChatID:    chatID,
		CreatedAt: now,
		LastSeen:  now,
	}
	s.save()
	log.Printf("[Pairing] Neuer Request: %s @%s (Channel: %s)", userID, userName, channel)
	return true
}

// Approve genehmigt einen Pairing-Request.
func (s *Store) Approve(channel, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(channel, userID)
	e, ok := s.entries[k]
	if !ok {
		return fmt.Errorf("kein Pairing-Eintrag für %s", k)
	}
	now := time.Now()
	e.Status = StatusApproved
	e.ApprovedAt = &now
	s.save()
	log.Printf("[Pairing] ✅ Genehmigt: %s @%s (Channel: %s)", userID, e.UserName, channel)
	return nil
}

// Block blockiert einen User.
func (s *Store) Block(channel, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(channel, userID)
	e, ok := s.entries[k]
	if !ok {
		return fmt.Errorf("kein Pairing-Eintrag für %s", k)
	}
	e.Status = StatusBlocked
	s.save()
	log.Printf("[Pairing] 🚫 Blockiert: %s @%s (Channel: %s)", userID, e.UserName, channel)
	return nil
}

// Remove löscht einen Pairing-Eintrag komplett.
func (s *Store) Remove(channel, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(channel, userID)
	if _, ok := s.entries[k]; !ok {
		return fmt.Errorf("kein Pairing-Eintrag für %s", k)
	}
	delete(s.entries, k)
	s.save()
	log.Printf("[Pairing] 🗑️ Entfernt: %s (Channel: %s)", userID, channel)
	return nil
}

// SetNote setzt eine Admin-Notiz für einen Eintrag.
func (s *Store) SetNote(channel, userID, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	k := key(channel, userID)
	e, ok := s.entries[k]
	if !ok {
		return fmt.Errorf("kein Pairing-Eintrag für %s", k)
	}
	e.Note = note
	s.save()
	return nil
}

// GetEntry gibt einen einzelnen Eintrag zurück (oder nil).
func (s *Store) GetEntry(channel, userID string) *PairEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[key(channel, userID)]
	if !ok {
		return nil
	}
	// Kopie zurückgeben
	copy := *e
	return &copy
}

// List gibt alle Einträge zurück (optional gefiltert nach Status).
func (s *Store) List(statusFilter PairStatus) []PairEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []PairEntry
	for _, e := range s.entries {
		if statusFilter == "" || e.Status == statusFilter {
			result = append(result, *e)
		}
	}
	return result
}

// Stats gibt Statistiken zurück.
func (s *Store) Stats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := map[string]int{
		"total":    len(s.entries),
		"pending":  0,
		"approved": 0,
		"blocked":  0,
	}
	for _, e := range s.entries {
		stats[string(e.Status)]++
	}
	return stats
}

// ── Persistenz ────────────────────────────────────────────────────────────────

func (s *Store) load() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[Pairing] Warnung: %s konnte nicht gelesen werden: %v", s.filePath, err)
		}
		return
	}

	var entries []*PairEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		log.Printf("[Pairing] Warnung: %s konnte nicht geparst werden: %v", s.filePath, err)
		return
	}

	for _, e := range entries {
		s.entries[key(e.Channel, e.UserID)] = e
	}
	log.Printf("[Pairing] %d Einträge geladen aus %s", len(entries), filepath.Base(s.filePath))
}

func (s *Store) save() {
	entries := make([]*PairEntry, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, e)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		log.Printf("[Pairing] Fehler beim Serialisieren: %v", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(s.filePath), 0755); err != nil {
		log.Printf("[Pairing] Fehler beim Erstellen des Verzeichnisses: %v", err)
		return
	}

	if err := os.WriteFile(s.filePath, data, 0600); err != nil {
		log.Printf("[Pairing] Fehler beim Speichern: %v", err)
	}
}
