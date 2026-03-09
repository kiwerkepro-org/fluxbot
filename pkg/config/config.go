package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

// Config ist die Hauptkonfiguration von FluxBot (config.json)
type Config struct {
	Channels     ChannelsConfig  `json:"channels"`
	Providers    ProvidersConfig `json:"providers"`
	Workspace    WorkspaceConfig `json:"workspace"`
	Voice        VoiceConfig     `json:"voice"`
	ImageGen     ImageGenConfig  `json:"imageGen"`
	VideoGen     VideoGenConfig  `json:"videoGen"`
	Dashboard    DashboardConfig `json:"dashboard"`
	Pairing      PairingConfig      `json:"pairing"`                // DM-Pairing Mode (P9)
	Integrations []Integration      `json:"integrations,omitempty"` // externe API-Keys für Skills
	SkillSecret  string             `json:"skillSecret,omitempty"`  // HMAC-Key für Skill-Signierung
	Security     SecurityConfig     `json:"security"`               // vt-go Implementierung
	BrowserSkills BrowserSkillsConfig `json:"browserSkills,omitempty"` // Session 42: Web-Suche + Browser CDP
	DangerousTools DangerousToolsConfig `json:"dangerousTools,omitempty"` // P11: Dangerous-Tools Whitelist
}

// DangerousToolsConfig konfiguriert die Dangerous-Tools Whitelist (P11).
// Enabled: wenn true, werden User-Prompts auf gefährliche Operationen geprüft.
// AdminIDs: User-IDs die alle Operationen nutzen dürfen (Format: "123456" oder "telegram:123456").
// Blocked: Liste der gesperrten Kategorien (leer = alles erlaubt).
type DangerousToolsConfig struct {
	Enabled  bool     `json:"enabled"`            // true = Prüfung aktiv (Default: true)
	AdminIDs []string `json:"adminIDs,omitempty"` // Bypass-Liste für Admins
	Blocked  []string `json:"blocked,omitempty"`  // Kategorien: system.run, file.delete, file.modify, code.eval, network.unrestricted
}

// BrowserSkillsConfig konfiguriert die Browser Skills (Playwright).
// Keys werden aus dem Vault geladen (SEARCH_API_KEY, BROWSER_TYPE, BROWSER_ALLOWED_DOMAINS).
type BrowserSkillsConfig struct {
	SearchAPIKey    string `json:"searchAPIKey,omitempty"`    // Tavily API-Key (SEARCH_API_KEY im Vault)
	BrowserEndpoint string `json:"browserEndpoint,omitempty"` // Deprecated – wird ignoriert (Playwright verwaltet Browser selbst)
	BrowserType     string `json:"browserType,omitempty"`     // Browser-Engine: "chromium" (default), "firefox", "webkit"
	AllowedDomains  string `json:"allowedDomains,omitempty"`  // Kommagetrennte Domain-Whitelist (leer = alle)
}

// PairingConfig konfiguriert den DM-Pairing Mode (P9).
// Wenn Enabled=true, müssen unbekannte DM-Sender erst gepairt werden.
// AllowFrom (pro Channel) hat Vorrang – dort eingetragene IDs sind IMMER erlaubt.
type PairingConfig struct {
	Enabled bool   `json:"enabled"` // true = Pairing aktiv (unbekannte DMs werden blockiert)
	Message string `json:"message"` // Custom-Nachricht für ungepaarte User (leer = Default)
}

// Integration speichert einen benannten API-Key für externe Dienste.
// Skills referenzieren ihn als {{NAME}} Platzhalter.
type Integration struct {
	Name        string `json:"name"`        // Platzhalter-Name, z.B. "CAL_API_KEY"
	Description string `json:"description"` // Lesbare Beschreibung, z.B. "Cal.com API Key"
	Value       string `json:"value"`       // Der eigentliche Key / Token
}

