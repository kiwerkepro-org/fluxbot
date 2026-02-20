package agent

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/ki-werke/fluxbot/pkg/channels"
	"github.com/ki-werke/fluxbot/pkg/provider"
	"github.com/ki-werke/fluxbot/pkg/security"
	"github.com/ki-werke/fluxbot/pkg/skills"
	"github.com/ki-werke/fluxbot/pkg/imagegen"
	"github.com/ki-werke/fluxbot/pkg/voice"
)

// Agent ist der Kern von FluxBot
type Agent struct {
	manager         *channels.Manager
	provider        provider.Provider
	sessions        *SessionManager
	skillsLoader    *skills.Loader
	models          map[string]string
	transcriber     voice.Transcriber
	voiceLang       string
	guard           *security.Guard
	imageGenerators []imagegen.Generator // Alle verfügbaren Bildgeneratoren (in Auswahl-Reihenfolge)
	imageSize       string
	videoDefault    string // "disabled" oder Provider-Name – steuert Video-Meldung
	soul            string // Inhalt von SOUL.md + IDENTITY.md (Persönlichkeit)
	systemPromptFn  func(session *Session, rules string) string
}

// Config enthält die Konfiguration für den Agent
type Config struct {
	Provider        provider.Provider
	Manager         *channels.Manager
	Sessions        *SessionManager
	SkillsLoader    *skills.Loader
	Models          map[string]string
	Transcriber     voice.Transcriber
	VoiceLang       string
	Guard           *security.Guard      // Optional – nil = kein Security-Check
	ImageGenerators []imagegen.Generator // Optional – leer = keine Bildgenerierung
	ImageSize       string
	VideoDefault    string // "disabled" oder Provider-Name – steuert Video-Meldung
	Soul            string // Inhalt von SOUL.md + IDENTITY.md (leer = nur Basis-Prompt)
}

// New erstellt einen neuen Agent
func New(cfg Config) *Agent {
	lang := cfg.VoiceLang
	if lang == "" {
		lang = "de"
	}
	a := &Agent{
		manager:         cfg.Manager,
		provider:        cfg.Provider,
		sessions:        cfg.Sessions,
		skillsLoader:    cfg.SkillsLoader,
		models:          cfg.Models,
		transcriber:     cfg.Transcriber,
		voiceLang:       lang,
		guard:           cfg.Guard,
		imageGenerators: cfg.ImageGenerators,
		imageSize:       cfg.ImageSize,
		videoDefault:    cfg.VideoDefault,
		soul:            cfg.Soul,
	}
	a.systemPromptFn = a.buildSystemPrompt
	return a
}

// UpdateImageGenerators ersetzt die aktiven Bild-Generatoren zur Laufzeit.
// Wird aufgerufen wenn die Config im Dashboard geändert wird.
func (a *Agent) UpdateImageGenerators(gens []imagegen.Generator) {
	a.imageGenerators = gens
	if len(gens) == 0 {
		log.Println("[Agent] 🔄 Bildgenerierung deaktiviert (0 Generatoren).")
	} else {
		names := make([]string, len(gens))
		for i, g := range gens {
			names[i] = g.Name()
		}
		log.Printf("[Agent] 🔄 Bildgeneratoren neu geladen: %v", names)
	}
}

// Run startet den Agent-Loop
func (a *Agent) Run(ctx context.Context) {
	voiceStatus := "deaktiviert"
	if a.transcriber != nil {
		voiceStatus = a.transcriber.Name()
	}
	guardStatus := "deaktiviert"
	if a.guard != nil {
		guardStatus = "aktiv"
	}
	log.Printf("[Agent] Agent-Loop gestartet | Voice: %s | Security: %s", voiceStatus, guardStatus)

	for {
		select {
		case <-ctx.Done():
			log.Println("[Agent] Context abgebrochen – Agent beendet")
			return
		case msg, ok := <-a.manager.Messages():
			if !ok {
				log.Println("[Agent] Nachrichten-Bus geschlossen")
				return
			}
			go a.handleMessage(ctx, msg)
		}
	}
}

