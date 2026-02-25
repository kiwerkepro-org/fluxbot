// Package cron implementiert das Reminder-System für FluxBot.
// Nutzer können über Chat-Befehle tägliche/wöchentliche/individuelle
// Erinnerungen anlegen. Diese werden persistent in workspace/reminders.json
// gespeichert und überleben Docker-Neustarts.
package cron

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// SendFunc ist die Callback-Funktion, die aufgerufen wird wenn ein Reminder auslöst.
// channelID: z.B. "telegram", chatID: z.B. "123456789"
type SendFunc func(channelID, chatID, text string)

// Manager verwaltet alle Cron-Jobs und Reminders.
type Manager struct {
	c        *cron.Cron
	store    *reminderStore
	sendFn   SendFunc
	entries  map[int]cron.EntryID // reminderID → cronEntryID
	mu       sync.Mutex
}

// New erstellt einen neuen CronManager.
// storePath: Pfad zur reminders.json (z.B. "workspace/reminders.json")
// sendFn:    Callback zum Versenden von Nachrichten
func New(storePath string, sendFn SendFunc) *Manager {
	m := &Manager{
		// Seconds-Field aktivieren für präzisere Planung
		c:       cron.New(cron.WithLocation(time.UTC)),
		store:   newStore(storePath),
		sendFn:  sendFn,
		entries: make(map[int]cron.EntryID),
	}
	return m
}

// Start startet den Scheduler und re-registriert alle gespeicherten Reminders.
func (m *Manager) Start() {
	// Alle persistierten Reminders wieder einplanen
	for _, r := range m.store.all() {
		if err := m.schedule(r); err != nil {
			log.Printf("[Cron] Warnung: Konnte Reminder %d nicht neu einplanen: %v", r.ID, err)
		}
	}
	m.c.Start()
	log.Printf("[Cron] Scheduler gestartet (%d Reminder(s) aktiv)", len(m.store.all()))
}

// Stop hält den Scheduler an.
func (m *Manager) Stop() {
	m.c.Stop()
	log.Println("[Cron] Scheduler gestoppt.")
}

// schedule registriert einen Reminder im Cron-Scheduler.
func (m *Manager) schedule(r *Reminder) error {
	// Zeitzone laden
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil {
		return fmt.Errorf("ungültige Zeitzone %q: %w", r.Timezone, err)
	}

	// Cron-Ausdruck mit Zeitzone: "CRON_TZ=Europe/Vienna 0 6 * * *"
	expr := fmt.Sprintf("CRON_TZ=%s %s", loc.String(), r.CronExpr)

	reminderID := r.ID
	channelID := r.ChannelID
	chatID := r.ChatID
	message := r.Message
	userName := r.UserName

	entryID, err := m.c.AddFunc(expr, func() {
		log.Printf("[Cron] Reminder %d ausgelöst für %s (%s/%s)", reminderID, userName, channelID, chatID)
		m.sendFn(channelID, chatID, "⏰ "+message)
	})
	if err != nil {
		return fmt.Errorf("cron.AddFunc fehlgeschlagen: %w", err)
	}

	m.mu.Lock()
	m.entries[r.ID] = entryID
	m.mu.Unlock()
	return nil
}

// AddReminder legt einen neuen Reminder an und plant ihn ein.
// Gibt eine Bestätigungs-Nachricht zurück.
func (m *Manager) AddReminder(r *Reminder) (string, error) {
	// Cron-Ausdruck validieren (ohne Zeitzone, nur um Syntax zu prüfen)
	if _, err := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse(r.CronExpr); err != nil {
		return "", fmt.Errorf("ungültiger Cron-Ausdruck %q: %w", r.CronExpr, err)
	}

	// Zeitzone validieren
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return "", fmt.Errorf("unbekannte Zeitzone %q", r.Timezone)
	}

	// Speichern (bekommt ID + CreatedAt zugewiesen)
	m.store.add(r)

	// Einplanen
	if err := m.schedule(r); err != nil {
		// Rollback: aus Store entfernen
		m.store.delete(r.ID)
		return "", err
	}

	return fmt.Sprintf("✅ Reminder #%d gespeichert: _%s_\n📅 %s", r.ID, r.Message, r.TimeStr), nil
}

