package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ki-werke/fluxbot/pkg/skills"
)

// ConversationTurn speichert einen einzelnen Gesprächs-Turn
type ConversationTurn struct {
	Role    string `json:"role"`    // "user" oder "assistant"
	Content string `json:"content"`
}

// Memory speichert persistente Nutzerdaten
type Memory struct {
	UserFacts []string           `json:"user_facts"`
	History   []ConversationTurn `json:"history"`
}

const maxHistoryTurns = 20 // Maximale Anzahl gespeicherter Turns (10 Exchanges)

// ForgetState verwaltet den Zustand einer laufenden Lösch-Anfrage
type ForgetState struct {
	Options []int // Indizes der zur Wahl stehenden Fakten (0-basiert intern)
}

// ImageRequestState speichert eine ausstehende Bildanfrage (Provider- und Format-Auswahl)
type ImageRequestState struct {
	Prompt       string // Extrahierter Bild-Prompt
	Format       string // "landscape", "portrait", "square" – leer = noch nicht gewählt
	GeneratorIdx int    // -1 = noch nicht gewählt, >=0 = Index in imageGenerators
	Step         string // "provider" = warte auf Provider, "format" = warte auf Format
}

// Session verwaltet den Zustand einer Benutzer-Session
type Session struct {
	UserID    string
	ChannelID string
	Memory    Memory

	// Disambiguierungsstatus (nil wenn keine aktive Disambiguierung)
	Disambiguation *skills.DisambiguationState

	// Löschstatus (nil wenn keine aktive Löschanfrage)
	ForgetState *ForgetState

	// Ausstehende Bildanfrage (nil wenn keine aktive Provider-Auswahl)
	ImageRequest *ImageRequestState

	memPath string
}

// SessionManager verwaltet alle aktiven Sessions
type SessionManager struct {
	workspacePath string
	sessions      map[string]*Session
}

// NewSessionManager erstellt einen neuen Session-Manager
func NewSessionManager(workspacePath string) *SessionManager {
	return &SessionManager{
		workspacePath: workspacePath,
		sessions:      make(map[string]*Session),
	}
}

// GetOrCreate gibt eine bestehende Session zurück oder erstellt eine neue
func (sm *SessionManager) GetOrCreate(userID, channelID string) *Session {
	key := channelID + ":" + userID
	if s, ok := sm.sessions[key]; ok {
		return s
	}

	s := &Session{
		UserID:    userID,
		ChannelID: channelID,
		memPath:   filepath.Join(sm.workspacePath, "sessions", channelID+"_"+userID+".json"),
	}
	s.Memory = s.loadMemory()
	sm.sessions[key] = s
	return s
}

// loadMemory lädt das Gedächtnis aus einer JSON-Datei
func (s *Session) loadMemory() Memory {
	var mem Memory
	data, err := os.ReadFile(s.memPath)
	if err == nil {
		json.Unmarshal(data, &mem)
	}
	return mem
}

// SaveMemory speichert das Gedächtnis in eine JSON-Datei
func (s *Session) SaveMemory() {
	if err := os.MkdirAll(filepath.Dir(s.memPath), 0755); err != nil {
		log.Printf("[Session] Fehler beim Erstellen des Verzeichnisses: %v", err)
		return
	}
	data, _ := json.MarshalIndent(s.Memory, "", "  ")
	if err := os.WriteFile(s.memPath, data, 0644); err != nil {
		log.Printf("[Session] Fehler beim Speichern des Gedächtnisses: %v", err)
	}
}

// AddFact fügt einen neuen Fakt zum Gedächtnis hinzu
func (s *Session) AddFact(fact string) {
	s.Memory.UserFacts = append(s.Memory.UserFacts, fact)
	s.SaveMemory()
}

// FactsSummary gibt alle Fakten als String zurück
func (s *Session) FactsSummary() string {
	if len(s.Memory.UserFacts) == 0 {
		return ""
	}
	result := ""
	for i, f := range s.Memory.UserFacts {
		if i > 0 {
			result += "; "
		}
		result += f
	}
	return result
}

// ListFacts gibt eine nummerierte Liste der Fakten zurück (1-basiert für den Nutzer)
func (s *Session) ListFacts() string {
	if len(s.Memory.UserFacts) == 0 {
		return "Mein Gedächtnis ist leer."
	}
	var sb strings.Builder
	sb.WriteString("📋 Mein Gedächtnis über dich:\n")
	for i, f := range s.Memory.UserFacts {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, f))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// DeleteFactAt löscht einen Fakt anhand seines 0-basierten Index
// Gibt true zurück wenn erfolgreich
func (s *Session) DeleteFactAt(index int) bool {
	if index < 0 || index >= len(s.Memory.UserFacts) {
		return false
	}
	s.Memory.UserFacts = append(s.Memory.UserFacts[:index], s.Memory.UserFacts[index+1:]...)
	s.SaveMemory()
	return true
}

// DeleteFactsAt löscht mehrere Fakten anhand ihrer 0-basierten Indizes (in absteigender Reihenfolge)
func (s *Session) DeleteFactsAt(indices []int) {
	// Absteigend sortieren damit Indizes beim Löschen stabil bleiben
	for i := len(indices) - 1; i >= 0; i-- {
		idx := indices[i]
		if idx >= 0 && idx < len(s.Memory.UserFacts) {
			s.Memory.UserFacts = append(s.Memory.UserFacts[:idx], s.Memory.UserFacts[idx+1:]...)
		}
	}
	s.SaveMemory()
}

// ClearAllFacts löscht das gesamte Gedächtnis
func (s *Session) ClearAllFacts() {
	s.Memory.UserFacts = []string{}
	s.SaveMemory()
}

// AddToHistory fügt einen Turn zum Gesprächsverlauf hinzu und trimmt auf maxHistoryTurns
func (s *Session) AddToHistory(role, content string) {
	s.Memory.History = append(s.Memory.History, ConversationTurn{Role: role, Content: content})
	if len(s.Memory.History) > maxHistoryTurns {
		s.Memory.History = s.Memory.History[len(s.Memory.History)-maxHistoryTurns:]
	}
	s.SaveMemory()
}

// ClearHistory löscht den gesamten Gesprächsverlauf
func (s *Session) ClearHistory() {
	s.Memory.History = []ConversationTurn{}
	s.SaveMemory()
}

// FindFactsByKeyword sucht Fakten, die das Keyword enthalten (case-insensitiv)
// Gibt die 0-basierten Indizes zurück
func (s *Session) FindFactsByKeyword(keyword string) []int {
	lower := strings.ToLower(keyword)
	var matches []int
	for i, f := range s.Memory.UserFacts {
		if strings.Contains(strings.ToLower(f), lower) {
			matches = append(matches, i)
		}
	}
	return matches
}
