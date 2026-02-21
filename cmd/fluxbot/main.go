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
	"github.com/ki-werke/fluxbot/pkg/email"
	"github.com/ki-werke/fluxbot/pkg/imagegen"
	"github.com/ki-werke/fluxbot/pkg/provider"
	"github.com/ki-werke/fluxbot/pkg/security"
	"github.com/ki-werke/fluxbot/pkg/setup"
	"github.com/ki-werke/fluxbot/pkg/skills"
	"github.com/ki-werke/fluxbot/pkg/voice"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "./workspace/config.json", "Pfad zur Konfigurationsdatei")
	debug := flag.Bool("debug", false, "Debug-Logging aktivieren")
	service := flag.String("service", "", "Service-Modus: install | uninstall | run")
	flag.Parse()

	if *debug {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

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

	if *service == "run" || isWindowsService() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		runAsWindowsService(ctx, cancel, *configPath)
		return
	}

	printBanner()

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

func printBanner() {
	log.Println("╔══════════════════════════════════════╗")
	log.Printf("║  FluxBot %-28s║", version+"  ")
	log.Println("║  KI-WERKE | github.com/ki-werke      ║")
	log.Println("╚══════════════════════════════════════╝")
}

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

	logsDir := filepath.Join(cfg.Workspace.Path, "logs")
	if err := os.MkdirAll(logsDir, 0755); err == nil {
		logPath := filepath.Join(logsDir, "fluxbot.log")
		if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
			log.SetOutput(io.MultiWriter(os.Stdout, logFile))
			log.Printf("[Main] Terminal-Log: %s", logPath)
		}
	}

	// ── VirusTotal Scanner initialisieren ────────────────────────────────────
	if cfg.Security.ScanUploads {
		if err := security.InitVT(cfg.Security.VirusTotalAPIKey); err != nil {
			log.Printf("[Main] Warnung: VirusTotal konnte nicht initialisiert werden: %v", err)
		} else {
			log.Println("[Main] Sicherheits-Modul (VirusTotal) erfolgreich gestartet")
		}
	}

	soul := loadSoul(cfg.Workspace.Path)

	providerBaseURLs := map[string]string{
		"openai":     "https://api.openai.com/v1/chat/completions",
		"google":     "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
		"xai":        "https://api.x.ai/v1/chat/completions",
		"groq":       "https://api.groq.com/openai/v1/chat/completions",
		"mistral":    "https://api.mistral.ai/v1/chat/completions",
		"together":   "https://api.together.xyz/v1/chat/completions",
		"deepseek":   "https://api.deepseek.com/chat/completions",
		"perplexity": "https://api.perplexity.ai/chat/completions",
		"cohere":     "https://api.cohere.com/compatibility/v1/chat/completions",
		"fireworks":  "https://api.fireworks.ai/inference/v1/chat/completions",
		"novita":     "https://api.novita.ai/v3/openai/chat/completions",
		"deepinfra":  "https://api.deepinfra.com/v1/openai/chat/completions",
		"cerebras":   "https://api.cerebras.ai/v1/chat/completions",
		"lepton":     "https://api.lepton.ai/api/v1/chat/completions",
		"anyscale":   "https://api.endpoints.anyscale.com/v1/chat/completions",
		"replicate":  "https://openai-compat.replicate.com/v1/chat/completions",
		"ollama":     "http://localhost:11434/v1/chat/completions",
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
		baseURL, known := providerBaseURLs[p]
		if !known {
			log.Fatalf("[Main] Unbekannter Provider '%s'.", p)
		}
		apiKey := getProviderAPIKey(cfg, p)
		aiProvider = provider.NewOpenAICompat(p, apiKey, baseURL)
	}
	log.Printf("[Main] AI-Provider: %s", aiProvider.Name())

	var transcriber voice.Transcriber
	if cfg.Voice.Enabled {
		switch cfg.Voice.Provider {
		case "groq":
			transcriber = voice.NewGroqTranscriber(cfg.Voice.APIKey)
		case "openai":
			transcriber = voice.NewOpenAITranscriber(cfg.Voice.APIKey)
		case "ollama":
			transcriber = voice.NewOllamaTranscriber(cfg.Voice.OllamaURL)
		}
		if transcriber != nil {
			log.Printf("[Main] Voice: %s (Sprache: %s)", transcriber.Name(), cfg.Voice.Language)
		}
	} else {
		log.Println("[Main] Voice: deaktiviert")
	}

	var imageGenerators []imagegen.Generator
	imageGenerators = buildImageGenerators(cfg)
	if len(imageGenerators) == 0 {
		log.Println("[Main] Bild-Generierung: deaktiviert")
	} else {
		names := make([]string, len(imageGenerators))
		for i, g := range imageGenerators {
			names[i] = g.Name()
		}
		log.Printf("[Main] Bild-Generierung: %s", strings.Join(names, ", "))
	}

	guard := security.NewGuard(security.GuardConfig{
		WorkspacePath:  cfg.Workspace.Path,
		MaxMsgPerMin:   30,
		BlockInjection: true,
	})
	log.Println("[Main] Security Guard: aktiv")
	go guard.CleanOldLogs(90)

	manager := channels.NewManager(100)
	if cfg.Channels.Telegram.Enabled {
		manager.Register(channels.NewTelegramChannel(channels.TelegramConfig{
			Token:     cfg.Channels.Telegram.Token,
			AllowFrom: cfg.Channels.Telegram.AllowFrom,
		}))
	}
	if cfg.Channels.Discord.Enabled {
		manager.Register(channels.NewDiscordChannel(channels.DiscordConfig{
			Token:     cfg.Channels.Discord.Token,
			AllowFrom: cfg.Channels.Discord.AllowFrom,
		}))
	}

	log.Printf("[Main] Aktive Kanäle: %s", strings.Join(manager.ActiveChannels(), ", "))

	emailSender := buildEmailSender(cfg)
	if emailSender != nil && emailSender.IsConfigured() {
		log.Printf("[Main] E-Mail-Versand: aktiv (Von: %s)", emailSender.From())
	}

	skillsLoader := skills.NewLoader(cfg.Workspace.Path)
	skillsLoader.SetSecret(cfg.SkillSecret)
	if len(cfg.Integrations) > 0 {
		integMap := make(map[string]string, len(cfg.Integrations))
		for _, integ := range cfg.Integrations {
			integMap[integ.Name] = integ.Value
		}
		skillsLoader.SetIntegrations(integMap)
		log.Printf("[Main] Integrationen: %d konfiguriert", len(cfg.Integrations))
	}

	sessionManager := agent.NewSessionManager(cfg.Workspace.Path)
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
		VideoDefault:    cfg.VideoGen.Default,
		EmailSender:     emailSender,
		Soul:            soul,
	})

	if cfg.Dashboard.Enabled {
		logPath := filepath.Join(cfg.Workspace.Path, "logs", "fluxbot.log")
		onReload := func() {
			newCfg, err := config.Load(configPath)
			if err != nil {
				return
			}
			fluxAgent.UpdateImageGenerators(buildImageGenerators(newCfg))
			fluxAgent.UpdateEmailSender(buildEmailSender(newCfg))
			log.Printf("[Main] ✅ Config neu geladen.")
		}
		dash := dashboard.New(configPath, cfg.Workspace.Path, cfg.Dashboard.Password, cfg.Dashboard.Port, manager.ActiveChannels, logPath, onReload)
		go dash.Start(ctx)
	}

	defer manager.Stop()
	if err := manager.Start(ctx); err != nil {
		log.Fatalf("[Main] Fehler beim Starten der Kanäle: %v", err)
	}

	log.Println("[Main] FluxBot läuft.")
	fluxAgent.Run(ctx)
}

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
	default:
		return cfg.Providers.Custom.APIKey
	}
}

