package skills

import (
	"strings"
)

// Matcher implementiert den Skill-Matching-Algorithmus mit Disambiguierung.
type Matcher struct {
	skills map[string]*Skill
}

// NewMatcher erstellt einen neuen Matcher.
func NewMatcher(skills map[string]*Skill) *Matcher {
	return &Matcher{skills: skills}
}

// Match findet den passendsten Skill für eine Benutzeranfrage.
//
// Algorithmus:
//  1. Alle Skills gegen den Prompt scoren
//  2. Wenn ein Skill klar dominiert → zurückgeben
//  3. Wenn mehrere Skills ähnlich scoren UND ein gemeinsamer Eltern-Skill
//     eine FollowUpQuestion hat → Disambiguierung einleiten
func (m *Matcher) Match(userPrompt string) *MatchResult {
	lower := strings.ToLower(userPrompt)
	scores := m.scoreAll(lower)

	if len(scores) == 0 {
		return &MatchResult{FallbackContent: m.loadFallback()}
	}

	// Top-Skill und Score ermitteln
	best := scores[0]

	// Kein Match gefunden
	if best.score == 0 {
		return &MatchResult{FallbackContent: m.loadFallback()}
	}

	// Prüfen ob eindeutig oder mehrdeutig
	// Eindeutig: top-Score ist deutlich höher als alle anderen, UND keine aktiven Sub-Kategorien
	if len(scores) == 1 || (scores[0].score-scores[1].score) >= 10 {
		// Klarer Gewinner – aber hat der Gewinner Sub-Skills die spezifischer matchen?
		winner := best.skill
		if len(winner.SubCategories) > 0 {
			subResult := m.matchSubCategories(winner, lower)
			if subResult != nil {
				return subResult
			}
		}
		return &MatchResult{Skill: winner, Score: best.score}
	}

	// Mehrdeutig: mehrere Skills mit ähnlichem Score
	// Finde den gemeinsamen Eltern-Skill
	topSkills := m.getTopSkills(scores, 3)
	parent := m.findCommonParent(topSkills)

	if parent != nil && parent.FollowUpQuestion != "" {
		// Disambiguierung über den Eltern-Skill
		return m.buildDisambiguationResult(parent, topSkills)
	}

	// Kein gemeinsamer Eltern: den besten nehmen
	return &MatchResult{Skill: best.skill, Score: best.score}
}

// matchSubCategories prüft ob Sub-Skills spezifischer matchen als der Eltern-Skill.
func (m *Matcher) matchSubCategories(parent *Skill, lowerPrompt string) *MatchResult {
	type subScore struct {
		skill *Skill
		score int
	}

	var subScores []subScore
	for _, sub := range parent.SubCategories {
		score := m.scoreSkill(sub, lowerPrompt)
		if score > 0 {
			subScores = append(subScores, subScore{sub, score})
		}
	}

	if len(subScores) == 0 {
		// Keine Sub-Skills matchen → Eltern-Skill ist klar genug
		return nil
	}

	// Sortieren
	for i := 0; i < len(subScores)-1; i++ {
		for j := i + 1; j < len(subScores); j++ {
			if subScores[j].score > subScores[i].score {
				subScores[i], subScores[j] = subScores[j], subScores[i]
			}
		}
	}

	// Ein Sub-Skill dominiert klar
	if len(subScores) == 1 || (subScores[0].score-subScores[1].score) >= 10 {
		return &MatchResult{Skill: subScores[0].skill, Score: subScores[0].score}
	}

	// Mehrere Sub-Skills → Disambiguierung über den Eltern-Skill
	if parent.FollowUpQuestion != "" {
		topSubs := make([]*Skill, 0, len(subScores))
		for _, s := range subScores {
			topSubs = append(topSubs, s.skill)
		}
		return m.buildDisambiguationResult(parent, topSubs)
	}

	// Kein FollowUpQuestion: besten Sub-Skill nehmen
	return &MatchResult{Skill: subScores[0].skill, Score: subScores[0].score}
}

