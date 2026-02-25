package cron

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

// Reminder beschreibt eine einzelne geplante Erinnerung.
type Reminder struct {
	ID        int       `json:"id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	ChannelID string    `json:"channel_id"`
	ChatID    string    `json:"chat_id"`
	CronExpr  string    `json:"cron_expr"`  // robfig/cron Ausdruck, z.B. "0 6 * * *"
	TimeStr   string    `json:"time_str"`   // Lesbarer String, z.B. "täglich 06:00 Europe/Vienna"
	Timezone  string    `json:"timezone"`   // z.B. "Europe/Vienna"
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// reminderStore verwaltet alle Reminders persistent (JSON-Datei).
type reminderStore struct {
	mu        sync.RWMutex
	reminders []*Reminder
	nextID    int
	path      string
}

func newStore(path string) *reminderStore {
	s := &reminderStore{path: path, nextID: 1}
	s.load()
	return s
}

// load liest die JSON-Datei vom Disk. Fehler sind nicht fatal (leere Liste).
func (s *reminderStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return // Datei existiert noch nicht – okay
	}
	if err := json.Unmarshal(data, &s.reminders); err != nil {
		log.Printf("[Cron] Fehler beim Laden der Reminders: %v", err)
		return
	}
	// Höchste ID ermitteln für nextID
	for _, r := range s.reminders {
		if r.ID >= s.nextID {
			s.nextID = r.ID + 1
		}
	}
	log.Printf("[Cron] %d Reminder(s) geladen aus %s", len(s.reminders), s.path)
}

// save schreibt alle Reminders in die JSON-Datei.
func (s *reminderStore) save() {
	data, err := json.MarshalIndent(s.reminders, "", "  ")
	if err != nil {
		log.Printf("[Cron] Fehler beim Serialisieren der Reminders: %v", err)
		return
	}
	if err := os.WriteFile(s.path, data, 0600); err != nil {
		log.Printf("[Cron] Fehler beim Speichern der Reminders: %v", err)
	}
}

// add fügt einen neuen Reminder hinzu und vergibt eine ID.
func (s *reminderStore) add(r *Reminder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ID = s.nextID
	s.nextID++
	r.CreatedAt = time.Now()
	s.reminders = append(s.reminders, r)
	s.save()
}

// delete entfernt einen Reminder anhand der ID.
// Gibt true zurück wenn gefunden + gelöscht.
func (s *reminderStore) delete(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, r := range s.reminders {
		if r.ID == id {
			s.reminders = append(s.reminders[:i], s.reminders[i+1:]...)
			s.save()
			return true
		}
	}
	return false
}

// listByUser gibt alle Reminders eines bestimmten Users zurück.
func (s *reminderStore) listByUser(userID string) []*Reminder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Reminder
	for _, r := range s.reminders {
		if r.UserID == userID {
			result = append(result, r)
		}
	}
	return result
}

// all gibt alle gespeicherten Reminders zurück (für Re-Registrierung beim Start).
func (s *reminderStore) all() []*Reminder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]*Reminder, len(s.reminders))
	copy(cp, s.reminders)
	return cp
}