// handleMessage verarbeitet eine einzelne eingehende Nachricht
func (a *Agent) handleMessage(ctx context.Context, msg channels.Message) {
	session := a.sessions.GetOrCreate(msg.SenderID, msg.ChannelID)
	a.manager.Typing(msg)

	// ── SECURITY CHECK ──────────────────────────────────────────────────────
	if a.guard != nil && msg.Type == channels.MessageTypeText {
		result := a.guard.Check(msg.ChannelID, msg.SenderID, string(msg.Type), msg.Text)
		if !result.Allowed {
			if err := a.manager.Reply(msg, result.Response); err != nil {
				log.Printf("[Agent] Fehler beim Senden der Security-Antwort: %v", err)
			}
			return
		}
		// Text auf sichere Länge kürzen
		msg.Text = security.SanitizeText(msg.Text, 4000)
	}

	switch msg.Type {
	case channels.MessageTypeVoice:
		a.handleVoice(ctx, msg, session)
	case channels.MessageTypeText:
		text := msg.Text
		response := a.processText(ctx, msg, session)
		// Leere Antwort = Bild wurde direkt gesendet (kein Text mehr nötig)
		if response == "" {
			break
		}
		// Gesprächsverlauf speichern
		session.AddToHistory("user", text)
		session.AddToHistory("assistant", response)
		if err := a.manager.Reply(msg, response); err != nil {
			log.Printf("[Agent] Fehler beim Senden: %v", err)
		}
	default:
		log.Printf("[Agent] Unbekannter Nachrichtentyp: %s", msg.Type)
	}
}

// handleVoice verarbeitet Sprachnachrichten
func (a *Agent) handleVoice(ctx context.Context, msg channels.Message, session *Session) {
	if msg.MediaPath != "" {
		defer os.Remove(msg.MediaPath)
	}

	if a.transcriber == nil {
		a.manager.Reply(msg, "🎙️ Spracherkennung ist nicht aktiviert.\nFüge in config.json hinzu:\n\"voice\": {\"enabled\": true, \"provider\": \"groq\", \"apiKey\": \"DEIN_GROQ_KEY\"}")
		return
	}

	if msg.MediaPath == "" {
		a.manager.Reply(msg, "❌ Sprachnachricht konnte nicht heruntergeladen werden.")
		return
	}

	log.Printf("[Agent] Transkribiere | Provider: %s", a.transcriber.Name())
	text, err := a.transcriber.Transcribe(ctx, msg.MediaPath, a.voiceLang)
	if err != nil {
		log.Printf("[Agent] Transkriptions-Fehler: %v", err)
		a.manager.Reply(msg, fmt.Sprintf("❌ Transkription fehlgeschlagen: %v", err))
		return
	}

	if strings.TrimSpace(text) == "" {
		a.manager.Reply(msg, "🎙️ Ich konnte nichts in der Sprachnachricht erkennen.")
		return
	}

	log.Printf("[Agent] Transkription: %s", text)
	textMsg := msg
	textMsg.Type = channels.MessageTypeText
	textMsg.Text = text

	response := a.processText(ctx, textMsg, session)
	// Leere Antwort = Bild wurde direkt gesendet, nur Transkription anzeigen
	if response == "" {
		a.manager.Reply(msg, fmt.Sprintf("🎙️ _%s_", text))
		return
	}
	// Gesprächsverlauf speichern (transkribierter Text als "user"-Turn)
	session.AddToHistory("user", text)
	session.AddToHistory("assistant", response)
	a.manager.Reply(msg, fmt.Sprintf("🎙️ _%s_\n\n%s", text, response))
}