// Resolve löst eine Disambiguierung anhand der Benutzerantwort auf.
func (m *Matcher) Resolve(state *DisambiguationState, response string) *MatchResult {
	lower := strings.ToLower(response)

	type candidate struct {
		skill *Skill
		score int
	}

	var candidates []candidate

	// Antwort gegen alle Optionen scoren
	for _, option := range state.Options {
		score := 0

		// Tags prüfen
		for _, tag := range option.Tags {
			if strings.Contains(lower, strings.ToLower(tag)) {
				score += 10
			}
		}

		// DisambiguationKeywords prüfen (höhere Gewichtung)
		for _, kw := range option.DisambiguationKeywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				score += 15
			}
		}

		// Direkter Name-Match (z.B. User sagt "Plugin" → matcht "wordpress-plugin")
		nameParts := strings.Split(option.Name, "-")
		for _, part := range nameParts {
			if strings.Contains(lower, part) {
				score += 12
			}
		}

		if score > 0 {
			candidates = append(candidates, candidate{option, score})
		}
	}

	if len(candidates) == 0 {
		// Keine Auflösung möglich
		return &MatchResult{
			NeedsDisambiguation: true,
			Question:            "Ich habe das nicht verstanden. " + state.Question,
			Options:             m.buildOptionLabels(state.Options),
			OptionSkills:        state.Options,
		}
	}

	// Sortieren
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	best := candidates[0]

	// Hat der aufgelöste Skill selbst noch Sub-Kategorien?
	if len(best.skill.SubCategories) > 0 && best.skill.FollowUpQuestion != "" {
		// Weitere Disambiguierungsebene
		return m.buildDisambiguationResult(best.skill, best.skill.SubCategories)
	}

	return &MatchResult{Skill: best.skill, Score: best.score}
}

// --- Hilfsfunktionen ---

type scoredSkill struct {
	skill *Skill
	score int
}

// scoreAll berechnet den Score aller Top-Level-Skills (ohne Sub-Skills mit Parent).
func (m *Matcher) scoreAll(lowerPrompt string) []scoredSkill {
	var results []scoredSkill

	for _, skill := range m.skills {
		// Nur Top-Level-Skills (ohne Parent) für initiales Matching
		if skill.Parent != "" {
			continue
		}
		score := m.scoreSkill(skill, lowerPrompt)
		if score > 0 {
			results = append(results, scoredSkill{skill, score})
		}
	}

	// Absteigend sortieren
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// scoreSkill berechnet den Matching-Score eines einzelnen Skills.
func (m *Matcher) scoreSkill(skill *Skill, lowerPrompt string) int {
	score := 0

	// Tags: +10 pro Treffer
	for _, tag := range skill.Tags {
		if strings.Contains(lowerPrompt, strings.ToLower(tag)) {
			score += 10
		}
	}

	// DisambiguationKeywords: +5 pro Treffer (schwächere Gewichtung im initialen Match)
	for _, kw := range skill.DisambiguationKeywords {
		if strings.Contains(lowerPrompt, strings.ToLower(kw)) {
			score += 5
		}
	}

	return score
}

// getTopSkills gibt die N besten Skills zurück.
func (m *Matcher) getTopSkills(scores []scoredSkill, n int) []*Skill {
	result := make([]*Skill, 0, n)
	for i, s := range scores {
		if i >= n {
			break
		}
		result = append(result, s.skill)
	}
	return result
}

// findCommonParent findet den gemeinsamen Eltern-Skill einer Gruppe von Skills.
func (m *Matcher) findCommonParent(skills []*Skill) *Skill {
	if len(skills) == 0 {
		return nil
	}

	// Prüfe ob alle Skills den gleichen Parent haben
	parentName := skills[0].Parent
	if parentName == "" {
		return nil
	}

	for _, s := range skills[1:] {
		if s.Parent != parentName {
			return nil
		}
	}

	if parent, ok := m.skills[parentName]; ok {
		return parent
	}
	return nil
}

// buildDisambiguationResult erstellt ein MatchResult mit Disambiguierungsfrage.
func (m *Matcher) buildDisambiguationResult(parent *Skill, options []*Skill) *MatchResult {
	labels := m.buildOptionLabels(options)
	return &MatchResult{
		NeedsDisambiguation: true,
		Question:            parent.FollowUpQuestion,
		Options:             labels,
		OptionSkills:        options,
	}
}

// buildOptionLabels erstellt lesbare Kurzbezeichnungen für die Optionen.
func (m *Matcher) buildOptionLabels(options []*Skill) []string {
	labels := make([]string, 0, len(options))
	for _, opt := range options {
		// Nehme den letzten Teil des Namens als Label (z.B. "wordpress-plugin" → "Plugin")
		parts := strings.Split(opt.Name, "-")
		label := strings.Title(parts[len(parts)-1])
		labels = append(labels, label)
	}
	return labels
}

// loadFallback lädt den AGENTS.md-Fallback (wenn kein Skill passt).
func (m *Matcher) loadFallback() string {
	// Wird vom Loader überschrieben – hier leer
	return ""
}
