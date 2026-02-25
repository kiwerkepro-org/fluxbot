package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ki-werke/fluxbot/pkg/channels"
	cronpkg "github.com/ki-werke/fluxbot/pkg/cron"
	"github.com/ki-werke/fluxbot/pkg/email"
	googleapi "github.com/ki-werke/fluxbot/pkg/google"
	"github.com/ki-werke/fluxbot/pkg/imagegen"
	"github.com/ki-werke/fluxbot/pkg/provider"
	"github.com/ki-werke/fluxbot/pkg/security"
	"github.com/ki-werke/fluxbot/pkg/skills"
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
	imageSize        string
	videoDefault     string        // "disabled" oder Provider-Name – steuert Video-Meldung
	emailSender      *email.Sender // nil = E-Mail deaktiviert
	calcomBaseURL     string // Cal.com / Cal.eu API-Basis-URL
	calcomAPIKey      string // Cal.com API-Key
	calcomOwnerEmail  string // Standard-E-Mail für Buchungen
	calcomEventTypeID int    // Optional: fixe Event Type ID (0 = auto-fetch)
	googleClient     *googleapi.Client // nil = Google deaktiviert
	soul              string // Inhalt von SOUL.md + IDENTITY.md (Persönlichkeit)
	cronManager      *cronpkg.Manager // nil = Cron deaktiviert
	systemPromptFn   func(session *Session, rules string) string
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
	VideoDefault     string        // "disabled" oder Provider-Name – steuert Video-Meldung
	EmailSender      *email.Sender // Optional – nil = E-Mail deaktiviert
	CalcomBaseURL     string // Optional – Cal.com / Cal.eu API-Basis-URL
	CalcomAPIKey      string // Optional – Cal.com API-Key
	CalcomOwnerEmail  string // Optional – Standard-E-Mail für Buchungen
	CalcomEventTypeID int    // Optional – fixe Event Type ID (0 = auto-fetch)
	GoogleClient     *googleapi.Client // Optional – nil = Google deaktiviert
	CronManager      *cronpkg.Manager // Optional – nil = Cron deaktiviert
	Soul              string // Inhalt von SOUL.md + IDENTITY.md (leer = nur Basis-Prompt)
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
		emailSender:      cfg.EmailSender,
		calcomBaseURL:     cfg.CalcomBaseURL,
		calcomAPIKey:      cfg.CalcomAPIKey,
		calcomOwnerEmail:  cfg.CalcomOwnerEmail,
		calcomEventTypeID: cfg.CalcomEventTypeID,
		googleClient:     cfg.GoogleClient,
		cronManager:      cfg.CronManager,
		soul:             cfg.Soul,
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

// UpdateEmailSender ersetzt den SMTP-Sender zur Laufzeit.
// Wird aufgerufen wenn die Config im Dashboard geändert wird.
func (a *Agent) UpdateEmailSender(sender *email.Sender) {
	a.emailSender = sender
	if sender == nil || !sender.IsConfigured() {
		log.Println("[Agent] 🔄 E-Mail-Versand deaktiviert (keine SMTP-Credentials).")
	} else {
		log.Printf("[Agent] 🔄 E-Mail-Sender aktiv (Von: %s)", sender.From())
	}
}

// UpdateCalcomConfig aktualisiert die Cal.com-Konfiguration zur Laufzeit.
// Wird aufgerufen wenn die Config im Dashboard geändert wird (Hot-Reload).
func (a *Agent) UpdateCalcomConfig(baseURL, apiKey, ownerEmail string, eventTypeID int) {
	a.calcomBaseURL = baseURL
	a.calcomAPIKey = apiKey
	a.calcomOwnerEmail = ownerEmail
	a.calcomEventTypeID = eventTypeID
	if baseURL == "" || apiKey == "" {
		log.Println("[Agent] 🔄 Cal.com deaktiviert (keine Zugangsdaten).")
	} else {
		if eventTypeID > 0 {
			log.Printf("[Agent] 🔄 Cal.com aktiv (%s | EventTypeID: %d)", baseURL, eventTypeID)
		} else {
			log.Printf("[Agent] 🔄 Cal.com aktiv (%s | EventTypeID: auto)", baseURL)
		}
	}
}

// UpdateGoogleClient ersetzt den Google API-Client zur Laufzeit (Hot-Reload).
func (a *Agent) UpdateGoogleClient(client *googleapi.Client) {
	a.googleClient = client
	if client == nil || !client.IsConfigured() {
		log.Println("[Agent] 🔄 Google API deaktiviert (keine Zugangsdaten).")
	} else {
		log.Println("[Agent] 🔄 Google API aktiviert (Calendar, Docs, Sheets, Drive, Gmail).")
	}
}