// processText verarbeitet eine Textnachricht
func (a *Agent) processText(ctx context.Context, msg channels.Message, session *Session) string {
	text := msg.Text

	// ── BILD-FLOW (ausstehende Anfrage – Provider- oder Format-Auswahl) ─────
	if session.ImageRequest != nil {
		return a.handleImageRequestStep(ctx, msg, session, text)
	}

	// ── GEDÄCHTNIS-LÖSCH-FLOW ──────────────────────────────────────────────
	if session.ForgetState != nil {
		return a.handleForgetResponse(session, text)
	}

	if a.isForgetCommand(text) {
		return a.handleForgetCommand(session, text)
	}

	// ── NEUES GESPRÄCH ─────────────────────────────────────────────────────
	if a.isNewConversationCommand(text) {
		session.ClearHistory()
		log.Printf("[Agent] Gesprächsverlauf zurückgesetzt für %s", msg.SenderID)
		return "🔄 Neues Gespräch gestartet. Ich habe den bisherigen Verlauf gelöscht, meine Erinnerungen über dich behalte ich aber."
	}

	// ── BILD-GENERIERUNG ─────────────────────────────────────────────────────
	if a.isImageRequest(text) {
		if len(a.imageGenerators) == 0 {
			return "🎨 Bildgenerierung ist aktuell nicht aktiviert.\n\n" +
				"Du kannst das im Dashboard unter dem Tab *Bilder* ändern – " +
				"wähle dort einen Provider aus und trage deinen API-Key ein.\n\n" +
				"💡 Empfehlung: OpenRouter gibt dir Zugang zu FLUX.2 Pro, Seedream und vielen weiteren Modellen.\n" +
				"→ openrouter.ai"
		}
		return a.handleImageRequest(ctx, msg, session, text)
	}

	// ── VIDEO-GENERIERUNG ─────────────────────────────────────────────────────
	if a.isVideoRequest(text) {
		if a.videoDefault == "" || a.videoDefault == "disabled" {
			return "🎬 Videogenerierung ist aktuell nicht aktiviert.\n\n" +
				"Du kannst das im Dashboard unter dem Tab *Videos* ändern – " +
				"wähle dort einen Provider aus und trage deinen API-Key ein.\n\n" +
				"💡 Empfehlung: Runway Gen-4 Turbo für professionelle KI-Videos.\n" +
				"→ app.runwayml.com"
		}
		return "🎬 Videogenerierung mit *" + a.videoDefault + "* ist konfiguriert, aber noch nicht vollständig implementiert. Bitte warte auf das nächste Update."
	}

	// ── MERKEN-LOGIK ───────────────────────────────────────────────────────
	if a.isMemoryCommand(text) {
		fact := a.extractFact(text)
		if fact != "" {
			session.AddFact(fact)
			log.Printf("[Agent] Fakt gemerkt für %s: %s", msg.SenderID, fact)
			return fmt.Sprintf("✅ Gespeichert: _%s_", fact)
		}
	}

	// ── DISAMBIGUIERUNGS-FLOW ──────────────────────────────────────────────
	if session.Disambiguation != nil {
		return a.handleDisambiguationResponse(ctx, msg, session, text)
	}

	matchResult := a.skillsLoader.Match(text)

	if matchResult.NeedsDisambiguation {
		session.Disambiguation = &skills.DisambiguationState{
			OriginalText: text,
			Question:     matchResult.Question,
			Options:      matchResult.OptionSkills,
		}
		return a.buildDisambiguationQuestion(matchResult)
	}

	var rules string
	if matchResult.Skill != nil {
		rules = matchResult.Skill.Content
		log.Printf("[Agent] Skill: %s", matchResult.Skill.Name)
	} else {
		rules = matchResult.FallbackContent
	}

	return a.callAI(ctx, msg, session, text, rules)
}

// ── GEDÄCHTNIS-LÖSCHUNG ────────────────────────────────────────────────────

func (a *Agent) isForgetCommand(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "vergiss") ||
		strings.Contains(lower, "vergessen") ||
		strings.Contains(lower, "lösch") ||
		strings.Contains(lower, "streiche") ||
		strings.Contains(lower, "aus deinem gedächtnis")
}

func (a *Agent) isForgetAll(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "alles") ||
		strings.Contains(lower, "komplett") ||
		strings.Contains(lower, "gesamtes gedächtnis") ||
		strings.Contains(lower, "alles vergessen") ||
		strings.Contains(lower, "alles löschen")
}

func (a *Agent) isNewConversationCommand(text string) bool {
	lower := strings.ToLower(text)
	return lower == "neues gespräch" ||
		lower == "reset" ||
		lower == "neu starten" ||
		strings.Contains(lower, "starte neu") ||
		strings.Contains(lower, "neues gespräch starten") ||
		strings.Contains(lower, "verlauf löschen") ||
		strings.Contains(lower, "verlauf zurücksetzen")
}