// DeleteReminder entfernt einen Reminder anhand der ID.
// Gibt eine Bestätigungs-Nachricht zurück.
func (m *Manager) DeleteReminder(id int, userID string) (string, error) {
	// Prüfen ob der Reminder dem User gehört
	reminders := m.store.listByUser(userID)
	found := false
	for _, r := range reminders {
		if r.ID == id {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("Reminder #%d nicht gefunden (oder gehört dir nicht)", id)
	}

	// Aus Scheduler entfernen
	m.mu.Lock()
	if entryID, ok := m.entries[id]; ok {
		m.c.Remove(entryID)
		delete(m.entries, id)
	}
	m.mu.Unlock()

	// Aus Store entfernen
	m.store.delete(id)
	return fmt.Sprintf("🗑️ Reminder #%d gelöscht.", id), nil
}

// ListReminders gibt alle Reminders eines Users als formatierten Text zurück.
func (m *Manager) ListReminders(userID string) string {
	reminders := m.store.listByUser(userID)
	if len(reminders) == 0 {
		return "📋 Du hast keine aktiven Erinnerungen.\n\nBeispiel: _Erinnere mich täglich um 08:00 Uhr Europe/Vienna: Kaffee trinken!_"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 Deine Erinnerungen (%d):\n\n", len(reminders)))
	for _, r := range reminders {
		sb.WriteString(fmt.Sprintf("*#%d* – %s\n📅 %s\n\n", r.ID, r.Message, r.TimeStr))
	}
	sb.WriteString("_Zum Löschen: 'Lösch Erinnerung #ID'_")
	return sb.String()
}

// ParseReminderRequest interpretiert eine natürlichsprachliche Anfrage
// und erzeugt den passenden Cron-Ausdruck.
// Gibt (cronExpr, timeStr, error) zurück.
//
// Unterstützte Formate:
//   - "täglich um 06:00" → "0 6 * * *"
//   - "täglich um 06:30" → "30 6 * * *"
//   - "montags um 09:00" → "0 9 * * 1"
//   - "stündlich"        → "0 * * * *"
//   - "wöchentlich montags um 09:00" → "0 9 * * 1"
func ParseReminderRequest(timeSpec, timezone string) (cronExpr, timeStr string, err error) {
	lower := strings.ToLower(strings.TrimSpace(timeSpec))

	// Stündlich
	if strings.Contains(lower, "stündlich") || strings.Contains(lower, "jede stunde") {
		return "0 * * * *", fmt.Sprintf("Stündlich (%s)", timezone), nil
	}

	// Täglich – extrahiere Uhrzeit HH:MM
	if strings.Contains(lower, "täglich") || strings.Contains(lower, "jeden tag") || strings.Contains(lower, "every day") {
		hhmm, ok := extractTime(lower)
		if !ok {
			return "", "", fmt.Errorf("keine Uhrzeit gefunden. Bitte Format: 'täglich um HH:MM' verwenden")
		}
		h, min := splitHHMM(hhmm)
		return fmt.Sprintf("%d %d * * *", min, h),
			fmt.Sprintf("Täglich um %s Uhr (%s)", hhmm, timezone), nil
	}

	// Wochentage
	dayMap := map[string]string{
		"montag": "1", "mo": "1",
		"dienstag": "2", "di": "2",
		"mittwoch": "3", "mi": "3",
		"donnerstag": "4", "do": "4",
		"freitag": "5", "fr": "5",
		"samstag": "6", "sa": "6",
		"sonntag": "0", "so": "0",
		"monday": "1", "tuesday": "2", "wednesday": "3",
		"thursday": "4", "friday": "5", "saturday": "6", "sunday": "0",
	}
	for dayName, dayNum := range dayMap {
		if strings.Contains(lower, dayName) {
			hhmm, ok := extractTime(lower)
			if !ok {
				return "", "", fmt.Errorf("keine Uhrzeit gefunden. Bitte Format: '%ss um HH:MM' verwenden", dayName)
			}
			h, min := splitHHMM(hhmm)
			return fmt.Sprintf("%d %d * * %s", min, h, dayNum),
				fmt.Sprintf("Jeden %s um %s Uhr (%s)", strings.Title(dayName), hhmm, timezone), nil
		}
	}

	return "", "", fmt.Errorf("Zeitformat nicht erkannt. Beispiele:\n\u2022 'täglich um 08:00'\n\u2022 'montags um 09:30'\n\u2022 'stündlich'")
}

// extractTime sucht ein HH:MM-Muster in einem String.
func extractTime(s string) (string, bool) {
	// Suche nach "HH:MM" Muster
	for i := 0; i < len(s)-4; i++ {
		if s[i] >= '0' && s[i] <= '2' &&
			s[i+1] >= '0' && s[i+1] <= '9' &&
			s[i+2] == ':' &&
			s[i+3] >= '0' && s[i+3] <= '5' &&
			s[i+4] >= '0' && s[i+4] <= '9' {
			return s[i : i+5], true
		}
		// Einfaches "H:MM" (einstellige Stunde)
		if s[i] >= '0' && s[i] <= '9' &&
			i+1 < len(s) && s[i+1] == ':' &&
			i+3 < len(s) && s[i+2] >= '0' && s[i+2] <= '5' &&
			s[i+3] >= '0' && s[i+3] <= '9' {
			return "0" + s[i:i+4], true
		}
	}
	return "", false
}

// splitHHMM trennt "HH:MM" in Stunde (int) und Minute (int).
func splitHHMM(hhmm string) (hour, minute int) {
	parts := strings.SplitN(hhmm, ":", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	fmt.Sscanf(parts[0], "%d", &hour)
	fmt.Sscanf(parts[1], "%d", &minute)
	return
}
