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
	workspacePath  string
	skills         map[string]*Skill // name → Skill (nur gültig signierte)
	invalidSkills  map[string]*Skill // name → Skill (ungültige Signatur – nur für Dashboard)
	secret         string            // HMAC-Secret für Signatur-Prüfung (leer = deaktiviert)
	integrations   map[string]string // Platzhalter-Name → Wert
}

// NewLoader erstellt einen neuen Skills-Loader und lädt alle Skills sofort.
func NewLoader(workspacePath string) *Loader {
	l := &Loader{
		workspacePath: workspacePath,
		skills:        make(map[string]*Skill),
		invalidSkills: make(map[string]*Skill),
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
	l.invalidSkills = make(map[string]*Skill)
	l.loadAll()
	log.Printf("[Skills] 🔄 Skills neu geladen (%d aktiv, %d benötigen Neu-Signierung)", len(l.skills), len(l.invalidSkills))
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

		// Signaturprüfung: ungültig signierte Skills werden ins Dashboard geladen (NeedsResigning=true),
		// aber NICHT für den Agenten freigegeben (nur l.invalidSkills, nicht l.skills).
		sigInvalid := l.isSignatureInvalid(path)
		if sigInvalid {
			skill, err := l.parseSkillFile(path)
			if err != nil {
				log.Printf("[Skills] Fehler beim Laden von %s: %v", path, err)
				continue
			}
			skill.NeedsResigning = true
			l.invalidSkills[skill.Name] = skill
			log.Printf("[Skills] ⚠️ Skill %s benötigt Neu-Signierung (Inhalt geändert)", filepath.Base(path))
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

// isSignatureInvalid gibt true zurück wenn eine .sig-Datei vorhanden ist aber nicht passt.
// (= Inhalt wurde nach dem letzten Signieren geändert → Neu-Signierung nötig)
// Unsignierte Skills (keine .sig) und Skills ohne gesetztem Secret gelten als gültig.
func (l *Loader) isSignatureInvalid(path string) bool {
	if l.secret == "" {
		return false // Signierung deaktiviert → immer gültig
	}
	ok, err := VerifyFile(path, l.secret)
	if err != nil {
		log.Printf("[Skills] ⚠️ Signaturprüfung Fehler für %s: %v", filepath.Base(path), err)
		return false // Lesefehler → nicht als ungültig markieren
	}
	if !ok {
		sigPath := path + ".sig"
		if _, statErr := os.Stat(sigPath); statErr == nil {
			return true // .sig vorhanden aber falsch = ungültig
		}
		log.Printf("[Skills] ℹ️ Skill %s ist unsigniert", filepath.Base(path))
		return true // keine .sig = unsigniert = muss signiert werden
	}
	return false
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

// GetByName gibt einen Skill anhand seines Namens zurück (nil wenn nicht gefunden).
func (l *Loader) GetByName(name string) *Skill {
	if s, ok := l.skills[name]; ok {
		return s
	}
	return nil
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

// ListSkills gibt alle geladenen Skills zurück – gültige + jene die neu signiert werden müssen.
// Für das Dashboard: ungültige Skills erscheinen mit NeedsResigning=true.
// Für den Agenten: nur l.skills (gültige) werden für Matching verwendet.
func (l *Loader) ListSkills() []*Skill {
	result := make([]*Skill, 0, len(l.skills)+len(l.invalidSkills))
	for _, s := range l.skills {
		result = append(result, s)
	}
	for _, s := range l.invalidSkills {
		result = append(result, s)
	}
	return result
}

// SignSkill signiert eine Skill-Datei neu und verschiebt sie nach dem Signieren in l.skills.
func (l *Loader) SignSkill(skillName string) error {
	if l.secret == "" {
		return fmt.Errorf("skill-secret nicht gesetzt")
	}
	// In gültigen Skills suchen
	skill, ok := l.skills[skillName]
	if !ok {
		// In ungültigen Skills suchen (nach Umbenennung o.ä.)
		skill, ok = l.invalidSkills[skillName]
		if !ok {
			return fmt.Errorf("skill '%s' nicht gefunden", skillName)
		}
	}
	if err := SignFile(skill.Path, l.secret); err != nil {
		return err
	}
	// Nach erfolgreichem Signieren: aus invalidSkills entfernen und in skills übernehmen
	if skill.NeedsResigning {
		skill.NeedsResigning = false
		skill.Content = l.substituteIntegrations(skill.Content)
		delete(l.invalidSkills, skillName)
		l.skills[skillName] = skill
	}
	return nil
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
	s = strings.Trim(s, "[] ")
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