func getProviderModels(cfg *config.Config) map[string]string {
	p := cfg.Providers.Default
	var models map[string]string
	switch p {
	case "openrouter":
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
	return config.DefaultModels()
}

func buildImageGenerators(cfg *config.Config) []imagegen.Generator {
	ig := cfg.ImageGen
	if ig.Default == "disabled" || ig.Default == "" {
		return nil
	}
	switch ig.Default {
	case "openai":
		return []imagegen.Generator{imagegen.NewOpenAIImageGenerator(ig.OpenAI.APIKey, ig.Quality)}
	}
	return nil
}

func buildEmailSender(cfg *config.Config) *email.Sender {
	integMap := make(map[string]string, len(cfg.Integrations))
	for _, integ := range cfg.Integrations {
		integMap[integ.Name] = integ.Value
	}
	host, user, pass := integMap["SMTP_HOST"], integMap["SMTP_USER"], integMap["SMTP_PASSWORD"]
	if host == "" || user == "" || pass == "" {
		return nil
	}
	return email.NewSender(host, integMap["SMTP_PORT"], user, pass, integMap["SMTP_FROM"])
}

func loadSoul(workspacePath string) string {
	files := []string{"SOUL.md", "IDENTITY.md"}
	var parts []string
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(workspacePath, f))
		if err != nil {
			continue
		}
		parts = append(parts, string(data))
		log.Printf("[Main] Soul geladen: %s", f)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func init() {
	hmacSecret := os.Getenv("FLUXBOT_HMAC_SECRET")
	if hmacSecret == "" {
		log.Println("[Security] HINWEIS: FLUXBOT_HMAC_SECRET nicht gesetzt.")
	} else {
		log.Println("[Security] ✅ FLUXBOT_HMAC_SECRET geladen.")
	}
}