// DashboardConfig konfiguriert das Web-Dashboard
type DashboardConfig struct {
	Enabled  bool   `json:"enabled"`
	Port     int    `json:"port"`     // default: 9090
	Password string `json:"password"` // leer = kein Passwortschutz
	Username string `json:"username"` // Benutzername, default: "admin"
}

// WebChatConfig konfiguriert den eingebetteten Web-Chat (P15).
// Erreichbar unter http://localhost:PORT/chat via WebSocket.
type WebChatConfig struct {
	Enabled bool `json:"enabled"` // true = Web-Chat aktiv (Default: true)
}

// ChannelsConfig enthält die Konfiguration aller Kommunikationskanäle
type ChannelsConfig struct {
	Telegram TelegramChannelConfig `json:"telegram"`
	Discord  DiscordChannelConfig  `json:"discord"`
	Slack    SlackChannelConfig    `json:"slack"`
	Matrix   MatrixChannelConfig   `json:"matrix"`
	WhatsApp WhatsAppChannelConfig `json:"whatsapp"`
	WebChat  WebChatConfig         `json:"webchat"`
}

// AccessMode steuert wer Nachrichten senden darf.
// "open"      – alle User erlaubt (kein Filter)
// "allowlist" – nur User aus AllowFrom erlaubt
// "pairing"   – unbekannte User müssen erst im Dashboard freigegeben werden
type AccessMode = string

const (
	AccessOpen      AccessMode = "open"
	AccessAllowlist AccessMode = "allowlist"
	AccessPairing   AccessMode = "pairing"
)

type TelegramChannelConfig struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom"`
	DMMode    string   `json:"dmMode"`    // "open" | "allowlist" | "pairing" (default: inherit from Pairing.Enabled)
	GroupMode string   `json:"groupMode"` // "open" | "allowlist" (default: "open")
}

type DiscordChannelConfig struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom"`
	DMMode    string   `json:"dmMode"`    // "open" | "allowlist" | "pairing"
	GroupMode string   `json:"groupMode"` // "open" | "allowlist"
}

type SlackChannelConfig struct {
	Enabled       bool     `json:"enabled"`
	BotToken      string   `json:"botToken"`
	AppToken      string   `json:"appToken"`
	SigningSecret string   `json:"signingSecret"` // Slack Signing Secret für HMAC
	WebhookPort   int      `json:"webhookPort"`   // Port für Events API (default: 3000)
	AllowFrom     []string `json:"allowFrom"`
	DMMode        string   `json:"dmMode"`    // "open" | "allowlist" | "pairing"
	GroupMode     string   `json:"groupMode"` // "open" | "allowlist"
}

type MatrixChannelConfig struct {
	Enabled    bool     `json:"enabled"`
	HomeServer string   `json:"homeServer"`
	UserID     string   `json:"userId"`
	Token      string   `json:"token"`
	AllowFrom  []string `json:"allowFrom"`
	DMMode     string   `json:"dmMode"`    // "open" | "allowlist" | "pairing"
	GroupMode  string   `json:"groupMode"` // "open" | "allowlist"
}

// WhatsAppChannelConfig – nutzt die WhatsApp Business Cloud API (Meta)
type WhatsAppChannelConfig struct {
	Enabled       bool     `json:"enabled"`
	Provider      string   `json:"provider"`      // "meta" (weitere folgen)
	PhoneNumber   string   `json:"phoneNumber"`   // Anzeige-Rufnummer
	PhoneNumberID string   `json:"phoneNumberId"` // Meta Phone Number ID (aus Developer Portal)
	APIKey        string   `json:"apiKey"`        // Meta Access Token (permanent)
	WebhookSecret string   `json:"webhookSecret"` // HMAC-Verifizierungsschlüssel
	WebhookPort   int      `json:"webhookPort"`   // Port für Webhook-Server (default: 8443)
	AllowFrom     []string `json:"allowFrom"`     // Erlaubte Rufnummern (leer = alle)
	DMMode        string   `json:"dmMode"`        // "open" | "allowlist" | "pairing"
	GroupMode     string   `json:"groupMode"`     // "open" | "allowlist"
}