func (a *Agent) extractForgetKeyword(text string) string {
	lower := strings.ToLower(text)
	prefixes := []string{
		"vergiss das mit", "vergiss", "lösche aus deinem gedächtnis",
		"lösch aus deinem gedächtnis", "streiche", "lösch", "lösche",
	}
	for _, prefix := range prefixes {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			keyword := strings.TrimSpace(text[idx+len(prefix):])
			keyword = strings.TrimRight(keyword, ".!?,")
			if keyword != "" {
				return keyword
			}
		}
	}
	return ""
}

func (a *Agent) handleForgetCommand(session *Session, text string) string {
	if a.isForgetAll(text) {
		count := len(session.Memory.UserFacts)
		if count == 0 {
			return "🧠 Mein Gedächtnis ist bereits leer."
		}
		session.ClearAllFacts()
		log.Printf("[Agent] Gedächtnis gelöscht für %s: alle %d Fakten", session.UserID, count)
		return fmt.Sprintf("🗑️ Erledigt! Ich habe mein gesamtes Gedächtnis über dich gelöscht (%d Einträge).", count)
	}

	if len(session.Memory.UserFacts) == 0 {
		return "🧠 Mein Gedächtnis ist bereits leer – nichts zu löschen."
	}

	keyword := a.extractForgetKeyword(text)
	if keyword != "" {
		matches := session.FindFactsByKeyword(keyword)

		if len(matches) == 0 {
			return fmt.Sprintf("❓ Ich konnte keinen Eintrag mit \"%s\" finden.\n\n%s", keyword, session.ListFacts())
		}

		if len(matches) == 1 {
			deleted := session.Memory.UserFacts[matches[0]]
			session.DeleteFactAt(matches[0])
			log.Printf("[Agent] Fakt gelöscht für %s: %s", session.UserID, deleted)
			return fmt.Sprintf("✅ Gelöscht: _%s_", deleted)
		}

		session.ForgetState = &ForgetState{Options: matches}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("🔍 Ich habe %d Einträge mit \"%s\" gefunden. Welchen soll ich löschen?\n\n", len(matches), keyword))
		for i, idx := range matches {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, session.Memory.UserFacts[idx]))
		}
		sb.WriteString("\nBitte antworte mit der Nummer, oder schreibe \"alle\" um alle zu löschen.")
		return strings.TrimRight(sb.String(), "\n")
	}

	session.ForgetState = &ForgetState{Options: makeRange(len(session.Memory.UserFacts))}
	return fmt.Sprintf("%s\n\nWelchen Eintrag soll ich vergessen? Antworte mit der Nummer, oder schreibe \"alle\".", session.ListFacts())
}

func (a *Agent) handleForgetResponse(session *Session, text string) string {
	state := session.ForgetState
	session.ForgetState = nil

	lower := strings.ToLower(strings.TrimSpace(text))

	if lower == "alle" || lower == "alles" {
		deleted := make([]string, 0, len(state.Options))
		for _, idx := range state.Options {
			if idx < len(session.Memory.UserFacts) {
				deleted = append(deleted, session.Memory.UserFacts[idx])
			}
		}
		for i := len(state.Options) - 1; i >= 0; i-- {
			idx := state.Options[i]
			if idx >= 0 && idx < len(session.Memory.UserFacts) {
				session.Memory.UserFacts = append(session.Memory.UserFacts[:idx], session.Memory.UserFacts[idx+1:]...)
			}
		}
		session.SaveMemory()
		log.Printf("[Agent] Fakten gelöscht für %s: %v", session.UserID, deleted)
		return fmt.Sprintf("🗑️ Alle %d markierten Einträge gelöscht.", len(deleted))
	}

	if lower == "nein" || lower == "abbruch" || lower == "cancel" || lower == "stop" {
		return "✅ Alles klar, nichts gelöscht."
	}

	num, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || num < 1 || num > len(state.Options) {
		return fmt.Sprintf("❓ Bitte gib eine Zahl zwischen 1 und %d ein, oder \"alle\" / \"abbruch\".", len(state.Options))
	}

	targetIdx := state.Options[num-1]
	if targetIdx >= len(session.Memory.UserFacts) {
		return "❌ Dieser Eintrag existiert nicht mehr."
	}

	deleted := session.Memory.UserFacts[targetIdx]
	session.DeleteFactAt(targetIdx)
	log.Printf("[Agent] Fakt gelöscht für %s: %s", session.UserID, deleted)
	return fmt.Sprintf("✅ Gelöscht: _%s_", deleted)
}

