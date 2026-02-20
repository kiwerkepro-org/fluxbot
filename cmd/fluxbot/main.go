package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ki-werke/fluxbot/pkg/agent"
	"github.com/ki-werke/fluxbot/pkg/channels"
	"github.com/ki-werke/fluxbot/pkg/config"
	"github.com/ki-werke/fluxbot/pkg/dashboard"
	"github.com/ki-werke/fluxbot/pkg/imagegen"
	"github.com/ki-werke/fluxbot/pkg/provider"
	"github.com/ki-werke/fluxbot/pkg/security"
	"github.com/ki-werke/fluxbot/pkg/setup"
	"github.com/ki-werke/fluxbot/pkg/skills"
	"github.com/ki-werke/fluxbot/pkg/voice"
)

// version wird beim Build via -ldflags gesetzt:
//
//	go build -ldflags="-X main.version=v1.2.3"
//
// Ohne Flag gilt "dev" als Standardwert.
var version = "dev"

func main() {
	configPath := flag.String("config", "./workspace/config.json", "Pfad zur Konfigurationsdatei")
	debug := flag.Bool("debug", false, "Debug-Logging aktivieren")
	service := flag.String("service", "", "Service-Modus: install | uninstall | run")
	flag.Parse()

	if *debug {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	// ── Service-Befehle (install / uninstall) ────────────────────────────────
	switch *service {
	case "install":
		exe, err := os.Executable()
		if err != nil {
			log.Fatalf("[Service] Executable-Pfad nicht ermittelbar: %v", err)
		}
		if err := installService(exe, *configPath); err != nil {
			log.Fatalf("[Service] Installation fehlgeschlagen: %v", err)
		}
		return

	case "uninstall":
		if err := uninstallService(); err != nil {
			log.Fatalf("[Service] Deinstallation fehlgeschlagen: %v", err)
		}
		return
	}

	// ── Windows Service Modus (vom SCM gestartet oder --service run) ─────────
	if *service == "run" || isWindowsService() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runAsWindowsService(ctx, cancel, *configPath)
		return
	}

	// ── Normaler Console-Modus ───────────────────────────────────────────────
	printBanner()

	// ── Setup-Wizard wenn keine config.json vorhanden ─────────────────────────
	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		log.Println("[Main] Keine config.json gefunden – Einrichtungsassistent wird gestartet...")
		if err := setup.RunWizard(*configPath); err != nil {
			log.Fatalf("[Main] Einrichtung fehlgeschlagen: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("[Main] Signal empfangen: %v – fahre herunter...", sig)
		cancel()
	}()

	runBot(ctx, *configPath)
	log.Println("[Main] FluxBot beendet. Tschüss!")
}

// printBanner gibt das ASCII-Banner aus.
func printBanner() {
	log.Println("╔══════════════════════════════════════╗")
	log.Printf( "║  FluxBot %-28s║", version+"  ")
	log.Println("║  KI-WERKE | github.com/ki-werke      ║")
	log.Println("╚══════════════════════════════════════╝")
}

// runBot initialisiert alle Komponenten und startet FluxBot.
// Blockiert bis ctx abgebrochen wird.
func runBot(ctx context.Context, configPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("[Main] Konfigurationsfehler: %v\n  → Kopiere workspace/config.example.json nach workspace/config.json", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("%v", err)
	}

	if err := os.MkdirAll(cfg.Workspace.Path, 0755); err != nil {
		log.Fatalf("[Main] Workspace-Verzeichnis konnte nicht erstellt werden: %v", err)
	}

	// ── Terminal-Log in Datei schreiben (stdout + fluxbot.log) ───────────────
	logsDir := filepath.Join(cfg.Workspace.Path, "logs")
	if err := os.MkdirAll(logsDir, 0755); err == nil {
		logPath := filepath.Join(logsDir, "fluxbot.log")
		if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
			log.SetOutput(io.MultiWriter(os.Stdout, logFile))
			log.Printf("[Main] Terminal-Log: %s", logPath)
		}
	}

	// SOUL.md + IDENTITY.md laden (Persönlichkeit von Fluxy)
	soul := loadSoul(cfg.Workspace.Path)

	// ── AI-Provider ──────────────────────────────────────────────────────────
	// Base-URLs für alle bekannten OpenAI-kompatiblen Anbieter.
	// Anthropic wird separat behandelt (anderes API-Format).
	providerBaseURLs := map[string]string{
		"openai":      "https://api.openai.com/v1/chat/completions",
		"google":      "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
		"xai":         "https://api.x.ai/v1/chat/completions",
		"groq":        "https://api.groq.com/openai/v1/chat/completions",
		"mistral":     "https://api.mistral.ai/v1/chat/completions",
		"together":    "https://api.together.xyz/v1/chat/completions",
		"deepseek":    "https://api.deepseek.com/chat/completions",
		"perplexity":  "https://api.perplexity.ai/chat/completions",
		"cohere":      "https://api.cohere.com/compatibility/v1/chat/completions",
		"fireworks":   "https://api.fireworks.ai/inference/v1/chat/completions",
		"novita":      "https://api.novita.ai/v3/openai/chat/completions",
		"deepinfra":   "https://api.deepinfra.com/v1/openai/chat/completions",
		"cerebras":    "https://api.cerebras.ai/v1/chat/completions",
		"lepton":      "https://api.lepton.ai/api/v1/chat/completions",
		"anyscale":    "https://api.endpoints.anyscale.com/v1/chat/completions",
		"replicate":   "https://openai-compat.replicate.com/v1/chat/completions",
		"ollama":      "http://localhost:11434/v1/chat/completions",
	}

	var aiProvider provider.Provider
	p := cfg.Providers.Default

	switch p {
	case "openrouter", "":
		aiProvider = provider.NewOpenRouter(cfg.Providers.OpenRouter.APIKey)
	case "anthropic":
		aiProvider = provider.NewAnthropic(cfg.Providers.Anthropic.APIKey)
	case "custom":
		aiProvider = provider.NewOpenAICompat(
			cfg.Providers.Custom.Name,
			cfg.Providers.Custom.APIKey,
			cfg.Providers.Custom.BaseURL,
		)
	default:
		// Alle anderen bekannten Anbieter nutzen die gleiche OpenAI-kompatible API
		baseURL, known := providerBaseURLs[p]
		if !known {
			log.Fatalf("[Main] Unbekannter Provider '%s'. Nutze 'custom' mit baseUrl für eigene Endpunkte.", p)
		}
		apiKey := getProviderAPIKey(cfg, p)
		aiProvider = provider.NewOpenAICompat(p, apiKey, baseURL)
	}
	log.Printf("[Main] AI-Provider: %s", aiProvider.Name())

	// ── Voice (Spracherkennung) ───────────────────────────────────────────────
	var transcriber voice.Transcriber
	if cfg.Voice.Enabled {
		switch cfg.Voice.Provider {
		case "groq":
			transcriber = voice.NewGroqTranscriber(cfg.Voice.APIKey)
			log.Printf("[Main] Voice: %s (Sprache: %s)", transcriber.Name(), cfg.Voice.Language)
		case "openai":
			transcriber = voice.NewOpenAITranscriber(cfg.Voice.APIKey)
			log.Printf("[Main] Voice: %s (Sprache: %s)", transcriber.Name(), cfg.Voice.Language)
		case "ollama":
			transcriber = voice.NewOllamaTranscriber(cfg.Voice.OllamaURL)
			log.Printf("[Main] Voice: %s (Sprache: %s)", transcriber.Name(), cfg.Voice.Language)
		}
	} else {
		log.Println("[Main] Voice: deaktiviert")
	}

	// ── Bild-Generatoren ─────────────────────────────────────────────────────
	var imageGenerators []imagegen.Generator
	imageGenerators = buildImageGenerators(cfg)
	if len(imageGenerators) == 0 {
		log.Println("[Main] Bild-Generierung: deaktiviert (kein Provider konfiguriert)")
	} else {
		names := make([]string, len(imageGenerators))
		for i, g := range imageGenerators {
			names[i] = g.Name()
		}
		log.Printf("[Main] Bild-Generierung: %s", strings.Join(names, ", "))
	}

	// ── Security ──────────────────────────────────────────────────────────────
	guard := security.NewGuard(security.GuardConfig{
		WorkspacePath:  cfg.Workspace.Path,
		MaxMsgPerMin:   30,
		BlockInjection: true,
	})
	log.Println("[Main] Security Guard: aktiv (Rate-Limit: 30/min, Injection-Block: ja)")
	go guard.CleanOldLogs(90)

	// ── Channel-Manager ───────────────────────────────────────────────────────
	manager := channels.NewManager(100)

	if cfg.Channels.Telegram.Enabled {
		tg := channels.NewTelegramChannel(channels.TelegramConfig{
			Token:     cfg.Channels.Telegram.Token,
			AllowFrom: cfg.Channels.Telegram.AllowFrom,
		})
		manager.Register(tg)
	}

	if cfg.Channels.Discord.Enabled {
		dc := channels.NewDiscordChannel(channels.DiscordConfig{
			Token:     cfg.Channels.Discord.Token,
			AllowFrom: cfg.Channels.Discord.AllowFrom,
		})
		manager.Register(dc)
	}

	if cfg.Channels.Slack.Enabled {
		slack := channels.NewSlackChannel(channels.SlackConfig{
			BotToken:      cfg.Channels.Slack.BotToken,
			AppToken:      cfg.Channels.Slack.AppToken,
			SigningSecret: cfg.Channels.Slack.SigningSecret,
			WebhookPort:   cfg.Channels.Slack.WebhookPort,
			AllowFrom:     cfg.Channels.Slack.AllowFrom,
		})
		manager.Register(slack)
	}

	if cfg.Channels.Matrix.Enabled {
		matrix := channels.NewMatrixChannel(channels.MatrixConfig{
			HomeServer: cfg.Channels.Matrix.HomeServer,
			UserID:     cfg.Channels.Matrix.UserID,
			Token:      cfg.Channels.Matrix.Token,
			AllowFrom:  cfg.Channels.Matrix.AllowFrom,
		})
		manager.Register(matrix)
	}

	if cfg.Channels.WhatsApp.Enabled {
		wa := channels.NewWhatsAppChannel(channels.WhatsAppConfig{
			Provider:      cfg.Channels.WhatsApp.Provider,
			PhoneNumber:   cfg.Channels.WhatsApp.PhoneNumber,
			PhoneNumberID: cfg.Channels.WhatsApp.PhoneNumberID,
			APIKey:        cfg.Channels.WhatsApp.APIKey,
			WebhookSecret: cfg.Channels.WhatsApp.WebhookSecret,
			WebhookPort:   cfg.Channels.WhatsApp.WebhookPort,
			AllowFrom:     cfg.Channels.WhatsApp.AllowFrom,
		})
		manager.Register(wa)
	}

	log.Printf("[Main] Aktive Kanäle: %s", strings.Join(manager.ActiveChannels(), ", "))

	// ── Agent starten ─────────────────────────────────────────────────────────
	skillsLoader := skills.NewLoader(cfg.Workspace.Path)
	sessionManager := agent.NewSessionManager(cfg.Workspace.Path)
	log.Printf("[Main] Workspace: %s", cfg.Workspace.Path)

	// Aktive Modell-Map je nach Provider
	activeModels := getProviderModels(cfg)

	fluxAgent := agent.New(agent.Config{
		Provider:        aiProvider,
		Manager:         manager,
		Sessions:        sessionManager,
		SkillsLoader:    skillsLoader,
		Models:          activeModels,
		Transcriber:     transcriber,
		VoiceLang:       cfg.Voice.Language,
		Guard:           guard,
		ImageGenerators: imageGenerators,
		ImageSize:       cfg.ImageGen.Size,
		Soul:            soul,
	})

	// ── Dashboard ─────────────────────────────────────────────────────────────
	if cfg.Dashboard.Enabled {
		logPath := filepath.Join(cfg.Workspace.Path, "logs", "fluxbot.log")
		// Reload-Callback: Config neu lesen + Image-Generators aktualisieren
		onReload := func() {
			newCfg, err := config.Load(configPath)
			if err != nil {
				log.Printf("[Main] Reload: Config-Fehler: %v", err)
				return
			}
			fluxAgent.UpdateImageGenerators(buildImageGenerators(newCfg))
			log.Printf("[Main] ✅ Config neu geladen – Bildgeneratoren aktualisiert.")
		}
		dash := dashboard.New(
			configPath,
			cfg.Workspace.Path,
			cfg.Dashboard.Password,
			cfg.Dashboard.Port,
			manager.ActiveChannels,
			logPath,
			onReload,
		)
		go dash.Start(ctx)
	}

	defer manager.Stop()

	if err := manager.Start(ctx); err != nil {
		log.Fatalf("[Main] Fehler beim Starten der Kanäle: %v", err)
	}

	log.Println("[Main] FluxBot läuft.")
	fluxAgent.Run(ctx)
}