// ProvidersConfig enthält alle KI-Provider-Konfigurationen.
// Default legt fest welcher Provider aktiv ist (z.B. "openrouter", "openai", "anthropic", ...).
// Für jeden bekannten Provider gibt es ein benanntes Feld.
// Für alle anderen Anbieter steht "custom" bereit.
type ProvidersConfig struct {
	Default string `json:"default"`

	// ── Empfohlen ─────────────────────────────────────────────────────────────
	OpenRouter OpenRouterConfig `json:"openrouter"` // openrouter.ai  (speziell: hat Models-Map)

	// ── Führende Anbieter ─────────────────────────────────────────────────────
	Anthropic  GenericAPIConfig `json:"anthropic"`  // api.anthropic.com
	OpenAI     GenericAPIConfig `json:"openai"`     // api.openai.com
	Google     GenericAPIConfig `json:"google"`     // Gemini – generativelanguage.googleapis.com
	XAI        GenericAPIConfig `json:"xai"`        // Grok – api.x.ai
	Groq       GenericAPIConfig `json:"groq"`       // api.groq.com (kostenloser Tier)
	Mistral    GenericAPIConfig `json:"mistral"`    // api.mistral.ai
	Together   GenericAPIConfig `json:"together"`   // api.together.xyz
	DeepSeek   GenericAPIConfig `json:"deepseek"`   // api.deepseek.com
	Perplexity GenericAPIConfig `json:"perplexity"` // api.perplexity.ai
	Cohere     GenericAPIConfig `json:"cohere"`     // api.cohere.com
	Fireworks  GenericAPIConfig `json:"fireworks"`  // api.fireworks.ai
	Ollama     GenericAPIConfig `json:"ollama"`     // lokal – kein API-Key nötig

	// ── Benutzerdefiniert ─────────────────────────────────────────────────────
	// Für jeden Anbieter der oben nicht aufgelistet ist.
	// Jede OpenAI-kompatible API funktioniert.
	Custom CustomProviderConfig `json:"custom"`
}

// OpenRouterConfig konfiguriert den OpenRouter-Provider (speziell: enthält Models-Map)
type OpenRouterConfig struct {
	APIKey string            `json:"apiKey"`
	Models map[string]string `json:"models"`
}

// GenericAPIConfig konfiguriert einen generischen OpenAI-kompatiblen Provider.
// Der API-Key reicht – die Base-URL ist in FluxBot hinterlegt.
type GenericAPIConfig struct {
	APIKey string            `json:"apiKey"`
	Models map[string]string `json:"models,omitempty"`
}

// CustomProviderConfig für beliebige OpenAI-kompatible APIs.
// Einfach Name, API-Key und Base-URL eintragen.
type CustomProviderConfig struct {
	Name    string            `json:"name"`
	APIKey  string            `json:"apiKey"`
	BaseURL string            `json:"baseUrl"`
	Models  map[string]string `json:"models,omitempty"`
}

// WorkspaceConfig legt den Pfad zum Workspace-Verzeichnis fest
type WorkspaceConfig struct {
	Path string `json:"path"`
}