func makeRange(n int) []int {
	r := make([]int, n)
	for i := range r {
		r[i] = i
	}
	return r
}

// ── DISAMBIGUIERUNG ────────────────────────────────────────────────────────

func (a *Agent) handleDisambiguationResponse(ctx context.Context, msg channels.Message, session *Session, response string) string {
	state := session.Disambiguation

	resolveResult := a.skillsLoader.ResolveDisambiguation(state, response)

	if resolveResult.NeedsDisambiguation {
		session.Disambiguation = &skills.DisambiguationState{
			OriginalText: state.OriginalText,
			Question:     resolveResult.Question,
			Options:      resolveResult.OptionSkills,
		}
		return a.buildDisambiguationQuestion(resolveResult)
	}

	session.Disambiguation = nil

	var rules string
	if resolveResult.Skill != nil {
		rules = resolveResult.Skill.Content
		log.Printf("[Agent] Disambiguierung aufgelöst → Skill: %s", resolveResult.Skill.Name)
	}

	combinedText := state.OriginalText + " " + response
	return a.callAI(ctx, msg, session, combinedText, rules)
}

func (a *Agent) buildDisambiguationQuestion(result *skills.MatchResult) string {
	if len(result.Options) == 0 {
		return result.Question
	}
	options := strings.Join(result.Options, " / ")
	return fmt.Sprintf("❓ %s\n(%s)", result.Question, options)
}

// ── KI-AUFRUF ──────────────────────────────────────────────────────────────

func (a *Agent) callAI(ctx context.Context, msg channels.Message, session *Session, text, rules string) string {
	modelID, modelName := provider.RouteModel(text, a.models)
	log.Printf("[Agent] Nachricht: %.50s... | Modell: %s | User: %s | History: %d Turns",
		text, modelName, msg.SenderID, len(session.Memory.History))

	systemPrompt := a.systemPromptFn(session, rules)

	// Gesprächsverlauf + aktuelle Nachricht zusammenbauen
	messages := make([]provider.Message, 0, len(session.Memory.History)+1)
	for _, turn := range session.Memory.History {
		messages = append(messages, provider.Message{Role: turn.Role, Content: turn.Content})
	}
	messages = append(messages, provider.Message{Role: "user", Content: text})

	req := provider.Request{
		Model:    modelID,
		System:   systemPrompt,
		Messages: messages,
	}

	response, err := a.provider.Complete(ctx, req)
	if err != nil {
		log.Printf("[Agent] Provider-Fehler: %v", err)
		return fmt.Sprintf("❌ Fehler bei der KI-Anfrage: %v", err)
	}
	return response
}

func (a *Agent) buildSystemPrompt(session *Session, rules string) string {
	facts := session.FactsSummary()

	var prompt string
	if a.soul != "" {
		// Persönlichkeit aus SOUL.md / IDENTITY.md
		prompt = a.soul + "\n\n"
	} else {
		// Fallback: Basis-Identität
		prompt = "Du bist Fluxy, ein KI-Assistent von KI-WERKE.\n\n"
	}

	prompt += "ANTWORTREGELN – STRIKTE PFLICHT:\n" +
		"- Beantworte NUR was gefragt wurde. Nichts mehr.\n" +
		"- Maximal 2-3 Sätze, außer der Nutzer fragt EXPLIZIT nach einer ausführlichen Antwort.\n" +
		"- KEINE ungebetenen Zusatzinfos, Tipps, Links, Vorschläge oder Erklärungen.\n" +
		"- KEINE Einleitungsphrasen wie \"Natürlich!\", \"Gerne!\", \"Sicher!\", \"Selbstverständlich!\".\n" +
		"- Bei einfachen Bestätigungen oder Statusmeldungen: eine Zeile reicht.\n"

	if facts != "" {
		prompt += fmt.Sprintf("\nGEDÄCHTNIS ÜBER DEN NUTZER: %s\n", facts)
	}
	if rules != "" {
		prompt += fmt.Sprintf("\nAUFGABE/KONTEXT:\n%s\n", rules)
	}
	return prompt
}

