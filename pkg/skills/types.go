package skills

// Skill repräsentiert einen geladenen Skill aus einer Markdown-Datei.
// Unterstützt Hierarchie und Disambiguierung.
type Skill struct {
	Name                   string   // Eindeutiger Name (z.B. "wordpress-plugin")
	Path                   string   // Pfad zur .md-Datei
	Content                string   // Vollständiger Markdown-Inhalt (ohne Frontmatter)
	Tags                   []string // Keywords für initiales Matching (z.B. ["wordpress","plugin"])
	DisambiguationKeywords []string // Keywords für Sub-Auflösung (z.B. ["addon","erweiterung"])
	FollowUpQuestion       string   // Rückfrage bei Mehrdeutigkeit (z.B. "Plugin oder Theme?")
	SubCategoryNames       []string // Namen der Sub-Skills (z.B. ["wordpress-plugin","wordpress-theme"])
	Parent                 string   // Name des übergeordneten Skills (z.B. "wordpress")

	// Wird nach dem Laden befüllt
	SubCategories  []*Skill
	NeedsResigning bool // true = .sig vorhanden aber ungültig (Inhalt geändert)
}

// MatchResult ist das Ergebnis eines Skill-Matching-Vorgangs.
type MatchResult struct {
	Skill               *Skill   // Der gefundene Skill (nil wenn NeedsDisambiguation)
	Score               int      // Matching-Score
	NeedsDisambiguation bool     // true = Bot soll Rückfrage stellen
	Question            string   // Die Rückfrage an den User
	Options             []string // Kurzbezeichnungen der Optionen (für Anzeige)
	OptionSkills        []*Skill // Die Sub-Skills zur Auswahl
	FallbackContent     string   // Fallback-Inhalt (AGENTS.md) wenn kein Match
}

// DisambiguationState speichert den Zustand einer laufenden Disambiguierung in der Session.
type DisambiguationState struct {
	OriginalText string   // Ursprüngliche Benutzeranfrage
	Question     string   // Die gestellte Rückfrage
	Options      []*Skill // Die zur Auswahl stehenden Sub-Skills
	ParentSkill  string   // Name des übergeordneten Skills
}