// VoiceConfig konfiguriert die Sprachverarbeitung (optional)
type VoiceConfig struct {
	// ── STT (Speech-to-Text / Transkription) ──────────────────────────────────
	Enabled   bool   `json:"enabled"`
	Provider  string `json:"provider"` // groq, openai, ollama
	APIKey    string `json:"apiKey"`
	Language  string `json:"language"`  // ISO-639-1 Code, z.B. "de" – leer = auto
	OllamaURL string `json:"ollamaUrl"` // Ollama API URL, z.B. http://ollama:11434

	// ── TTS (Text-to-Speech / Sprachausgabe) ─────────────────────────────────
	TTSEnabled  bool   `json:"ttsEnabled"`            // false = nur STT (Default)
	TTSProvider string `json:"ttsProvider,omitempty"` // "openai", "google", "azure"
	TTSAPIKey   string `json:"ttsApiKey,omitempty"`   // leer = verwendet apiKey (STT-Key)
	// TTSVoice: Provider-spezifischer Stimmen-Name
	//   OpenAI:  "alloy","nova","echo","fable","onyx","shimmer"
	//   Google:  "de-DE-Neural2-C","de-DE-Neural2-F","de-DE-Wavenet-C","de-DE-Standard-C"
	//   Azure:   "de-AT-IngridNeural","de-AT-JonasNeural","de-DE-KatjaNeural","de-DE-ConradNeural"
	TTSVoice       string `json:"ttsVoice,omitempty"`
	TTSAzureRegion string `json:"ttsAzureRegion,omitempty"` // Azure: Region z.B. "westeurope" (nur für Provider "azure")
	// TTSMode steuert wann TTS ausgelöst wird:
	// "voice"   = nur wenn User eine Sprachnachricht schickt (Default)
	// "always"  = immer (alle Antworten als Voice)
	// "keyword" = nur bei Präfix "sprich:" oder "/voice"
	TTSMode string `json:"ttsMode,omitempty"`
}

// ImageGenConfig konfiguriert die Bild-Generierung.
// Default legt den aktiven Provider fest. Jeder Provider hat seinen eigenen API-Key.
// Wird kein Default gesetzt, wird der erste Provider mit einem Key automatisch gewählt.
type ImageGenConfig struct {
	Default string `json:"default"` // "openrouter","fal","openai","stability","together","replicate","" = aus

	// ── Provider ──────────────────────────────────────────────────────────────
	OpenRouter ImageGenProviderConfig `json:"openrouter"` // FLUX.2 Pro, Seedream, 50+ Modelle
	Fal        ImageGenProviderConfig `json:"fal"`        // Flux Pro Ultra, SDXL, ...
	OpenAI     ImageGenProviderConfig `json:"openai"`     // DALL-E 3
	Stability  ImageGenProviderConfig `json:"stability"`  // Stable Diffusion 3.5
	Together   ImageGenProviderConfig `json:"together"`   // Flux, SDXL über Together AI
	Replicate  ImageGenProviderConfig `json:"replicate"`  // Tausende Modelle

	// ── Darstellung ───────────────────────────────────────────────────────────
	Size    string `json:"size"`    // "square", "landscape", "portrait"
	Quality string `json:"quality"` // DALL-E: "standard" oder "hd"
}

// ImageGenProviderConfig enthält API-Key und konfigurierbare Modell-Liste für einen Bild-Provider.
// Models: Liste der angebotenen Modell-Slugs. Leer = Provider-Defaults werden verwendet.
type ImageGenProviderConfig struct {
	APIKey string   `json:"apiKey"`
	Models []string `json:"models,omitempty"` // leer = Provider-Defaults
}

// VideoGenConfig konfiguriert die Video-Generierung.
// Default legt den aktiven Provider fest.
type VideoGenConfig struct {
	Default string `json:"default"` // "runway","kling","luma","pika","hailuo","sora","veo","" = aus

	// ── Provider ──────────────────────────────────────────────────────────────
	Runway VideoGenProviderConfig `json:"runway"` // Runway Gen-4
	Kling  VideoGenProviderConfig `json:"kling"`  // Kling AI 2.0
	Luma   VideoGenProviderConfig `json:"luma"`   // Luma Dream Machine
	Pika   VideoGenProviderConfig `json:"pika"`   // Pika Labs
	Hailuo VideoGenProviderConfig `json:"hailuo"` // HailuoAI (Minimax)
	Sora   VideoGenProviderConfig `json:"sora"`   // OpenAI Sora
	Veo    VideoGenProviderConfig `json:"veo"`    // Google Veo 3

	// ── Parameter ─────────────────────────────────────────────────────────────
	Duration    int    `json:"duration"`    // Videolänge in Sekunden (default: 5)
	AspectRatio string `json:"aspectRatio"` // "16:9", "9:16", "1:1"
	Quality     string `json:"quality"`     // "standard", "hd"
}