// ── GEDÄCHTNIS-MERKEN ──────────────────────────────────────────────────────

func (a *Agent) isMemoryCommand(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))

	// Fragen explizit ausschließen – "was hast du dir gemerkt?" darf NICHT triggern
	questionStarters := []string{"was ", "wie ", "wann ", "wo ", "wer ", "hast ", "kannst ", "weißt ", "zeig ", "welche ", "welchen "}
	for _, q := range questionStarters {
		if strings.HasPrefix(lower, q) {
			return false
		}
	}

	// Nur klare Imperativ-Formen triggern
	return strings.Contains(lower, "merke dir") ||
		strings.Contains(lower, "merk dir") ||
		strings.Contains(lower, "nicht vergessen:") ||
		strings.Contains(lower, "bitte merke dir") ||
		strings.Contains(lower, "bitte merk dir")
}

func (a *Agent) extractFact(text string) string {
	lower := strings.ToLower(text)
	prefixes := []string{
		"bitte merke dir", "bitte merk dir",
		"merke dir", "merk dir",
		"nicht vergessen:",
	}
	for _, prefix := range prefixes {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			fact := strings.TrimSpace(text[idx+len(prefix):])
			fact = strings.TrimLeft(fact, ":,. ")
			if fact != "" {
				return fact
			}
		}
	}
	// Kein Fallback auf den kompletten Text – lieber nichts speichern als Unsinn
	return ""
}

// ── BILD-GENERIERUNG ───────────────────────────────────────────────────────

// isImageRequest erkennt Anfragen zur Bildgenerierung
func (a *Agent) isImageRequest(text string) bool {
	lower := strings.ToLower(text)
	hasTrigger := strings.Contains(lower, "generiere") ||
		strings.Contains(lower, "erstelle ein bild") ||
		strings.Contains(lower, "male") ||
		strings.Contains(lower, "zeichne") ||
		strings.Contains(lower, "mach ein bild") ||
		strings.Contains(lower, "mach mir ein bild") ||
		strings.Contains(lower, "create an image") ||
		strings.Contains(lower, "generate an image") ||
		strings.Contains(lower, "generate image") ||
		strings.Contains(lower, "ein foto von") ||
		strings.Contains(lower, "ein bild von")
	hasBild := strings.Contains(lower, "bild") ||
		strings.Contains(lower, "foto") ||
		strings.Contains(lower, "image") ||
		strings.Contains(lower, "illustration") ||
		strings.Contains(lower, "artwork")
	return hasTrigger || (hasBild && (strings.Contains(lower, "von ") || strings.Contains(lower, "zeig")))
}

// isVideoRequest erkennt Videogenerierungs-Anfragen.
// Prüft ob "video"/"film" + ein Aktionsverb vorhanden ist.
func (a *Agent) isVideoRequest(text string) bool {
	lower := strings.ToLower(text)
	hasVideo := strings.Contains(lower, "video") || strings.Contains(lower, "film")
	if !hasVideo {
		return false
	}
	return strings.Contains(lower, "erstell") ||
		strings.Contains(lower, "generier") ||
		strings.Contains(lower, "mach") ||
		strings.Contains(lower, "produzier") ||
		strings.Contains(lower, "create") ||
		strings.Contains(lower, "generate") ||
		strings.Contains(lower, "render")
}

