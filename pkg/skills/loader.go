package skills

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Loader lädt alle Skills aus dem Workspace und baut den Skill-Baum auf.
type Loader struct {
	workspacePath string
	skills        map[string]*Skill // name → Skill
}

// NewLoader erstellt einen neuen Skills-Loader und lädt alle Skills sofort.
func NewLoader(workspacePath string) *Loader {
	l := &Loader{
		workspacePath: workspacePath,
		skills:        make(map[string]*Skill),
	}
	l.loadAll()
	return l
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
		skill, err := l.parseSkillFile(path)
		if err != nil {
			log.Printf("[Skills] Fehler beim Laden von %s: %v", path, err)
			continue
		}
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

	// Frontmatter parsen wenn vorhanden
	if strings.HasPrefix(content, "---") {
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
