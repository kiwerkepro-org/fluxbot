package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config ist die Hauptkonfiguration von FluxBot (config.json)
type Config struct {
	Channels  ChannelsConfig  `json:"channels"`
	Providers ProvidersConfig `json:"providers"`
	Workspace WorkspaceConfig `json:"workspace"`
	Voice     VoiceConfig     `json:"voice"`
	ImageGen  ImageGenConfig  `json:"imageGen"`
	Dashboard DashboardConfig `json:"dashboard"`
}

// DashboardConfig konfiguriert das Web-Dashboard
type DashboardConfig struct {
	Enabled  bool   `json:"enabled"`
	Port     int    `json:"port"`     // default: 8080
	Password string `json:"password"` // leer = kein Passwortschutz
}

// ChannelsConfig enthält die Konfiguration aller Kommunikationskanäle
type ChannelsConfig struct {
	Telegram TelegramChannelConfig `json:"telegram"`
	Discord  DiscordChannelConfig  `json:"discord"`
	Slack    SlackChannelConfig    `json:"slack"`
	Matrix   MatrixChannelConfig   `json:"matrix"`
	WhatsApp WhatsAppChannelConfig `json:"whatsapp"`
}

type TelegramChannelConfig struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom"`
}

type DiscordChannelConfig struct {
	Enabled   bool     `json:"enabled"`
	Token     string   `json:"token"`
	AllowFrom []string `json:"allowFrom"`
}

type SlackChannelConfig struct {
	Enabled       bool     `json:"enabled"`
	BotToken      string   `json:"botToken"`
	AppToken      string   `json:"appToken"`
	SigningSecret string   `json:"signingSecret"` // Slack Signing Secret für HMAC
	WebhookPort   int      `json:"webhookPort"`   // Port für Events API (default: 3000)
	AllowFrom     []string `json:"allowFrom"`
}

type MatrixChannelConfig struct {
	Enabled    bool     `json:"enabled"`
	HomeServer string   `json:"homeServer"`
	UserID     string   `json:"userId"`
	Token      string   `json:"token"`
	AllowFrom  []string `json:"allowFrom"`
}

// WhatsAppChannelConfig – nutzt die WhatsApp Business Cloud API (Meta)
type WhatsAppChannelConfig struct {
	Enabled       bool     `json:"enabled"`
	Provider      string   `json:"provider"`       // "meta" (weitere folgen)
	PhoneNumber   string   `json:"phoneNumber"`    // Anzeige-Rufnummer
	PhoneNumberID string   `json:"phoneNumberId"`  // Meta Phone Number ID (aus Developer Portal)
	APIKey        string   `json:"apiKey"`         // Meta Access Token (permanent)
	WebhookSecret string   `json:"webhookSecret"`  // HMAC-Verifizierungsschlüssel
	WebhookPort   int      `json:"webhookPort"`    // Port für Webhook-Server (default: 8443)
	AllowFrom     []string `json:"allowFrom"`      // Erlaubte Rufnummern (leer = alle)
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
	Enabled   bool   `json:"enabled"`
	Provider  string `json:"provider"`  // groq, openai, ollama
	APIKey    string `json:"apiKey"`
	Language  string `json:"language"`  // ISO-639-1 Code, z.B. "de" – leer = auto
	OllamaURL string `json:"ollamaUrl"` // Ollama API URL, z.B. http://ollama:11434
}

// ImageGenConfig konfiguriert optionale kostenpflichtige Bild-Provider (fal.ai, DALL-E).
// OpenRouter-Modelle (FLUX.2 Pro, Seedream 4.5) sind automatisch aktiv wenn
// providers.openrouter.apiKey gesetzt ist – kein eigener Eintrag nötig.
type ImageGenConfig struct {
	Enabled  bool   `json:"enabled"`  // Nur für fal.ai / DALL-E
	Provider string `json:"provider"` // "fal" oder "openai"
	APIKey   string `json:"apiKey"`   // fal.ai oder OpenAI Key
	Model    string `json:"model"`    // Modell-ID, z.B. "fal-ai/flux-2/pro"
	Size     string `json:"size"`     // "square", "landscape", "portrait"
	Quality  string `json:"quality"`  // Nur DALL-E: "standard" oder "hd"
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

	if cfg.Workspace.Path == "" {
		cfg.Workspace.Path = "./workspace"
	}

	if cfg.Voice.Language == "" {
		cfg.Voice.Language = "de"
	}

	if cfg.Voice.OllamaURL == "" {
		cfg.Voice.OllamaURL = "http://ollama:11434"
	}

	if cfg.ImageGen.Size == "" {
		cfg.ImageGen.Size = "landscape"
	}
	if cfg.ImageGen.Quality == "" {
		cfg.ImageGen.Quality = "standard"
	}

	if cfg.Channels.WhatsApp.WebhookPort == 0 {
		cfg.Channels.WhatsApp.WebhookPort = 8443
	}

	if cfg.Dashboard.Port == 0 {
		cfg.Dashboard.Port = 8080
	}

	return &cfg, nil
}

// DefaultModels gibt die Standard-Modellzuordnung zurück
func DefaultModels() map[string]string {
	return map[string]string{
		"default": "anthropic/claude-sonnet-4-5-20250929",
		"opus":    "anthropic/claude-opus-4-5-20251101",
		"search":  "perplexity/sonar-reasoning-pro",
		"ocr":     "mistralai/pixtral-12b",
	}
}
