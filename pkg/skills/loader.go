package skills

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Loader lädt alle Skills aus dem Workspace und baut den Skill-Baum auf.
type Loader struct {
	workspacePath string
	skills        map[string]*Skill // name → Skill
	secret        string            // HMAC-Secret für Signatur-Prüfung (leer = deaktiviert)
	integrations  map[string]string // Platzhalter-Name → Wert
}

// NewLoader erstellt einen neuen Skills-Loader und lädt alle Skills sofort.
func NewLoader(workspacePath string) *Loader {
	l := &Loader{
		workspacePath: workspacePath,
		skills:        make(map[string]*Skill),
		integrations:  make(map[string]string),
	}
	l.loadAll()
	return l
}

// SetSecret setzt den HMAC-Secret für Signaturprüfung.
func (l *Loader) SetSecret(secret string) {
	l.secret = secret
}

// SetIntegrations setzt die Integrations-Platzhalter (Name → Wert).
func (l *Loader) SetIntegrations(integrations map[string]string) {
	l.integrations = integrations
}

// Reload lädt alle Skills neu und wendet die aktuellen Integrationen an.
// Muss nach SetIntegrations aufgerufen werden damit Platzhalter ersetzt werden.
func (l *Loader) Reload() {
	l.skills = make(map[string]*Skill)
	l.loadAll()
	log.Printf("[Skills] 🔄 Skills neu geladen (%d Skills aktiv)", len(l.skills))
}

// SaveAndSign speichert einen Skill als .md-Datei, signiert ihn und lädt ihn sofort.
func (l *Loader) SaveAndSign(name, content string) error {
	skillsDir := filepath.Join(l.workspacePath, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("skills-verzeichnis erstellen: %w", err)
	}

	// Dateiname sichern: nur alphanumerisch + Bindestrich
	safeName := sanitizeSkillName(name)
	path := filepath.Join(skillsDir, safeName+".md")

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("skill schreiben: %w", err)
	}
	log.Printf("[Skills] ✅ Skill gespeichert: %s", path)

	// Signieren wenn Secret gesetzt
	if l.secret != "" {
		if err := SignFile(path, l.secret); err != nil {
			log.Printf("[Skills] ⚠️ Signierung fehlgeschlagen: %v", err)
		} else {
			log.Printf("[Skills] 🔐 Skill signiert: %s.sig", path)
		}
	}

	// Sofort in Speicher laden
	skill, err := l.parseSkillFile(path)
	if err != nil {
		return fmt.Errorf("skill parsen: %w", err)
	}
	l.skills[skill.Name] = skill
	return nil
}

// sanitizeSkillName bereinigt einen Skill-Namen für den Dateisystem-Einsatz.
func sanitizeSkillName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		} else if r == ' ' {
			result.WriteRune('-')
		}
	}
	s := result.String()
	if s == "" {
		s = "skill"
	}
	return s
}

// loadAll lädt alle .md-Dateien aus dem skills/-Verzeichnis und AGENTS.md.
func (l *Loader) loadAll() {
	skillsDir := filepath.Join(l.workspacePath, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		log.Printf("[Skills] Kein skills/-Verzeichnis gefunden: %v", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(skillsDir, entry.Name())

		// Signaturprüfung: manipulierte Skills werden übersprungen
		if !l.verifySkill(path) {
			continue
		}

		skill, err := l.parseSkillFile(path)
		if err != nil {
			log.Printf("[Skills] Fehler beim Laden von %s: %v", path, err)
			continue
		}

		// Integrations-Platzhalter substituieren
		skill.Content = l.substituteIntegrations(skill.Content)

		l.skills[skill.Name] = skill
	}

	// Skill-Baum aufbauen: SubCategories verlinken
	for _, skill := range l.skills {
		for _, subName := range skill.SubCategoryNames {
			if sub, ok := l.skills[subName]; ok {
				skill.SubCategories = append(skill.SubCategories, sub)
			}
		}
	}

	log.Printf("[Skills] %d Skills geladen", len(l.skills))
}

// verifySkill prüft die Signatur einer Skill-Datei wenn ein Secret gesetzt ist.
// Gibt false zurück wenn die Datei manipuliert wurde (Signatur vorhanden aber falsch).
func (l *Loader) verifySkill(path string) bool {
	if l.secret == "" {
		return true // Signierung deaktiviert → alles erlaubt
	}
	ok, err := VerifyFile(path, l.secret)
	if err != nil {
		log.Printf("[Skills] ⚠️ Signaturprüfung Fehler für %s: %v", filepath.Base(path), err)
		return true // Fehler beim Lesen der .sig = unsigniert → erlaubt
	}
	if !ok {
		// Keine .sig-Datei = unsigniert (erlaubt mit Warnung)
		// Vorhandene aber falsche .sig = manipuliert (abgelehnt)
		sigPath := path + ".sig"
		if _, statErr := os.Stat(sigPath); statErr == nil {
			log.Printf("[Skills] 🚨 Skill %s wurde manipuliert – wird NICHT geladen!", filepath.Base(path))
			return false
		}
		log.Printf("[Skills] ℹ️ Skill %s ist unsigniert", filepath.Base(path))
	}
	return true
}

// substituteIntegrations ersetzt {{PLACEHOLDER}} mit den konfigurierten Integrationswerten.
func (l *Loader) substituteIntegrations(content string) string {
	if len(l.integrations) == 0 {
		return content
	}
	for name, value := range l.integrations {
		content = strings.ReplaceAll(content, "{{"+name+"}}", value)
	}
	return content
}

// parseSkillFile parst eine .md-Datei mit optionalem YAML-Frontmatter.
// Frontmatter-Format (zwischen --- Blöcken):
//
//	name: skill-name
//	tags: tag1,tag2,tag3
//	disambiguationKeywords: kw1,kw2
//	followUpQuestion: Deine Rückfrage?
//	subCategories: sub1,sub2
//	parent: parent-name
func (l *Loader) parseSkillFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	skill := &Skill{
		Path: path,
		// Standardname = Dateiname ohne .md
		Name: strings.TrimSuffix(filepath.Base(path), ".md"),
	}

	// Äußeren Code-Block-Wrapper entfernen (z.B. ```markdown ... ```)
	// Skill-Dateien werden manchmal mit diesem Wrapper gespeichert.
	if strings.HasPrefix(strings.TrimSpace(content), "```") {
		// Erste Zeile (```markdown o.ä.) entfernen
		if nl := strings.Index(content, "\n"); nl >= 0 {
			content = content[nl+1:]
		}
		// Schließendes ``` am Ende entfernen
		if idx := strings.LastIndex(content, "```"); idx >= 0 {
			content = strings.TrimSpace(content[:idx])
		}
	}

	// Frontmatter parsen wenn vorhanden
	if strings.HasPrefix(strings.TrimSpace(content), "---") {
		content = strings.TrimSpace(content)
		parts := strings.SplitN(content, "---", 3)
		if len(parts) >= 3 {
			l.parseFrontmatter(skill, parts[1])
			skill.Content = strings.TrimSpace(parts[2])
		} else {
			skill.Content = content
		}
	} else {
		skill.Content = content
	}

	// Fallback: Tags aus Dateiname ableiten wenn keine Tags gesetzt
	if len(skill.Tags) == 0 {
		skill.Tags = strings.Split(strings.ReplaceAll(skill.Name, "-", ","), ",")
	}

	return skill, nil
}