// extractImagePrompt extrahiert den Bildprompt aus dem Text
func (a *Agent) extractImagePrompt(text string) string {
	lower := strings.ToLower(text)
	prefixes := []string{
		"erstelle ein bild von", "erstelle ein foto von", "erstelle ein bild:",
		"generiere ein bild von", "generiere ein foto von", "generiere ein bild:",
		"generiere", "erstelle", "male ein bild von", "male",
		"zeichne ein bild von", "zeichne",
		"mach ein bild von", "mach mir ein bild von",
		"ein bild von", "ein foto von",
		"create an image of", "generate an image of",
		"generate an image:", "create an image:",
	}
	for _, prefix := range prefixes {
		if idx := strings.Index(lower, prefix); idx >= 0 {
			prompt := strings.TrimSpace(text[idx+len(prefix):])
			prompt = strings.TrimLeft(prompt, ":,. ")
			if len(prompt) > 10 {
				return prompt
			}
		}
	}
	return text
}

// detectImageFormat erkennt ein gewünschtes Format im Prompt.
// Gibt "landscape", "portrait", "square" oder "" zurück.
func detectImageFormat(text string) string {
	lower := strings.ToLower(text)
	for _, kw := range []string{"hochformat", "portrait", "9:16", "vertikal", "hoch", "9/16"} {
		if strings.Contains(lower, kw) {
			return "portrait"
		}
	}
	for _, kw := range []string{"querformat", "landscape", "16:9", "horizontal", "quer", "breit", "16/9"} {
		if strings.Contains(lower, kw) {
			return "landscape"
		}
	}
	for _, kw := range []string{"quadrat", "square", "1:1", "quadratisch", "1/1"} {
		if strings.Contains(lower, kw) {
			return "square"
		}
	}
	return ""
}

// handleImageRequest verarbeitet neue Bild-Generierungs-Anfragen.
func (a *Agent) handleImageRequest(ctx context.Context, msg channels.Message, session *Session, text string) string {
	prompt := a.extractImagePrompt(text)
	format := detectImageFormat(text)

	state := &ImageRequestState{Prompt: prompt, Format: format, GeneratorIdx: -1}

	// Schritt 1: Provider wählen (wenn mehrere vorhanden)
	if len(a.imageGenerators) > 1 {
		state.Step = "provider"
		session.ImageRequest = state
		log.Printf("[Agent] Bild-Anfrage | %d Provider zur Auswahl | Format erkannt: %q | Prompt: %.80s", len(a.imageGenerators), format, prompt)
		return a.askForProvider()
	}

	// Nur ein Provider → direkt zu Format
	state.GeneratorIdx = 0
	if format != "" {
		session.ImageRequest = nil
		return a.generateImage(ctx, msg, a.imageGenerators[0], prompt, format)
	}
	state.Step = "format"
	session.ImageRequest = state
	return a.askForFormat()
}

// handleImageRequestStep leitet eine Nutzerantwort an den richtigen Handler weiter.
func (a *Agent) handleImageRequestStep(ctx context.Context, msg channels.Message, session *Session, text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "abbruch" || lower == "cancel" || lower == "stop" {
		session.ImageRequest = nil
		return "✅ Bildgenerierung abgebrochen."
	}
	switch session.ImageRequest.Step {
	case "provider":
		return a.handleProviderChoice(ctx, msg, session, text)
	case "format":
		return a.handleFormatChoice(ctx, msg, session, text)
	}
	return ""
}

// handleProviderChoice verarbeitet die Antwort auf die Provider-Frage.
func (a *Agent) handleProviderChoice(ctx context.Context, msg channels.Message, session *Session, text string) string {
	state := session.ImageRequest
	lower := strings.ToLower(strings.TrimSpace(text))

	// Auswahl per Nummer
	num, err := strconv.Atoi(strings.TrimSpace(text))
	if err == nil && num >= 1 && num <= len(a.imageGenerators) {
		state.GeneratorIdx = num - 1
	} else {
		// Auswahl per Name (Teilstring-Match)
		for i, gen := range a.imageGenerators {
			if strings.Contains(strings.ToLower(gen.Name()), lower) {
				state.GeneratorIdx = i
				break
			}
		}
	}

	if state.GeneratorIdx < 0 {
		return fmt.Sprintf("❓ Bitte wähle eine Zahl zwischen 1 und %d – oder schreibe \"abbruch\".", len(a.imageGenerators))
	}

	// Provider gewählt – Format bekannt?
	if state.Format != "" {
		session.ImageRequest = nil
		return a.generateImage(ctx, msg, a.imageGenerators[state.GeneratorIdx], state.Prompt, state.Format)
	}
	state.Step = "format"
	return a.askForFormat()
}