// UpdateCronManager setzt den CronManager zur Laufzeit (Hot-Reload-faehig).
func (a *Agent) UpdateCronManager(m *cronpkg.Manager) {
	a.cronManager = m
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
	case channels.MessageTypeImage:
		a.handleImageAnalysis(ctx, msg, session)
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
	if a.transcriber == nil {
		a.manager.Reply(msg, "🎙️ Spracherkennung ist nicht aktiviert.\nFüge in config.json hinzu:\n\"voice\": {\"enabled\": true, \"provider\": \"groq\", \"apiKey\": \"DEIN_GROQ_KEY\"}")
		return
	}

	var mediaPath string

	// Fall 1: Sprachdaten kommen aus dem RAM (vom Security-Scanner durchgereicht)
	if len(msg.VoiceData) > 0 {
		tmpFile, err := os.CreateTemp("", "voice-*.ogg")
		if err != nil {
			log.Printf("[Agent] Fehler beim Erstellen der Temp-Datei: %v", err)
			a.manager.Reply(msg, "❌ Interner Fehler bei der Sprachverarbeitung.")
			return
		}
		if _, err := tmpFile.Write(msg.VoiceData); err != nil {
			tmpFile.Close()
			log.Printf("[Agent] Fehler beim Schreiben der Temp-Datei: %v", err)
			return
		}
		tmpFile.Close()
		mediaPath = tmpFile.Name()
		defer os.Remove(mediaPath) // Sofort nach der Transkription restlos löschen
	} else if msg.MediaPath != "" {
		// Fall 2: Klassischer Datei-Download (Fallback)
		mediaPath = msg.MediaPath
		defer os.Remove(mediaPath)
	} else {
		// Weder RAM-Daten noch lokaler Pfad vorhanden
		a.manager.Reply(msg, "❌ Sprachnachricht konnte nicht geladen werden.")
		return
	}

	log.Printf("[Agent] Transkribiere | Provider: %s", a.transcriber.Name())
	text, err := a.transcriber.Transcribe(ctx, mediaPath, a.voiceLang)
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

// handleImageAnalysis analysiert ein empfangenes Foto via Vision-AI.
// Das Bild wird aus msg.MediaPath geladen, base64-kodiert und an den AI-Provider gesendet.
// msg.Text enthaelt optional eine Caption des Users (z.B. "Was siehst du hier?").
func (a *Agent) handleImageAnalysis(ctx context.Context, msg channels.Message, session *Session) {
	if msg.MediaPath == "" {
		a.manager.Reply(msg, "\u274c Foto konnte nicht geladen werden.")
		return
	}
	defer os.Remove(msg.MediaPath) // Temp-Datei nach Verarbeitung loeschen

	// Bild aus Temp-Datei lesen
	imageData, err := os.ReadFile(msg.MediaPath)
	if err != nil {
		log.Printf("[Agent] Fehler beim Lesen des Fotos: %v", err)
		a.manager.Reply(msg, "\u274c Foto konnte nicht gelesen werden.")
		return
	}

	// MIME-Typ anhand der Dateiendung ermitteln
	imageMIME := "image/jpeg"
	path := msg.MediaPath
	if len(path) >= 4 {
		ext := strings.ToLower(path[len(path)-4:])
		switch ext {
		case ".png":
			imageMIME = "image/png"
		case ".gif":
			imageMIME = "image/gif"
		case ".webp":
			imageMIME = "image/webp"
		}
	}

	// Nutzer-Prompt: Caption nutzen, sonst Default-Frage
	userPrompt := msg.Text
	if strings.TrimSpace(userPrompt) == "" {
		userPrompt = "Was siehst du auf diesem Bild? Beschreibe den Inhalt kurz und praezise."
	}

	// Schreib-Indikator
	a.manager.Typing(msg)

	// Vision-Modell waehlen (ocr-Key = Pixtral / GPT-4o / Claude Vision je nach Provider)
	visionModel := a.models["ocr"]
	if visionModel == "" {
		visionModel = a.models["default"]
	}

	log.Printf("[Agent] Bildanalyse | Modell: %s | MIME: %s | Groesse: %d bytes | Prompt: %.80s",
		visionModel, imageMIME, len(imageData), userPrompt)

	req := provider.Request{
		Model:  visionModel,
		System: a.buildSystemPrompt(session, ""),
		Messages: []provider.Message{
			{
				Role:      "user",
				Content:   userPrompt,
				ImageData: imageData,
				ImageMIME: imageMIME,
			},
		},
	}

	response, err := a.provider.Complete(ctx, req)
	if err != nil {
		log.Printf("[Agent] Bildanalyse-Fehler: %v", err)
		a.manager.Reply(msg, fmt.Sprintf("\u274c Bildanalyse fehlgeschlagen: %v", err))
		return
	}

	response = strings.TrimSpace(response)
	// Gespraechsverlauf speichern
	session.AddToHistory("user", "[Foto] "+userPrompt)
	session.AddToHistory("assistant", response)
	a.manager.Reply(msg, response)
}

// processText verarbeitet eine Textnachricht
func (a *Agent) processText(ctx context.Context, msg channels.Message, session *Session) string {
	text := msg.Text

	// ── BILD-FLOW (ausstehende Anfrage – Provider- oder Format-Auswahl) ─────
	if session.ImageRequest != nil {
		return a.handleImageRequestStep(ctx, msg, session, text)
	}

	// ── E-MAIL-BESTÄTIGUNGS-FLOW ───────────────────────────────────────────
	if session.EmailState != nil {
		return a.handleEmailConfirmation(ctx, msg, session, text)
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
	// Reminder-Befehle explizit ausschliessen (werden vom CronManager verarbeitet)
	if strings.Contains(lower, "erinnerung") || strings.Contains(lower, "reminder") {
		return false
	}
	return strings.Contains(lower, "vergiss") ||
		strings.Contains(lower, "vergessen") ||
		strings.Contains(lower, "lösch") ||
		strings.Contains(lower, "streiche") ||
		strings.Contains(lower, "entferne") ||
		strings.Contains(lower, "entfernen") ||
		strings.Contains(lower, "delete") ||
		strings.Contains(lower, "remove") ||
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
		// Längere Phrasen zuerst – damit "lösche aus deinem gedächtnis"
		// nicht als "lösche" + Rest geparst wird
		"vergiss das mit",
		"vergiss alles über",
		"vergiss",
		"lösche aus deinem gedächtnis",
		"lösch aus deinem gedächtnis",
		"entferne aus deinem gedächtnis",
		"delete aus deinem gedächtnis",
		"remove aus deinem gedächtnis",
		"streiche",
		// Kurze Varianten ganz am Ende
		"lösche",
		"lösch",
		"entferne",
		"entfernen",
		"delete",
		"remove",
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

	// KI hat Video-Anfrage erkannt → passende Meldung zurückgeben
	if strings.Contains(response, "__VIDEO_REQUEST__") {
		log.Printf("[Agent] 🎬 Video-Anfrage erkannt (KI-Klassifizierung)")
		if a.videoDefault == "" || a.videoDefault == "disabled" {
			return "🎬 Videogenerierung ist aktuell nicht aktiviert.\n\n" +
				"Du kannst das im Dashboard unter dem Tab *Videos* ändern – " +
				"wähle dort einen Provider aus und trage deinen API-Key ein.\n\n" +
				"💡 Empfehlung: Runway Gen-4 Turbo für professionelle KI-Videos.\n" +
				"→ app.runwayml.com"
		}
		return "🎬 Videogenerierung mit *" + a.videoDefault + "* ist konfiguriert, aber noch nicht vollständig implementiert."
	}

	// KI hat Skill-Erstellung erkannt → speichern + signieren
	if strings.Contains(response, "__SKILL_NAME:") && strings.Contains(response, "__SKILL_END__") {
		return a.handleSkillCreation(response)
	}

	// KI hat E-Mail-Anfrage erkannt → Vorschau erstellen + Bestätigung einholen
	if strings.Contains(response, "__SEND_EMAIL__") && strings.Contains(response, "__EMAIL_END__") {
		return a.handleEmailRequest(session, response)
	}

	// KI hat Cal.com-Buchungsanfrage erkannt → echten HTTP-Call ausführen
	if strings.Contains(response, "__CALCOM_BOOK__") && strings.Contains(response, "__CALCOM_BOOK_END__") {
		return a.handleCalcomBooking(response)
	}

	// KI hat Cal.com-Listenabfrage erkannt → echte Buchungen laden
	if strings.Contains(response, "__CALCOM_LIST__") {
		return a.handleCalcomList()
	}

	// ── Google API Marker ─────────────────────────────────────────────────────

	// Google Calendar – Termin erstellen
	if strings.Contains(response, "__GOOGLE_CAL_CREATE__") && strings.Contains(response, "__GOOGLE_CAL_CREATE_END__") {
		return a.handleGoogleCalCreate(response)
	}
	// Google Calendar – Termine auflisten
	if strings.Contains(response, "__GOOGLE_CAL_LIST__") {
		return a.handleGoogleCalList()
	}
	// Google Docs – neues Dokument erstellen
	if strings.Contains(response, "__GOOGLE_DOCS_CREATE__") && strings.Contains(response, "__GOOGLE_DOCS_CREATE_END__") {
		return a.handleGoogleDocsCreate(response)
	}
	// Google Docs – Text an Dokument anhängen
	if strings.Contains(response, "__GOOGLE_DOCS_APPEND__") && strings.Contains(response, "__GOOGLE_DOCS_APPEND_END__") {
		return a.handleGoogleDocsAppend(response)
	}
	// Google Docs – Dokument lesen
	if strings.Contains(response, "__GOOGLE_DOCS_READ__") && strings.Contains(response, "__GOOGLE_DOCS_READ_END__") {
		return a.handleGoogleDocsRead(response)
	}
	// Google Sheets – Tabelle erstellen
	if strings.Contains(response, "__GOOGLE_SHEETS_CREATE__") && strings.Contains(response, "__GOOGLE_SHEETS_CREATE_END__") {
		return a.handleGoogleSheetsCreate(response)
	}
	// Google Sheets – Werte lesen
	if strings.Contains(response, "__GOOGLE_SHEETS_READ__") && strings.Contains(response, "__GOOGLE_SHEETS_READ_END__") {
		return a.handleGoogleSheetsRead(response)
	}
	// Google Sheets – Werte schreiben
	if strings.Contains(response, "__GOOGLE_SHEETS_WRITE__") && strings.Contains(response, "__GOOGLE_SHEETS_WRITE_END__") {
		return a.handleGoogleSheetsWrite(response)
	}
	// Google Drive – Dateien auflisten/suchen
	if strings.Contains(response, "__GOOGLE_DRIVE_LIST__") && strings.Contains(response, "__GOOGLE_DRIVE_LIST_END__") {
		return a.handleGoogleDriveList(response)
	}
	// Gmail – E-Mail senden
	if strings.Contains(response, "__GMAIL_SEND__") && strings.Contains(response, "__GMAIL_SEND_END__") {
		return a.handleGmailSend(response)
	}
	// Gmail – E-Mails auflisten
	if strings.Contains(response, "__GMAIL_LIST__") && strings.Contains(response, "__GMAIL_LIST_END__") {
		return a.handleGmailList(response)
	}

	// ── Cron-Reminder Marker ──────────────────────────────────────────────────

	// Reminder anlegen
	if strings.Contains(response, "__REMINDER_CREATE__") && strings.Contains(response, "__REMINDER_CREATE_END__") {
		return a.handleReminderCreate(msg, response)
	}
	// Reminder auflisten
	if strings.Contains(response, "__REMINDER_LIST__") {
		return a.handleReminderList(msg)
	}
	// Reminder löschen
	if strings.Contains(response, "__REMINDER_DELETE__") && strings.Contains(response, "__REMINDER_DELETE_END__") {
		return a.handleReminderDelete(msg, response)
	}

	return response
}

// handleSkillCreation parst den Skill aus der KI-Antwort, speichert und signiert ihn.
func (a *Agent) handleSkillCreation(response string) string {
	// Format: __SKILL_NAME:dateiname__\n<inhalt>\n__SKILL_END__
	nameStart := strings.Index(response, "__SKILL_NAME:") + len("__SKILL_NAME:")
	nameEnd := strings.Index(response[nameStart:], "__")
	if nameEnd < 0 {
		return "❌ Skill-Format fehlerhaft. Bitte nochmal versuchen."
	}
	skillName := strings.TrimSpace(response[nameStart : nameStart+nameEnd])

	contentStart := nameStart + nameEnd + len("__\n")
	contentEnd := strings.Index(response, "__SKILL_END__")
	if contentStart >= contentEnd {
		return "❌ Skill-Inhalt leer. Bitte nochmal versuchen."
	}
	skillContent := strings.TrimSpace(response[contentStart:contentEnd])

	if err := a.skillsLoader.SaveAndSign(skillName, skillContent); err != nil {
		log.Printf("[Agent] ❌ Skill-Speicherung fehlgeschlagen: %v", err)
		return fmt.Sprintf("❌ Skill konnte nicht gespeichert werden: %v", err)
	}

	log.Printf("[Agent] ✅ Skill '%s' erstellt und signiert", skillName)
	return fmt.Sprintf("✅ Skill *%s* wurde erstellt, gespeichert und mit HMAC signiert.\n\n"+
		"FluxBot kennt diesen Skill ab sofort – er wird bei passenden Anfragen automatisch aktiviert.", skillName)
}

// ── E-MAIL-VERSAND ──────────────────────────────────────────────────────────

// handleEmailRequest parst die KI-Antwort mit __SEND_EMAIL__-Marker und zeigt eine Vorschau.
// Die E-Mail wird NICHT direkt gesendet – der Nutzer muss explizit "ja" bestätigen.
func (a *Agent) handleEmailRequest(session *Session, response string) string {
	// Format:
	//   __SEND_EMAIL__
	//   TO:<empfänger>
	//   SUBJECT:<betreff>
	//   BODY:<text>
	//   __EMAIL_END__
	start := strings.Index(response, "__SEND_EMAIL__")
	end := strings.Index(response, "__EMAIL_END__")
	if start < 0 || end < 0 || end <= start {
		return "❌ E-Mail-Format fehlerhaft. Bitte nochmal versuchen."
	}

	block := strings.TrimSpace(response[start+len("__SEND_EMAIL__") : end])

	var to, subject, body string
	var bodyLines []string
	inBody := false

	for _, line := range strings.Split(block, "\n") {
		upper := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(upper, "TO:"):
			to = strings.TrimSpace(line[strings.Index(line, ":")+1:])
		case strings.HasPrefix(upper, "SUBJECT:"):
			subject = strings.TrimSpace(line[strings.Index(line, ":")+1:])
			inBody = false
		case strings.HasPrefix(upper, "BODY:"):
			rest := strings.TrimSpace(line[strings.Index(line, ":")+1:])
			bodyLines = []string{rest}
			inBody = true
		case inBody:
			bodyLines = append(bodyLines, line)
		}
	}
	body = strings.TrimSpace(strings.Join(bodyLines, "\n"))

	if to == "" || subject == "" || body == "" {
		return "❌ E-Mail unvollständig (Empfänger, Betreff oder Text fehlt). Bitte nochmal versuchen."
	}

	// Vorschau speichern – wartet auf Bestätigung
	session.EmailState = &EmailState{To: to, Subject: subject, Body: body}

	log.Printf("[Agent] 📧 E-Mail vorbereitet → An: %s | Betreff: %s", to, subject)

	return fmt.Sprintf(
		"📧 *E-Mail-Vorschau*\n\n"+
			"*An:* %s\n"+
			"*Betreff:* %s\n\n"+
			"%s\n\n"+
			"──────────────────\n"+
			"Soll ich diese E-Mail jetzt senden?\n"+
			"✅ *ja* – senden   ❌ *nein* – abbrechen",
		to, subject, body,
	)
}

// handleEmailConfirmation verarbeitet die Bestätigung oder Ablehnung durch den Nutzer.
func (a *Agent) handleEmailConfirmation(ctx context.Context, msg channels.Message, session *Session, text string) string {
	state := session.EmailState
	lower := strings.ToLower(strings.TrimSpace(text))

	// Abbrechen
	if lower == "nein" || lower == "no" || lower == "abbruch" || lower == "cancel" || lower == "stop" || lower == "abbrechen" {
		session.EmailState = nil
		log.Printf("[Agent] 📧 E-Mail abgebrochen (Nutzer: %s)", msg.SenderID)
		return "✅ E-Mail wurde abgebrochen."
	}

	// Bestätigen
	if lower == "ja" || lower == "yes" || lower == "senden" || lower == "send" || lower == "ok" || lower == "jetzt senden" {
		session.EmailState = nil

		if a.emailSender == nil || !a.emailSender.IsConfigured() {
			log.Printf("[Agent] 📧 E-Mail-Versand fehlgeschlagen: kein SMTP-Sender konfiguriert")
			return "❌ E-Mail-Versand ist nicht eingerichtet.\n\n" +
				"Trage folgende Werte im Dashboard unter *Integrationen* ein:\n" +
				"• *SMTP_HOST* – z.B. smtp.gmail.com\n" +
				"• *SMTP_PORT* – z.B. 587\n" +
				"• *SMTP_USER* – deine E-Mail-Adresse\n" +
				"• *SMTP_PASSWORD* – dein App-Passwort\n" +
				"• *SMTP_FROM* – Absender (optional, Standard = SMTP_USER)"
		}

		log.Printf("[Agent] 📧 Sende E-Mail → An: %s | Betreff: %s", state.To, state.Subject)
		if err := a.emailSender.Send(email.Message{
			To:      state.To,
			Subject: state.Subject,
			Body:    state.Body,
		}); err != nil {
			log.Printf("[Agent] ❌ E-Mail-Versand fehlgeschlagen: %v", err)
			return fmt.Sprintf("❌ E-Mail konnte nicht gesendet werden:\n_%v_", err)
		}

		log.Printf("[Agent] ✅ E-Mail erfolgreich gesendet an %s", state.To)
		return fmt.Sprintf("✅ E-Mail an *%s* wurde erfolgreich gesendet.", state.To)
	}

	// Unklare Antwort
	return "❓ Bitte antworte mit *ja* (senden) oder *nein* (abbrechen)."
}

// ── CAL.COM INTEGRATION ──────────────────────────────────────────────────────

// calcomBookingPayload beschreibt die Buchungsdaten aus dem KI-Marker.
type calcomBookingPayload struct {
	Title         string `json:"title"`
	Start         string `json:"start"`
	End           string `json:"end"`
	AttendeeName  string `json:"attendeeName"`
	AttendeeEmail string `json:"attendeeEmail"`
	TimeZone      string `json:"timeZone"`
}

// isV2API gibt true zurück wenn die konfigurierte Base-URL auf V2 hinweist.
func (a *Agent) isV2API() bool {
	return strings.Contains(a.calcomBaseURL, "/v2")
}

// calcomRequest erstellt einen HTTP-Request mit den richtigen Auth-Headern für V1 oder V2.
// V1: ?apiKey= als Query-Parameter + Bearer als Fallback-Header
// V2: Authorization: Bearer + cal-api-version Header (kein Query-Parameter)
func (a *Agent) calcomRequest(method, path string, bodyBytes []byte) (*http.Request, error) {
	baseURL := strings.TrimRight(a.calcomBaseURL, "/")
	var apiURL string
	if a.isV2API() {
		apiURL = baseURL + path // kein ?apiKey= für V2
	} else {
		apiURL = baseURL + path + "?apiKey=" + a.calcomAPIKey
	}

	var reqBody *bytes.Reader
	if bodyBytes != nil {
		reqBody = bytes.NewReader(bodyBytes)
	} else {
		reqBody = bytes.NewReader([]byte{})
	}

	req, err := http.NewRequest(method, apiURL, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.calcomAPIKey)
	if a.isV2API() {
		req.Header.Set("cal-api-version", "2024-06-14")
	}
	return req, nil
}

// handleCalcomBooking parst den __CALCOM_BOOK__-Marker und erstellt einen echten Termin via Cal.com API.
// Unterstützt V1 ({ "responses": {...} }) und V2 ({ "attendee": {...} }) Payload-Format.
func (a *Agent) handleCalcomBooking(response string) string {
	if a.calcomBaseURL == "" || a.calcomAPIKey == "" {
		return "❌ Cal.com ist nicht konfiguriert.\n\n" +
			"Trage im Dashboard → Integrationen → Cal.com die API-Adresse und den API-Key ein."
	}

	start := strings.Index(response, "__CALCOM_BOOK__")
	end := strings.Index(response, "__CALCOM_BOOK_END__")
	if start < 0 || end < 0 || end <= start {
		return "❌ Cal.com Buchungs-Format fehlerhaft. Bitte nochmal versuchen."
	}

	jsonStr := strings.TrimSpace(response[start+len("__CALCOM_BOOK__") : end])

	var booking calcomBookingPayload
	if err := json.Unmarshal([]byte(jsonStr), &booking); err != nil {
		log.Printf("[Agent] ❌ Cal.com JSON ungültig: %v | Input: %s", err, jsonStr)
		return fmt.Sprintf("❌ Cal.com Buchungs-Format ungültig: %v", err)
	}

	// Defaults
	if booking.AttendeeName == "" {
		booking.AttendeeName = "JJ"
	}
	if booking.AttendeeEmail == "" {
		booking.AttendeeEmail = a.calcomOwnerEmail
	}
	if booking.TimeZone == "" {
		booking.TimeZone = "Europe/Vienna"
	}
	if booking.End == "" && booking.Start != "" {
		if t, err := time.Parse(time.RFC3339, booking.Start); err == nil {
			booking.End = t.Add(60 * time.Minute).Format(time.RFC3339)
		} else {
			booking.End = booking.Start
		}
	}

	// Event Type ID ermitteln
	eventTypeID, err := a.resolveCalcomEventTypeID()
	if err != nil {
		log.Printf("[Agent] ❌ Cal.com Event Type ID: %v", err)
		return fmt.Sprintf("❌ Cal.com Event Type ID konnte nicht ermittelt werden:\n_%v_\n\n"+
			"Lösung: Cal.com/Cal.eu öffnen → Event Types → Termin-Typ anklicken → "+
			"Zahl aus der URL kopieren (z.B. /event-types/123456) → "+
			"Dashboard → Integrationen → Cal.com → Feld 'Event Type ID' eintragen → Speichern.", err)
	}

	// Payload je nach API-Version aufbauen
	var payload map[string]interface{}
	if a.isV2API() {
		// Cal.com V2: timeZone + language + metadata auf Root-Ebene (nicht im attendee-Objekt).
		// responses enthält name + email des Teilnehmers.
		payload = map[string]interface{}{
			"eventTypeId": eventTypeID,
			"start":       booking.Start,
			"timeZone":    booking.TimeZone,
			"language":    "de",
			"metadata":    map[string]interface{}{},
			"responses": map[string]string{
				"name":  booking.AttendeeName,
				"email": booking.AttendeeEmail,
			},
		}
	} else {
		// Cal.com V1
		payload = map[string]interface{}{
			"eventTypeId": eventTypeID,
			"start":       booking.Start,
			"end":         booking.End,
			"responses": map[string]string{
				"name":  booking.AttendeeName,
				"email": booking.AttendeeEmail,
			},
			"timeZone": booking.TimeZone,
			"metadata": map[string]interface{}{},
		}
		if booking.Title != "" {
			payload["title"] = booking.Title
		}
	}

	body, _ := json.Marshal(payload)
	req, err := a.calcomRequest(http.MethodPost, "/bookings", body)
	if err != nil {
		return fmt.Sprintf("❌ HTTP-Request konnte nicht erstellt werden: %v", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[Agent] ❌ Cal.com POST /bookings: %v", err)
		return fmt.Sprintf("❌ Cal.com Buchung fehlgeschlagen:\n_%v_", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[Agent] ✅ Cal.com Termin erstellt: %s (Start: %s)", booking.Title, booking.Start)
		return fmt.Sprintf("✅ Termin *%s* wurde erfolgreich eingetragen!\n\n📅 *Start:* %s",
			booking.Title, booking.Start)
	}

	log.Printf("[Agent] ❌ Cal.com API Fehler %d: %s", resp.StatusCode, string(respBody))
	return fmt.Sprintf("❌ Cal.com API Fehler (%d):\n_%s_", resp.StatusCode, string(respBody))
}

// resolveCalcomEventTypeID gibt die konfigurierte Event Type ID zurück oder ermittelt sie via API.
// Unterstützt V1 { "event_types": [...] } und V2 { "data": { "eventTypeGroups": [...] } }.
func (a *Agent) resolveCalcomEventTypeID() (int, error) {
	if a.calcomEventTypeID > 0 {
		log.Printf("[Agent] 📅 Cal.com EventTypeID: %d (konfiguriert)", a.calcomEventTypeID)
		return a.calcomEventTypeID, nil
	}

	req, err := a.calcomRequest(http.MethodGet, "/event-types", nil)
	if err != nil {
		return 0, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("API Fehler %d: %s", resp.StatusCode, string(body))
	}

	// V2: { "status": "success", "data": { "eventTypeGroups": [{ "eventTypes": [{ "id": 1 }] }] } }
	if a.isV2API() {
		var v2 struct {
			Data struct {
				EventTypeGroups []struct {
					EventTypes []struct {
						ID int `json:"id"`
					} `json:"eventTypes"`
				} `json:"eventTypeGroups"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &v2); err == nil {
			for _, g := range v2.Data.EventTypeGroups {
				if len(g.EventTypes) > 0 {
					log.Printf("[Agent] 📅 Cal.com V2 EventTypeID auto: %d", g.EventTypes[0].ID)
					return g.EventTypes[0].ID, nil
				}
			}
		}
	} else {
		// V1: { "event_types": [...] }
		var v1 struct {
			EventTypes []struct {
				ID int `json:"id"`
			} `json:"event_types"`
		}
		if err := json.Unmarshal(body, &v1); err == nil && len(v1.EventTypes) > 0 {
			log.Printf("[Agent] 📅 Cal.com V1 EventTypeID auto: %d", v1.EventTypes[0].ID)
			return v1.EventTypes[0].ID, nil
		}
	}

	return 0, fmt.Errorf("keine Event Types gefunden – bitte Event Type ID manuell im Dashboard → Integrationen → Cal.com eintragen")
}

// handleCalcomList lädt die anstehenden Termine via GET /bookings.
// Unterstützt V1 und V2 Antwort-Formate.
func (a *Agent) handleCalcomList() string {
	if a.calcomBaseURL == "" || a.calcomAPIKey == "" {
		return "❌ Cal.com ist nicht konfiguriert.\n\n" +
			"Trage im Dashboard → Integrationen → Cal.com die API-Adresse und den API-Key ein."
	}

	req, err := a.calcomRequest(http.MethodGet, "/bookings", nil)
	if err != nil {
		return fmt.Sprintf("❌ Request konnte nicht erstellt werden: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("❌ Termine konnten nicht geladen werden:\n_%v_", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Sprintf("❌ Cal.com API Fehler (%d):\n_%s_", resp.StatusCode, string(body))
	}

	type bookingEntry struct {
		Title     string `json:"title"`
		StartTime string `json:"startTime"`
		Status    string `json:"status"`
	}
	var bookings []bookingEntry

	if a.isV2API() {
		// V2: { "status": "success", "data": [ { "title": ..., "start": ..., "status": ... } ] }
		var v2 struct {
			Data []struct {
				Title  string `json:"title"`
				Start  string `json:"start"`
				Status string `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &v2); err == nil {
			for _, b := range v2.Data {
				bookings = append(bookings, bookingEntry{Title: b.Title, StartTime: b.Start, Status: b.Status})
			}
		}
	} else {
		// V1: { "bookings": [...] }
		var v1 struct {
			Bookings []bookingEntry `json:"bookings"`
		}
		if err := json.Unmarshal(body, &v1); err == nil {
			bookings = v1.Bookings
		}
	}

	if len(bookings) == 0 {
		return "📅 Keine bevorstehenden Termine gefunden."
	}

	var sb strings.Builder
	sb.WriteString("📅 *Deine nächsten Termine:*\n\n")
	limit := 10
	if len(bookings) < limit {
		limit = len(bookings)
	}
	for _, b := range bookings[:limit] {
		status := ""
		if b.Status == "CANCELLED" {
			status = " ~~abgesagt~~"
		}
		sb.WriteString(fmt.Sprintf("• *%s* – %s%s\n", b.Title, b.StartTime, status))
	}
	log.Printf("[Agent] 📅 Cal.com Terminliste: %d Einträge", len(bookings))
	return strings.TrimRight(sb.String(), "\n")
}

// ── GOOGLE API HANDLER ─────────────────────────────────────────────────────

// googleNotConfigured gibt eine einheitliche Fehlermeldung zurück.
func (a *Agent) googleNotConfigured() string {
	return "❌ Google API ist nicht konfiguriert.\n\n" +
		"Bitte trage im Dashboard → Integrationen → Google deine OAuth2-Zugangsdaten ein " +
		"(Client ID, Client Secret, Refresh Token)."
}

// parseGoogleMarker extrahiert den JSON-Block zwischen start- und end-Marker.
func parseGoogleMarker(response, startMarker, endMarker string) ([]byte, error) {
	s := strings.Index(response, startMarker)
	e := strings.Index(response, endMarker)
	if s < 0 || e < 0 {
		return nil, fmt.Errorf("marker nicht gefunden")
	}
	block := strings.TrimSpace(response[s+len(startMarker) : e])
	// Markdown-Codeblock entfernen falls vorhanden
	block = strings.TrimPrefix(block, "```json")
	block = strings.TrimPrefix(block, "```")
	block = strings.TrimSuffix(block, "```")
	return []byte(strings.TrimSpace(block)), nil
}

// handleGoogleCalCreate erstellt einen Google Calendar-Termin.
// Marker-Format: __GOOGLE_CAL_CREATE__\n{"title":"...","start":"...","end":"...","description":"...","location":"...","calendarId":"primary"}\n__GOOGLE_CAL_CREATE_END__
func (a *Agent) handleGoogleCalCreate(response string) string {
	if a.googleClient == nil || !a.googleClient.IsConfigured() {
		return a.googleNotConfigured()
	}
	data, err := parseGoogleMarker(response, "__GOOGLE_CAL_CREATE__", "__GOOGLE_CAL_CREATE_END__")
	if err != nil {
		return "❌ Google Calendar: Ungültiger Marker-Block."
	}
	var ev googleapi.CalendarEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Sprintf("❌ Google Calendar: JSON-Fehler: %v", err)
	}
	result, err := a.googleClient.CalendarCreate(ev)
	if err != nil {
		log.Printf("[Agent] ❌ Google Calendar Fehler: %v", err)
		return fmt.Sprintf("❌ Google Calendar Fehler: %v", err)
	}
	log.Printf("[Agent] 📅 Google Calendar Termin erstellt: %s", result.Title)
	return fmt.Sprintf("✅ Termin *%s* wurde in Google Calendar eingetragen!\n🔗 %s", result.Title, result.HtmlURL)
}

// handleGoogleCalList listet bevorstehende Google Calendar-Termine auf.
// Marker-Format: __GOOGLE_CAL_LIST__
func (a *Agent) handleGoogleCalList() string {
	if a.googleClient == nil || !a.googleClient.IsConfigured() {
		return a.googleNotConfigured()
	}
	events, err := a.googleClient.CalendarList("primary", 10)
	if err != nil {
		log.Printf("[Agent] ❌ Google Calendar List Fehler: %v", err)
		return fmt.Sprintf("❌ Google Calendar Fehler: %v", err)
	}
	if len(events) == 0 {
		return "📅 Keine bevorstehenden Termine in Google Calendar."
	}
	var sb strings.Builder
	sb.WriteString("📅 *Deine nächsten Google Calendar Termine:*\n\n")
	for _, e := range events {
		sb.WriteString(fmt.Sprintf("• *%s* – %s\n", e.Title, e.Start))
		if e.Location != "" {
			sb.WriteString(fmt.Sprintf("  📍 %s\n", e.Location))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// handleGoogleDocsCreate erstellt ein neues Google Docs-Dokument.
// Marker-Format: __GOOGLE_DOCS_CREATE__\n{"title":"...","content":"..."}\n__GOOGLE_DOCS_CREATE_END__
func (a *Agent) handleGoogleDocsCreate(response string) string {
	if a.googleClient == nil || !a.googleClient.IsConfigured() {
		return a.googleNotConfigured()
	}
	data, err := parseGoogleMarker(response, "__GOOGLE_DOCS_CREATE__", "__GOOGLE_DOCS_CREATE_END__")
	if err != nil {
		return "❌ Google Docs: Ungültiger Marker-Block."
	}
	var payload struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Sprintf("❌ Google Docs: JSON-Fehler: %v", err)
	}
	result, err := a.googleClient.DocsCreate(payload.Title, payload.Content)
	if err != nil {
		log.Printf("[Agent] ❌ Google Docs Fehler: %v", err)
		return fmt.Sprintf("❌ Google Docs Fehler: %v", err)
	}
	log.Printf("[Agent] 📄 Google Docs Dokument erstellt: %s (%s)", result.Title, result.DocID)
	return fmt.Sprintf("✅ Dokument *%s* wurde in Google Docs erstellt!\n🔗 %s", result.Title, result.URL)
}

// handleGoogleDocsAppend fügt Text an ein bestehendes Google Docs-Dokument an.
// Marker-Format: __GOOGLE_DOCS_APPEND__\n{"docId":"...","content":"..."}\n__GOOGLE_DOCS_APPEND_END__
func (a *Agent) handleGoogleDocsAppend(response string) string {
	if a.googleClient == nil || !a.googleClient.IsConfigured() {
		return a.googleNotConfigured()
	}
	data, err := parseGoogleMarker(response, "__GOOGLE_DOCS_APPEND__", "__GOOGLE_DOCS_APPEND_END__")
	if err != nil {
		return "❌ Google Docs: Ungültiger Marker-Block."
	}
	var payload struct {
		DocID   string `json:"docId"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Sprintf("❌ Google Docs: JSON-Fehler: %v", err)
	}
	if err := a.googleClient.DocsAppend(payload.DocID, payload.Content); err != nil {
		log.Printf("[Agent] ❌ Google Docs Append Fehler: %v", err)
		return fmt.Sprintf("❌ Google Docs Fehler: %v", err)
	}
	log.Printf("[Agent] 📄 Google Docs Text angehängt an: %s", payload.DocID)
	return fmt.Sprintf("✅ Text wurde an das Google Docs-Dokument angehängt.\n🔗 https://docs.google.com/document/d/%s/edit", payload.DocID)
}

// handleGoogleDocsRead liest den Inhalt eines Google Docs-Dokuments.
// Marker-Format: __GOOGLE_DOCS_READ__\n{"docId":"..."}\n__GOOGLE_DOCS_READ_END__
func (a *Agent) handleGoogleDocsRead(response string) string {
	if a.googleClient == nil || !a.googleClient.IsConfigured() {
		return a.googleNotConfigured()
	}
	data, err := parseGoogleMarker(response, "__GOOGLE_DOCS_READ__", "__GOOGLE_DOCS_READ_END__")
	if err != nil {
		return "❌ Google Docs: Ungültiger Marker-Block."
	}
	var payload struct {
		DocID string `json:"docId"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Sprintf("❌ Google Docs: JSON-Fehler: %v", err)
	}
	title, content, err := a.googleClient.DocsRead(payload.DocID)
	if err != nil {
		log.Printf("[Agent] ❌ Google Docs Read Fehler: %v", err)
		return fmt.Sprintf("❌ Google Docs Fehler: %v", err)
	}
	// Inhalt auf sinnvolle Länge kürzen
	if len(content) > 2000 {
		content = content[:2000] + "\n\n[... Dokument zu lang, nur Anfang angezeigt]"
	}
	return fmt.Sprintf("📄 *%s*\n\n%s", title, content)
}

// handleGoogleSheetsCreate erstellt eine neue Google Sheets-Tabelle.
// Marker-Format: __GOOGLE_SHEETS_CREATE__\n{"title":"..."}\n__GOOGLE_SHEETS_CREATE_END__
func (a *Agent) handleGoogleSheetsCreate(response string) string {
	if a.googleClient == nil || !a.googleClient.IsConfigured() {
		return a.googleNotConfigured()
	}
	data, err := parseGoogleMarker(response, "__GOOGLE_SHEETS_CREATE__", "__GOOGLE_SHEETS_CREATE_END__")
	if err != nil {
		return "❌ Google Sheets: Ungültiger Marker-Block."
	}
	var payload struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Sprintf("❌ Google Sheets: JSON-Fehler: %v", err)
	}
	result, err := a.googleClient.SheetsCreate(payload.Title)
	if err != nil {
		log.Printf("[Agent] ❌ Google Sheets Fehler: %v", err)
		return fmt.Sprintf("❌ Google Sheets Fehler: %v", err)
	}
	log.Printf("[Agent] 📊 Google Sheets erstellt: %s (%s)", result.Title, result.SheetID)
	return fmt.Sprintf("✅ Tabelle *%s* wurde in Google Sheets erstellt!\n🔗 %s", result.Title, result.URL)
}

// handleGoogleSheetsRead liest Werte aus einer Google Sheets-Tabelle.
// Marker-Format: __GOOGLE_SHEETS_READ__\n{"sheetId":"...","range":"A1:Z100"}\n__GOOGLE_SHEETS_READ_END__
func (a *Agent) handleGoogleSheetsRead(response string) string {
	if a.googleClient == nil || !a.googleClient.IsConfigured() {
		return a.googleNotConfigured()
	}
	data, err := parseGoogleMarker(response, "__GOOGLE_SHEETS_READ__", "__GOOGLE_SHEETS_READ_END__")
	if err != nil {
		return "❌ Google Sheets: Ungültiger Marker-Block."
	}
	var payload struct {
		SheetID string `json:"sheetId"`
		Range   string `json:"range"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Sprintf("❌ Google Sheets: JSON-Fehler: %v", err)
	}
	rows, err := a.googleClient.SheetsRead(payload.SheetID, payload.Range)
	if err != nil {
		log.Printf("[Agent] ❌ Google Sheets Read Fehler: %v", err)
		return fmt.Sprintf("❌ Google Sheets Fehler: %v", err)
	}
	if len(rows) == 0 {
		return "📊 Die Tabelle ist leer (oder der angegebene Bereich enthält keine Daten)."
	}
	var sb strings.Builder
	sb.WriteString("📊 *Google Sheets Inhalt:*\n\n```\n")
	maxRows := 30
	if len(rows) < maxRows {
		maxRows = len(rows)
	}
	for _, row := range rows[:maxRows] {
		sb.WriteString(strings.Join(row, " | "))
		sb.WriteString("\n")
	}
	if len(rows) > 30 {
		sb.WriteString(fmt.Sprintf("... (%d weitere Zeilen nicht angezeigt)\n", len(rows)-30))
	}
	sb.WriteString("```")
	return sb.String()
}

// handleGoogleSheetsWrite schreibt Werte in eine Google Sheets-Tabelle.
// Marker-Format: __GOOGLE_SHEETS_WRITE__\n{"sheetId":"...","range":"A1","values":[["Name","Wert"],["Test","123"]]}\n__GOOGLE_SHEETS_WRITE_END__
func (a *Agent) handleGoogleSheetsWrite(response string) string {
	if a.googleClient == nil || !a.googleClient.IsConfigured() {
		return a.googleNotConfigured()
	}
	data, err := parseGoogleMarker(response, "__GOOGLE_SHEETS_WRITE__", "__GOOGLE_SHEETS_WRITE_END__")
	if err != nil {
		return "❌ Google Sheets: Ungültiger Marker-Block."
	}
	var payload struct {
		SheetID string          `json:"sheetId"`
		Range   string          `json:"range"`
		Values  [][]interface{} `json:"values"`
		Append  bool            `json:"append"` // true = anhängen statt überschreiben
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Sprintf("❌ Google Sheets: JSON-Fehler: %v", err)
	}
	var writeErr error
	if payload.Append {
		writeErr = a.googleClient.SheetsAppend(payload.SheetID, payload.Range, payload.Values)
	} else {
		writeErr = a.googleClient.SheetsWrite(payload.SheetID, payload.Range, payload.Values)
	}
	if writeErr != nil {
		log.Printf("[Agent] ❌ Google Sheets Write Fehler: %v", writeErr)
		return fmt.Sprintf("❌ Google Sheets Fehler: %v", writeErr)
	}
	action := "geschrieben"
	if payload.Append {
		action = "angehängt"
	}
	log.Printf("[Agent] 📊 Google Sheets Daten %s (Sheet: %s, Range: %s)", action, payload.SheetID, payload.Range)
	return fmt.Sprintf("✅ Daten wurden in Google Sheets %s.\n🔗 https://docs.google.com/spreadsheets/d/%s/edit", action, payload.SheetID)
}

// handleGoogleDriveList listet Dateien in Google Drive auf.
// Marker-Format: __GOOGLE_DRIVE_LIST__\n{"query":"name contains 'Report'","maxResults":10}\n__GOOGLE_DRIVE_LIST_END__
func (a *Agent) handleGoogleDriveList(response string) string {
	if a.googleClient == nil || !a.googleClient.IsConfigured() {
		return a.googleNotConfigured()
	}
	data, err := parseGoogleMarker(response, "__GOOGLE_DRIVE_LIST__", "__GOOGLE_DRIVE_LIST_END__")
	if err != nil {
		return "❌ Google Drive: Ungültiger Marker-Block."
	}
	var payload struct {
		Query      string `json:"query"`
		MaxResults int    `json:"maxResults"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Sprintf("❌ Google Drive: JSON-Fehler: %v", err)
	}
	files, err := a.googleClient.DriveList(payload.Query, payload.MaxResults)
	if err != nil {
		log.Printf("[Agent] ❌ Google Drive Fehler: %v", err)
		return fmt.Sprintf("❌ Google Drive Fehler: %v", err)
	}
	if len(files) == 0 {
		return "📁 Keine Dateien in Google Drive gefunden."
	}
	var sb strings.Builder
	sb.WriteString("📁 *Google Drive Dateien:*\n\n")
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("• *%s*\n  🔗 %s\n", f.Name, f.URL))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// handleGmailSend sendet eine E-Mail über Gmail.
// Marker-Format: __GMAIL_SEND__\n{"to":"...","subject":"...","body":"..."}\n__GMAIL_SEND_END__
func (a *Agent) handleGmailSend(response string) string {
	if a.googleClient == nil || !a.googleClient.IsConfigured() {
		return a.googleNotConfigured()
	}
	data, err := parseGoogleMarker(response, "__GMAIL_SEND__", "__GMAIL_SEND_END__")
	if err != nil {
		return "❌ Gmail: Ungültiger Marker-Block."
	}
	var msg googleapi.GmailMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return fmt.Sprintf("❌ Gmail: JSON-Fehler: %v", err)
	}
	if err := a.googleClient.GmailSend(msg); err != nil {
		log.Printf("[Agent] ❌ Gmail Send Fehler: %v", err)
		return fmt.Sprintf("❌ Gmail Fehler: %v", err)
	}
	return fmt.Sprintf("✅ E-Mail an *%s* wurde über Gmail gesendet!\n📧 Betreff: %s", msg.To, msg.Subject)
}

// handleGmailList listet E-Mails aus Gmail auf.
// Marker-Format: __GMAIL_LIST__\n{"query":"is:unread","maxResults":10}\n__GMAIL_LIST_END__
func (a *Agent) handleGmailList(response string) string {
	if a.googleClient == nil || !a.googleClient.IsConfigured() {
		return a.googleNotConfigured()
	}
	data, err := parseGoogleMarker(response, "__GMAIL_LIST__", "__GMAIL_LIST_END__")
	if err != nil {
		return "❌ Gmail: Ungültiger Marker-Block."
	}
	var payload struct {
		Query      string `json:"query"`
		MaxResults int    `json:"maxResults"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Sprintf("❌ Gmail: JSON-Fehler: %v", err)
	}
	messages, err := a.googleClient.GmailList(payload.Query, payload.MaxResults)
	if err != nil {
		log.Printf("[Agent] ❌ Gmail List Fehler: %v", err)
		return fmt.Sprintf("❌ Gmail Fehler: %v", err)
	}
	if len(messages) == 0 {
		return "📧 Keine E-Mails gefunden."
	}
	var sb strings.Builder
	sb.WriteString("📧 *Gmail:*\n\n")
	for _, m := range messages {
		sb.WriteString(fmt.Sprintf("• *%s*\n  Von: %s | %s\n  %s\n\n", m.Subject, m.From, m.Date, m.Snippet))
	}
	return strings.TrimRight(sb.String(), "\n")
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

	calcomInstruction := ""
	if a.calcomBaseURL != "" && a.calcomAPIKey != "" {
		ownerEmail := a.calcomOwnerEmail
		calcomInstruction = "\nCAL.COM TERMINBUCHUNG – HÖCHSTE PRIORITÄT:\n" +
			"Wenn der Nutzer einen Termin erstellen, buchen oder eintragen möchte – egal wie formuliert – " +
			"antworte EXAKT mit folgendem Format (kein weiterer Text außer dem Marker-Block):\n" +
			"__CALCOM_BOOK__\n" +
			"{\"title\":\"<Titel>\",\"start\":\"<ISO8601 UTC z.B. 2026-02-22T19:00:00Z>\",\"end\":\"<ISO8601 UTC>\",\"attendeeName\":\"JJ\",\"attendeeEmail\":\"" + ownerEmail + "\",\"timeZone\":\"Europe/Vienna\"}\n" +
			"__CALCOM_BOOK_END__\n" +
			"Zeitumrechnung Wien→UTC: UTC+1 im Winter (Feb–März), UTC+2 im Sommer.\n" +
			"Wenn der Nutzer Termine auflisten oder anzeigen möchte – antworte NUR mit: __CALCOM_LIST__\n" +
			"Niemals eventTypeId, E-Mail oder API-Key beim Nutzer erfragen – alles ist konfiguriert.\n"
	}

	emailInstruction := ""
	if a.emailSender != nil && a.emailSender.IsConfigured() {
		emailInstruction = "\nE-MAIL-VERSAND – HÖCHSTE PRIORITÄT:\n" +
			"Wenn der Nutzer eine E-Mail schreiben, senden oder versenden möchte – egal wie er es formuliert – " +
			"antworte EXAKT mit folgendem Format (kein weiterer Text):\n" +
			"__SEND_EMAIL__\n" +
			"TO:<empfänger@email.de>\n" +
			"SUBJECT:<Betreff>\n" +
			"BODY:<E-Mail-Text>\n" +
			"__EMAIL_END__\n" +
			"Wichtig: Der E-Mail-Text im BODY darf mehrere Zeilen haben. Kein Text vor oder nach den Markern.\n"
	}

	googleInstruction := ""
	if a.googleClient != nil && a.googleClient.IsConfigured() {
		googleInstruction = "\nGOOGLE WORKSPACE – HÖCHSTE PRIORITÄT:\n" +
			"Du hast Zugriff auf Google Calendar, Docs, Sheets, Drive und Gmail.\n" +
			"Verwende AUSSCHLIESSLICH die folgenden Marker wenn der Nutzer Google-Dienste nutzen möchte:\n\n" +
			"GOOGLE CALENDAR:\n" +
			"Termin erstellen:\n__GOOGLE_CAL_CREATE__\n{\"title\":\"<Titel>\",\"start\":\"<RFC3339+01:00>\",\"end\":\"<RFC3339+01:00>\",\"description\":\"<optional>\",\"location\":\"<optional>\"}\n__GOOGLE_CAL_CREATE_END__\n" +
			"Termine auflisten: antworte NUR mit: __GOOGLE_CAL_LIST__\n\n" +
			"GOOGLE DOCS:\n" +
			"Dokument erstellen:\n__GOOGLE_DOCS_CREATE__\n{\"title\":\"<Titel>\",\"content\":\"<Inhalt>\"}\n__GOOGLE_DOCS_CREATE_END__\n" +
			"Text anhängen:\n__GOOGLE_DOCS_APPEND__\n{\"docId\":\"<ID>\",\"content\":\"<Text>\"}\n__GOOGLE_DOCS_APPEND_END__\n" +
			"Dokument lesen:\n__GOOGLE_DOCS_READ__\n{\"docId\":\"<ID>\"}\n__GOOGLE_DOCS_READ_END__\n\n" +
			"GOOGLE SHEETS:\n" +
			"Tabelle erstellen:\n__GOOGLE_SHEETS_CREATE__\n{\"title\":\"<Titel>\"}\n__GOOGLE_SHEETS_CREATE_END__\n" +
			"Werte lesen:\n__GOOGLE_SHEETS_READ__\n{\"sheetId\":\"<ID>\",\"range\":\"A1:Z100\"}\n__GOOGLE_SHEETS_READ_END__\n" +
			"Werte schreiben:\n__GOOGLE_SHEETS_WRITE__\n{\"sheetId\":\"<ID>\",\"range\":\"A1\",\"values\":[[\"Spalte1\",\"Spalte2\"],[\"Wert1\",\"Wert2\"]],\"append\":false}\n__GOOGLE_SHEETS_WRITE_END__\n\n" +
			"GOOGLE DRIVE:\n" +
			"Dateien suchen:\n__GOOGLE_DRIVE_LIST__\n{\"query\":\"name contains '<Suchbegriff>'\",\"maxResults\":10}\n__GOOGLE_DRIVE_LIST_END__\n\n" +
			"GMAIL:\n" +
			"E-Mail senden:\n__GMAIL_SEND__\n{\"to\":\"<email>\",\"subject\":\"<Betreff>\",\"body\":\"<Text>\"}\n__GMAIL_SEND_END__\n" +
			"E-Mails auflisten:\n__GMAIL_LIST__\n{\"query\":\"is:unread\",\"maxResults\":10}\n__GMAIL_LIST_END__\n" +
			"Zeitzone für Kalender: Europe/Vienna. RFC3339-Format: 2026-02-25T10:00:00+01:00\n"
	}

	cronInstruction := ""
	if a.cronManager != nil {
		cronInstruction = "\nERINNERUNGEN (CRON) – HÖCHSTE PRIORITÄT:\n" +
			"Du kannst für den Nutzer Erinnerungen anlegen, auflisten und löschen.\n" +
			"WICHTIG: Frage den Nutzer immer nach der Zeitzone (z.B. Europe/Vienna), wenn sie nicht angegeben ist!\n\n" +
			"Erinnerung anlegen – antworte EXAKT mit:\n" +
			"__REMINDER_CREATE__\n" +
			"{\"time_spec\":\"<täglich um 08:00 / montags um 09:30 / stündlich>\",\"timezone\":\"<z.B. Europe/Vienna>\",\"message\":\"<Erinnerungstext>\"}\n" +
			"__REMINDER_CREATE_END__\n\n" +
			"Erinnerungen auflisten – antworte NUR mit: __REMINDER_LIST__\n\n" +
			"Erinnerung löschen – antworte EXAKT mit:\n" +
			"__REMINDER_DELETE__\n" +
			"{\"id\":<Nummer>}\n" +
			"__REMINDER_DELETE_END__\n" +
			"Unterstützte Zeitspezifikationen: \"täglich um HH:MM\", \"montags um HH:MM\", \"stündlich\".\n"
	}

	prompt += "ANTWORTREGELN – STRIKTE PFLICHT:\n" +
		"- Beantworte NUR was gefragt wurde. Nichts mehr.\n" +
		"- Maximal 2-3 Sätze, außer der Nutzer fragt EXPLIZIT nach einer ausführlichen Antwort.\n" +
		"- KEINE ungebetenen Zusatzinfos, Tipps, Links, Vorschläge oder Erklärungen.\n" +
		"- KEINE Einleitungsphrasen wie \"Natürlich!\", \"Gerne!\", \"Sicher!\", \"Selbstverständlich!\".\n" +
		"- Bei einfachen Bestätigungen oder Statusmeldungen: eine Zeile reicht.\n" +
		calcomInstruction +
		emailInstruction +
		googleInstruction +
		cronInstruction +
		"\nVIDEO-ERKENNUNG – HÖCHSTE PRIORITÄT:\n" +
		"Wenn der Nutzer in irgendeiner Form ein Video erstellen, generieren, produzieren, drehen, animieren oder rendern lassen möchte – egal wie er es formuliert, in welcher Sprache oder mit welchen Worten – antworte ausschließlich mit dem exakten Text: __VIDEO_REQUEST__\n" +
		"Kein weiterer Text, keine Erklärung, nur: __VIDEO_REQUEST__\n" +
		"\nSKILL-ERSTELLUNG – HÖCHSTE PRIORITÄT:\n" +
		"Wenn der Nutzer einen neuen Skill erstellen, speichern oder einrichten möchte, erstelle den kompletten Skill-Inhalt im Markdown-Format und gib EXAKT folgendes aus:\n" +
		"__SKILL_NAME:dateiname-ohne-leerzeichen__\n" +
		"<vollständiger skill-inhalt mit frontmatter>\n" +
		"__SKILL_END__\n" +
		"Das Frontmatter muss name, tags und mindestens einen Aktivierungs-Hinweis enthalten.\n" +
		"Für externe API-Keys in Skills verwende {{PLATZHALTER_NAME}} – nie echte Keys im Skill-Inhalt.\n" +
		"Kein Text vor oder nach den Markern.\n"

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

// ── Cron-Reminder Handler ─────────────────────────────────────────────────────

// reminderCreatePayload beschreibt die JSON-Daten aus dem __REMINDER_CREATE__-Marker.
type reminderCreatePayload struct {
	TimeSpec string `json:"time_spec"` // z.B. "täglich um 06:00"
	Timezone string `json:"timezone"`  // z.B. "Europe/Vienna"
	Message  string `json:"message"`   // Erinnerungstext
}

// handleReminderCreate legt eine neue Erinnerung an.
func (a *Agent) handleReminderCreate(msg channels.Message, response string) string {
	if a.cronManager == nil {
		return "⏰ Erinnerungen sind aktuell nicht aktiviert."
	}
	start := strings.Index(response, "__REMINDER_CREATE__")
	end := strings.Index(response, "__REMINDER_CREATE_END__")
	if start < 0 || end < 0 || end <= start {
		return "❌ Interner Fehler beim Verarbeiten der Erinnerung."
	}
	jsonStr := strings.TrimSpace(response[start+len("__REMINDER_CREATE__") : end])

	var p reminderCreatePayload
	if err := json.Unmarshal([]byte(jsonStr), &p); err != nil {
		log.Printf("[Agent] Reminder-JSON Fehler: %v | Raw: %s", err, jsonStr)
		return "❌ Erinnerung konnte nicht verarbeitet werden. Bitte nochmals versuchen."
	}

	cronExpr, timeStr, err := cronpkg.ParseReminderRequest(p.TimeSpec, p.Timezone)
	if err != nil {
		return fmt.Sprintf("❌ %v", err)
	}

	r := &cronpkg.Reminder{
		UserID:    msg.SenderID,
		UserName:  msg.UserName,
		ChannelID: msg.ChannelID,
		ChatID:    msg.ChatID,
		CronExpr:  cronExpr,
		TimeStr:   timeStr,
		Timezone:  p.Timezone,
		Message:   p.Message,
	}

	confirm, err := a.cronManager.AddReminder(r)
	if err != nil {
		return fmt.Sprintf("❌ Fehler beim Speichern: %v", err)
	}
	return confirm
}

// handleReminderList listet alle Erinnerungen des Users auf.
func (a *Agent) handleReminderList(msg channels.Message) string {
	if a.cronManager == nil {
		return "⏰ Erinnerungen sind aktuell nicht aktiviert."
	}
	return a.cronManager.ListReminders(msg.SenderID)
}

// reminderDeletePayload beschreibt die JSON-Daten aus dem __REMINDER_DELETE__-Marker.
type reminderDeletePayload struct {
	ID int `json:"id"`
}

// handleReminderDelete löscht eine Erinnerung anhand der ID.
func (a *Agent) handleReminderDelete(msg channels.Message, response string) string {
	if a.cronManager == nil {
		return "⏰ Erinnerungen sind aktuell nicht aktiviert."
	}
	start := strings.Index(response, "__REMINDER_DELETE__")
	end := strings.Index(response, "__REMINDER_DELETE_END__")
	if start < 0 || end < 0 || end <= start {
		return "❌ Interner Fehler beim Verarbeiten der Lösch-Anfrage."
	}
	jsonStr := strings.TrimSpace(response[start+len("__REMINDER_DELETE__") : end])

	var p reminderDeletePayload
	if err := json.Unmarshal([]byte(jsonStr), &p); err != nil {
		return "❌ Lösch-Anfrage konnte nicht verarbeitet werden."
	}

	confirm, err := a.cronManager.DeleteReminder(p.ID, msg.SenderID)
	if err != nil {
		return fmt.Sprintf("❌ %v", err)
	}
	return confirm
}