// getProviderAPIKey liest den API-Key für einen bekannten Provider aus der Config.
func getProviderAPIKey(cfg *config.Config, p string) string {
	switch p {
	case "openai":
		return cfg.Providers.OpenAI.APIKey
	case "google":
		return cfg.Providers.Google.APIKey
	case "xai":
		return cfg.Providers.XAI.APIKey
	case "groq":
		return cfg.Providers.Groq.APIKey
	case "mistral":
		return cfg.Providers.Mistral.APIKey
	case "together":
		return cfg.Providers.Together.APIKey
	case "deepseek":
		return cfg.Providers.DeepSeek.APIKey
	case "perplexity":
		return cfg.Providers.Perplexity.APIKey
	case "cohere":
		return cfg.Providers.Cohere.APIKey
	case "fireworks":
		return cfg.Providers.Fireworks.APIKey
	case "ollama":
		return "" // lokal, kein Key nötig
	default:
		return cfg.Providers.Custom.APIKey
	}
}

// getProviderModels liefert die Modell-Map für den aktiven Provider.
// Gibt es keine explizite Konfiguration, werden sinnvolle Defaults zurückgegeben.
func getProviderModels(cfg *config.Config) map[string]string {
	p := cfg.Providers.Default

	// Explizit konfigurierte Models bevorzugen
	var models map[string]string
	switch p {
	case "openrouter", "":
		models = cfg.Providers.OpenRouter.Models
	case "anthropic":
		models = cfg.Providers.Anthropic.Models
	case "openai":
		models = cfg.Providers.OpenAI.Models
	case "google":
		models = cfg.Providers.Google.Models
	case "xai":
		models = cfg.Providers.XAI.Models
	case "groq":
		models = cfg.Providers.Groq.Models
	case "mistral":
		models = cfg.Providers.Mistral.Models
	case "together":
		models = cfg.Providers.Together.Models
	case "deepseek":
		models = cfg.Providers.DeepSeek.Models
	case "perplexity":
		models = cfg.Providers.Perplexity.Models
	case "cohere":
		models = cfg.Providers.Cohere.Models
	case "fireworks":
		models = cfg.Providers.Fireworks.Models
	case "custom":
		models = cfg.Providers.Custom.Models
	}

	if len(models) > 0 {
		return models
	}

	// Fallback-Defaults je nach Provider
	defaults := map[string]map[string]string{
		"anthropic":  {"default": "claude-sonnet-4-5-20250929", "opus": "claude-opus-4-5-20251101"},
		"openai":     {"default": "gpt-4o", "opus": "gpt-4o"},
		"google":     {"default": "gemini-2.0-flash", "opus": "gemini-2.0-pro"},
		"xai":        {"default": "grok-2-latest", "opus": "grok-2-latest"},
		"groq":       {"default": "llama-3.3-70b-versatile", "opus": "llama-3.3-70b-versatile"},
		"mistral":    {"default": "mistral-large-latest", "opus": "mistral-large-latest"},
		"together":   {"default": "meta-llama/Llama-3.3-70B-Instruct-Turbo", "opus": "meta-llama/Llama-3.3-70B-Instruct-Turbo"},
		"deepseek":   {"default": "deepseek-chat", "opus": "deepseek-reasoner"},
		"perplexity": {"default": "sonar-pro", "opus": "sonar-reasoning-pro"},
		"cohere":     {"default": "command-r-plus", "opus": "command-r-plus"},
		"fireworks":  {"default": "accounts/fireworks/models/llama-v3p3-70b-instruct", "opus": "accounts/fireworks/models/llama-v3p3-70b-instruct"},
		"ollama":     {"default": "llama3.2", "opus": "llama3.2"},
	}

	if d, ok := defaults[p]; ok {
		return d
	}

	return config.DefaultModels()
}