// handleFormatChoice verarbeitet die Antwort auf die Format-Frage.
func (a *Agent) handleFormatChoice(ctx context.Context, msg channels.Message, session *Session, text string) string {
	state := session.ImageRequest
	lower := strings.ToLower(strings.TrimSpace(text))

	format := ""
	switch {
	case lower == "1" || strings.Contains(lower, "quer") || strings.Contains(lower, "landscape") || strings.Contains(lower, "16:9"):
		format = "landscape"
	case lower == "2" || strings.Contains(lower, "hoch") || strings.Contains(lower, "portrait") || strings.Contains(lower, "9:16"):
		format = "portrait"
	case lower == "3" || strings.Contains(lower, "quadrat") || strings.Contains(lower, "square") || strings.Contains(lower, "1:1"):
		format = "square"
	}

	if format == "" {
		return "❓ Bitte wähle:\n1️⃣  Querformat (16:9)\n2️⃣  Hochformat (9:16)\n3️⃣  Quadrat (1:1)\n\nOder schreibe \"abbruch\"."
	}

	session.ImageRequest = nil
	return a.generateImage(ctx, msg, a.imageGenerators[state.GeneratorIdx], state.Prompt, format)
}

// askForProvider gibt die Provider-Auswahl-Nachricht zurück.
func (a *Agent) askForProvider() string {
	var sb strings.Builder
	sb.WriteString("🎨 Womit soll ich das Bild generieren?\n\n")
	emojis := []string{"1️⃣", "2️⃣", "3️⃣", "4️⃣", "5️⃣"}
	for i, gen := range a.imageGenerators {
		if i < len(emojis) {
			sb.WriteString(fmt.Sprintf("%s  %s\n", emojis[i], gen.Name()))
		}
	}
	sb.WriteString("\n(Nummer oder Name – oder \"abbruch\")")
	return sb.String()
}

// askForFormat gibt die Format-Auswahl-Nachricht zurück.
func (a *Agent) askForFormat() string {
	return "📐 In welchem Format soll das Bild sein?\n\n" +
		"1️⃣  Querformat (16:9)\n" +
		"2️⃣  Hochformat (9:16)\n" +
		"3️⃣  Quadrat (1:1)\n\n" +
		"(Nummer oder Name – oder \"abbruch\")"
}

// generateImage generiert ein Bild und sendet es direkt.
// Gibt "" zurück wenn das Bild erfolgreich direkt gesendet wurde.
func (a *Agent) generateImage(ctx context.Context, msg channels.Message, gen imagegen.Generator, prompt, format string) string {
	if format == "" {
		format = a.imageSize // Fallback auf config-Default
	}
	log.Printf("[Agent] 🎨 Bild-Generierung | Provider: %s | Format: %s | Prompt: %.100s", gen.Name(), format, prompt)
	a.manager.Typing(msg)

	img, err := gen.Generate(ctx, prompt, format)
	if err != nil {
		log.Printf("[Agent] ❌ Bild-Generierung fehlgeschlagen | Provider: %s | Fehler: %v", gen.Name(), err)
		return fmt.Sprintf("❌ Bild konnte nicht generiert werden: %v", err)
	}

	log.Printf("[Agent] ✅ Bild-URL erhalten | Provider: %s | URL: %s", gen.Name(), img.URL)
	caption := fmt.Sprintf("🎨 _%s_", prompt)
	if err := a.manager.ReplyPhoto(msg, img.URL, caption); err != nil {
		log.Printf("[Agent] ⚠️  Bild-Senden fehlgeschlagen (sende URL stattdessen) | Fehler: %v", err)
		return fmt.Sprintf("🎨 Bild generiert: %s", img.URL)
	}

	log.Printf("[Agent] ✅ Bild erfolgreich gesendet | Provider: %s", gen.Name())
	return ""
}