// parseFrontmatter parst den Frontmatter-Block und befüllt den Skill.
func (l *Loader) parseFrontmatter(skill *Skill, frontmatter string) {
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])

		switch key {
		case "name":
			skill.Name = value
		case "tags":
			skill.Tags = splitAndTrim(value)
		case "disambiguationKeywords":
			skill.DisambiguationKeywords = splitAndTrim(value)
		case "followUpQuestion":
			skill.FollowUpQuestion = value
		case "subCategories":
			skill.SubCategoryNames = splitAndTrim(value)
		case "parent":
			skill.Parent = value
		}
	}
}

// FindBestSkill gibt den Skill-Inhalt zurück, der am besten zum Prompt passt.
// Wird vom Agent für einfache Fälle genutzt (ohne Disambiguierung).
func (l *Loader) FindBestSkill(userPrompt string) string {
	result := l.Match(userPrompt)
	if result.NeedsDisambiguation {
		// Beim einfachen Aufruf: Eltern-Skill-Inhalt zurückgeben
		if len(result.OptionSkills) > 0 && result.OptionSkills[0].Parent != "" {
			if parent, ok := l.skills[result.OptionSkills[0].Parent]; ok {
				return parent.Content
			}
		}
		return ""
	}
	if result.Skill != nil {
		return result.Skill.Content
	}
	return result.FallbackContent
}

// Match führt das vollständige Skill-Matching durch und gibt ein MatchResult zurück.
// Wird vom Agent für den Disambiguierungs-Flow genutzt.
func (l *Loader) Match(userPrompt string) *MatchResult {
	matcher := NewMatcher(l.skills)
	return matcher.Match(userPrompt)
}

// ResolveDisambiguation löst eine offene Disambiguierung anhand der Benutzerantwort auf.
func (l *Loader) ResolveDisambiguation(state *DisambiguationState, response string) *MatchResult {
	matcher := NewMatcher(l.skills)
	return matcher.Resolve(state, response)
}

// ListSkills gibt alle geladenen Skills zurück.
func (l *Loader) ListSkills() []*Skill {
	result := make([]*Skill, 0, len(l.skills))
	for _, s := range l.skills {
		result = append(result, s)
	}
	return result
}

// SignSkill signiert eine Skill-Datei neu.
func (l *Loader) SignSkill(skillName string) error {
	skill, ok := l.skills[skillName]
	if !ok {
		return fmt.Errorf("skill '%s' nicht gefunden", skillName)
	}
	if l.secret == "" {
		return fmt.Errorf("skill-secret nicht gesetzt")
	}
	return SignFile(skill.Path, l.secret)
}

// loadDefaultAgents lädt den AGENTS.md-Fallback
func (l *Loader) loadDefaultAgents() string {
	for _, name := range []string{"AGENTS.md", "master.md"} {
		path := filepath.Join(l.workspacePath, name)
		if data, err := os.ReadFile(path); err == nil {
			return string(data)
		}
	}
	return ""
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