// buildImageGenerators erstellt die Liste der aktiven Bild-Generatoren.
// Der aktive Provider wird aus cfg.ImageGen.Default bestimmt.
// Ist Default leer, wird der erste Provider mit gesetztem API-Key gewählt.
func buildImageGenerators(cfg *config.Config) []imagegen.Generator {
	ig := cfg.ImageGen

	// Explizit deaktiviert → sofort nil zurückgeben, kein Auto-Detect
	if ig.Default == "disabled" {
		return nil
	}

	// Auto-Detect wenn kein Default gesetzt (Erststart ohne Config)
	if ig.Default == "" {
		switch {
		case ig.OpenRouter.APIKey != "":
			ig.Default = "openrouter"
		case cfg.Providers.OpenRouter.APIKey != "":
			ig.Default = "openrouter-shared"
		case ig.Fal.APIKey != "":
			ig.Default = "fal"
		case ig.OpenAI.APIKey != "":
			ig.Default = "openai"
		case ig.Stability.APIKey != "":
			ig.Default = "stability"
		case ig.Together.APIKey != "":
			ig.Default = "together"
		case ig.Replicate.APIKey != "":
			ig.Default = "replicate"
		default:
			return nil
		}
	}

	switch ig.Default {
	case "openrouter":
		key := ig.OpenRouter.APIKey
		var gens []imagegen.Generator
		for _, m := range ig.OpenRouter.Models {
			gens = append(gens, imagegen.NewOpenRouterImageGenerator(key, m, m))
		}
		return gens
	case "openrouter-shared":
		// Gemeinsamen OpenRouter-Key aus LLM-Provider-Config verwenden
		key := cfg.Providers.OpenRouter.APIKey
		var gens []imagegen.Generator
		for _, m := range ig.OpenRouter.Models {
			gens = append(gens, imagegen.NewOpenRouterImageGenerator(key, m, m))
		}
		return gens
	case "fal":
		var gens []imagegen.Generator
		for _, m := range ig.Fal.Models {
			gens = append(gens, imagegen.NewFalGenerator(ig.Fal.APIKey, m))
		}
		return gens
	case "openai":
		return []imagegen.Generator{imagegen.NewOpenAIImageGenerator(ig.OpenAI.APIKey, ig.Quality)}
	case "stability":
		var gens []imagegen.Generator
		for _, m := range ig.Stability.Models {
			gens = append(gens, imagegen.NewFalGenerator(ig.Stability.APIKey, m))
		}
		return gens
	case "together":
		var gens []imagegen.Generator
		for _, m := range ig.Together.Models {
			gens = append(gens, imagegen.NewOpenRouterImageGenerator(ig.Together.APIKey, m, m))
		}
		return gens
	case "replicate":
		var gens []imagegen.Generator
		for _, m := range ig.Replicate.Models {
			gens = append(gens, imagegen.NewOpenRouterImageGenerator(ig.Replicate.APIKey, m, m))
		}
		return gens
	}
	return nil
}

// loadSoul lädt SOUL.md und IDENTITY.md aus dem Workspace und kombiniert beide.
func loadSoul(workspacePath string) string {
	files := []string{"SOUL.md", "IDENTITY.md"}
	var parts []string

	for _, f := range files {
		data, err := os.ReadFile(workspacePath + "/" + f)
		if err != nil {
			continue
		}
		parts = append(parts, string(data))
		log.Printf("[Main] Soul geladen: %s", f)
	}

	if len(parts) == 0 {
		log.Println("[Main] Soul: keine SOUL.md / IDENTITY.md gefunden – verwende Basis-Prompt")
		return ""
	}

	return strings.Join(parts, "\n\n---\n\n")
}