// VideoGenProviderConfig enthält API-Key und optionales Modell für einen Video-Provider.
type VideoGenProviderConfig struct {
	APIKey string `json:"apiKey"`
	Model  string `json:"model,omitempty"` // leer = Provider-Default
}

// Load lädt die Konfiguration aus einer JSON-Datei
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("konfigurationsdatei '%s' nicht gefunden: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("fehler beim Parsen der Konfiguration: %w", err)
	}

	if cfg.Providers.Default == "" {
		cfg.Providers.Default = "openrouter"
	}

	if len(cfg.Providers.OpenRouter.Models) == 0 {
		cfg.Providers.OpenRouter.Models = DefaultModels()
	}
	// "image"-Key immer sicherstellen – auch wenn config.json die Map nur teilweise definiert
	if cfg.Providers.OpenRouter.Models["image"] == "" {
		cfg.Providers.OpenRouter.Models["image"] = "google/gemini-2.0-flash-001"
	}

	if len(cfg.Providers.OpenAI.Models) == 0 {
		cfg.Providers.OpenAI.Models = map[string]string{
			"default": "gpt-4o",
			"opus":    "gpt-4o",
			"image":   "gpt-4o",
			"search":  "gpt-4o",
			"ocr":     "gpt-4o",
		}
	}

	if len(cfg.Providers.Anthropic.Models) == 0 {
		cfg.Providers.Anthropic.Models = map[string]string{
			"default": "claude-sonnet-4-5-20250929",
			"opus":    "claude-opus-4-5-20251101",
			"image":   "claude-sonnet-4-5-20250929",
			"search":  "claude-sonnet-4-5-20250929",
			"ocr":     "claude-sonnet-4-5-20250929",
		}
	}

	if len(cfg.Providers.Groq.Models) == 0 {
		cfg.Providers.Groq.Models = map[string]string{
			"default": "llama-3.3-70b-versatile",
			"opus":    "llama-3.3-70b-versatile",
			"image":   "llama-3.3-70b-versatile",
			"search":  "llama-3.3-70b-versatile",
			"ocr":     "llama-3.3-70b-versatile",
		}
	}

	// Ollama Standard-Modelle (lokal, kein API-Key nötig)
	// Nutzer kann im Dashboard überschreiben – hier nur Sinnvoll-Defaults
	if len(cfg.Providers.Ollama.Models) == 0 {
		cfg.Providers.Ollama.Models = map[string]string{
			"default": "llama3.2",
			"opus":    "llama3.2:latest",
			"image":   "llama3.2",
			"search":  "llama3.2",
			"ocr":     "llama3.2-vision:latest",
		}
	}

	if cfg.Workspace.Path == "" {
		cfg.Workspace.Path = "./workspace"
	}

	if cfg.Voice.Language == "" {
		cfg.Voice.Language = "de"
	}

	if cfg.Voice.OllamaURL == "" {
		cfg.Voice.OllamaURL = "http://ollama:11434"
	}

	// TTS Defaults
	if cfg.Voice.TTSProvider == "" {
		cfg.Voice.TTSProvider = "openai"
	}
	if cfg.Voice.TTSMode == "" {
		cfg.Voice.TTSMode = "voice" // nur bei eingehenden Sprachnachrichten antworten
	}
	if cfg.Voice.TTSAzureRegion == "" {
		cfg.Voice.TTSAzureRegion = "westeurope"
	}
	// TTSVoice Default ist provider-abhängig und wird in main.go gesetzt

	// ── ImageGen Defaults ───────────────────────────────────────────────────
	if cfg.ImageGen.Size == "" {
		cfg.ImageGen.Size = "landscape"
	}
	if cfg.ImageGen.Quality == "" {
		cfg.ImageGen.Quality = "standard"
	}
	// Standard-Modelle je Provider (werden nur gesetzt wenn Nutzer keine eigene Liste hat)
	if len(cfg.ImageGen.OpenRouter.Models) == 0 {
		cfg.ImageGen.OpenRouter.Models = []string{
			"black-forest-labs/flux-2-pro",
			"bytedance-seed/seedream-4.5",
			"black-forest-labs/flux-1-schnell:free",
		}
	}
	if len(cfg.ImageGen.Fal.Models) == 0 {
		cfg.ImageGen.Fal.Models = []string{
			"fal-ai/flux-pro/v1.1-ultra",
		}
	}
	if len(cfg.ImageGen.Stability.Models) == 0 {
		cfg.ImageGen.Stability.Models = []string{
			"stability-ai/stable-diffusion-3.5-large",
		}
	}
	if len(cfg.ImageGen.Together.Models) == 0 {
		cfg.ImageGen.Together.Models = []string{
			"black-forest-labs/FLUX.1-schnell-Free",
		}
	}
	if len(cfg.ImageGen.Replicate.Models) == 0 {
		cfg.ImageGen.Replicate.Models = []string{
			"black-forest-labs/flux-1.1-pro",
		}
	}

	// ── VideoGen Defaults ───────────────────────────────────────────────────
	if cfg.VideoGen.Duration == 0 {
		cfg.VideoGen.Duration = 5
	}
	if cfg.VideoGen.AspectRatio == "" {
		cfg.VideoGen.AspectRatio = "16:9"
	}
	if cfg.VideoGen.Quality == "" {
		cfg.VideoGen.Quality = "standard"
	}

	if cfg.Channels.WhatsApp.WebhookPort == 0 {
		cfg.Channels.WhatsApp.WebhookPort = 8443
	}

	if cfg.Dashboard.Port == 0 {
		cfg.Dashboard.Port = 9090
	}

	// ── DangerousTools Defaults ─────────────────────────────────────────────
	// Enabled=true per Default (JSON bool defaults to false, daher explizit setzen)
	// Wenn Blocked leer ist → alle Kategorien sperren
	if !cfg.DangerousTools.Enabled && len(cfg.DangerousTools.Blocked) == 0 {
		cfg.DangerousTools.Enabled = true
		cfg.DangerousTools.Blocked = []string{
			"system.run", "file.delete", "file.modify", "code.eval", "network.unrestricted",
		}
	}

	// ── Skill-Secret auto-generieren wenn noch nicht vorhanden ──────────────
	if cfg.SkillSecret == "" {
		cfg.SkillSecret = generateSecret()
		// Secret direkt in config.json persistieren
		if updated, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			_ = os.WriteFile(path, updated, 0600)
		}
	}

	return &cfg, nil
}

// generateSecret erzeugt einen kryptographisch sicheren 32-Byte Hex-String.
func generateSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand nicht verfügbar: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// DefaultModels gibt die Standard-Modellzuordnung zurück
func DefaultModels() map[string]string {
	return map[string]string{
		"default": "anthropic/claude-sonnet-4-5-20250929",
		"opus":    "anthropic/claude-opus-4-5-20251101",
		"search":  "perplexity/sonar-reasoning-pro",
		"ocr":     "mistralai/pixtral-12b",
		"image":   "google/gemini-2.0-flash-001",
	}
}

// SecurityConfig definiert die Einstellungen für Laufzeit-Scans (vt-go)
type SecurityConfig struct {
	VirusTotalAPIKey string `json:"virustotal_api_key"`
	ScanUploads      bool   `json:"scan_uploads"`
	ActionOnUnknown  string `json:"action_on_unknown"` // "block", "allow", "analyze"
}
