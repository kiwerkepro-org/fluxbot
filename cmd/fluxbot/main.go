package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/playwright-community/playwright-go"

	"github.com/ki-werke/fluxbot/pkg/agent"
	"github.com/ki-werke/fluxbot/pkg/browser"
	"github.com/ki-werke/fluxbot/pkg/channels"
	cronpkg "github.com/ki-werke/fluxbot/pkg/cron"
	"github.com/ki-werke/fluxbot/pkg/config"
	"github.com/ki-werke/fluxbot/pkg/dashboard"
	"github.com/ki-werke/fluxbot/pkg/email"
	googleapi "github.com/ki-werke/fluxbot/pkg/google"
	"github.com/ki-werke/fluxbot/pkg/imagegen"
	"github.com/ki-werke/fluxbot/pkg/pairing"
	"github.com/ki-werke/fluxbot/pkg/provider"
	searchpkg "github.com/ki-werke/fluxbot/pkg/search"
	"github.com/ki-werke/fluxbot/pkg/security"
	"github.com/ki-werke/fluxbot/pkg/setup"
	"github.com/ki-werke/fluxbot/pkg/skills"
	systempkg "github.com/ki-werke/fluxbot/pkg/system"
	"github.com/ki-werke/fluxbot/pkg/voice"
)

// version wird per -ldflags="-X main.version=vX.Y.Z" beim Build gesetzt.
// Lokal / ohne ldflags bleibt der Wert "dev".
var version = "v1.2.4"

func main() {
	configPath        := flag.String("config", "./workspace/config.json", "Pfad zur Konfigurationsdatei")
	debug             := flag.Bool("debug", false, "Debug-Logging aktivieren")
	service           := flag.String("service", "", "Service-Modus: install | uninstall | run")
	installPlaywright := flag.Bool("install-playwright", false, "Playwright-Browser installieren und beenden")
	flag.Parse()

	// Playwright-Browser-Installation (von Installer-Skripten aufgerufen)
	if *installPlaywright {
		log.Println("[Setup] Installiere Playwright-Browser (Chromium)...")
		pw, err := playwright.Run()
		if err != nil {
			log.Fatalf("[Setup] Playwright konnte nicht gestartet werden: %v", err)
		}
		log.Println("[Setup] ✅ Playwright-Browser erfolgreich installiert.")
		pw.Stop()
		return
	}

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

	// Vom Terminal lösen damit FluxBot nicht stirbt wenn das Konsolenfenster geschlossen wird.
	// Im Debug-Modus bleibt die Konsole verbunden (Entwickler sieht Ausgabe direkt).
	if !*debug {
		detachConsole()
	}

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

	runBot(ctx, *configPath, *debug)
	log.Println("[Main] FluxBot beendet. Tschüss!")
}

func printBanner() {
	log.Println("╔══════════════════════════════════════╗")
	log.Printf("║  FluxBot %-28s║", version+"  ")
	log.Println("║  KI-WERKE | github.com/ki-werke      ║")
	log.Println("╚══════════════════════════════════════╝")
}

func runBot(ctx context.Context, configPath string, debugMode bool) {
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("[Main] Konfigurationsfehler: %v\n  → Kopiere workspace/config.example.json nach workspace/config.json", err)
	}

	if err := os.MkdirAll(cfg.Workspace.Path, 0755); err != nil {
		log.Fatalf("[Main] Workspace-Verzeichnis konnte nicht erstellt werden: %v", err)
	}

	logsDir := filepath.Join(cfg.Workspace.Path, "logs")
	if err := os.MkdirAll(logsDir, 0755); err == nil {
		logPath := filepath.Join(logsDir, "fluxbot.log")
		if logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); err == nil {
			if debugMode {
				log.SetOutput(io.MultiWriter(os.Stdout, logFile))
			} else {
				// Nach detachConsole() ist stdout nicht mehr gültig → nur in Datei loggen
				log.SetOutput(logFile)
			}
			log.Printf("[Main] Terminal-Log: %s", logPath)
		}
	}

	// ── Secret-Provider initialisieren (Keyring → Vault → Fallback) ─────────
	// Lokal (Windows/macOS): ChainedProvider(KeyringProvider, VaultProvider)
	// Docker (Linux):        VaultProvider (AES-256-GCM Datei)
	vault, err := security.NewSecretProvider(cfg.Workspace.Path)
	if err != nil {
		log.Fatalf("[Main] Secret-Provider konnte nicht initialisiert werden: %v", err)
	}

	// ── Einmalige Migration: Secrets aus config.json → primären Provider ─────
	configSecrets := extractSecrets(cfg)
	if migrated, err := vault.MigrateFromConfig(configSecrets); err != nil {
		log.Printf("[Main] Vault-Migration fehlgeschlagen: %v", err)
	} else if migrated > 0 {
		log.Printf("[Main] ✅ %d Secrets aus config.json in den Vault migriert.", migrated)
		// Secrets aus config.json entfernen und Datei sauber speichern
		clearSecretsFromConfig(cfg)
		if data, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			if err := os.WriteFile(configPath, data, 0600); err != nil {
				log.Printf("[Main] Warnung: config.json konnte nicht bereinigt werden: %v", err)
			} else {
				log.Println("[Main] ✅ config.json bereinigt – keine Secrets mehr im Klartext.")
			}
		}
	}

	// ── Secrets aus Vault in Config laden (für Runtime) ───────────────────────
	applySecrets(cfg, vault)

	// ── Konfiguration validieren (NACH applySecrets – Secrets kommen aus Vault) ─
	if err := cfg.Validate(); err != nil {
		log.Fatalf("%v", err)
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
	}

	// ── Ollama Base-URL aus Vault laden (Default: localhost:11434) ────────────
	ollamaBaseURL := provider.OllamaDefaultBaseURL
	if v, err := vault.Get("OLLAMA_BASE_URL"); err == nil && v != "" {
		ollamaBaseURL = v
	}

	var aiProvider provider.Provider
	p := cfg.Providers.Default

	switch p {
	case "openrouter", "":
		aiProvider = provider.NewOpenRouter(cfg.Providers.OpenRouter.APIKey)
	case "anthropic":
		aiProvider = provider.NewAnthropic(cfg.Providers.Anthropic.APIKey)
	case "ollama":
		// Ping-Check: Warnung wenn Ollama nicht erreichbar – kein Absturz
		if err := provider.PingOllama(ollamaBaseURL); err != nil {
			log.Printf("[Main] ⚠️  Ollama-Warnung: %v", err)
			log.Printf("[Main]     FluxBot startet trotzdem – Anfragen schlagen fehl bis Ollama läuft.")
		} else {
			log.Printf("[Main] ✅  Ollama erreichbar unter %s", ollamaBaseURL)
		}
		aiProvider = provider.NewOllama(ollamaBaseURL, cfg.Providers.Ollama.APIKey)
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
			log.Printf("[Main] Voice STT: %s (Sprache: %s)", transcriber.Name(), cfg.Voice.Language)
		}
	} else {
		log.Println("[Main] Voice STT: deaktiviert")
	}

	// ── TTS (Text-to-Speech / Sprachausgabe) ────────────────────────────────
	// Aktivierungslogik: TTS startet automatisch sobald ein Key vorhanden ist.
	// ttsEnabled: false in config.json = explizites Deaktivieren (Override).
	ttsSpeaker, ttsVoiceEffective := buildTTSSpeaker(cfg, vault)
	cfg.Voice.TTSVoice = ttsVoiceEffective

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

	// ── Pairing-Store initialisieren (P9: DM-Pairing Mode) ───────────────────
	pairingStorePath := filepath.Join(cfg.Workspace.Path, "pairing.json")
	pairingStore := pairing.New(pairingStorePath)
	if cfg.Pairing.Enabled {
		stats := pairingStore.Stats()
		log.Printf("[Main] DM-Pairing Mode: aktiv (%d gepairt, %d pending, %d blockiert)",
			stats["approved"], stats["pending"], stats["blocked"])
	} else {
		log.Println("[Main] DM-Pairing Mode: deaktiviert")
	}

	manager := channels.NewManager(100)
	if cfg.Channels.Telegram.Enabled {
		manager.Register(channels.NewTelegramChannel(channels.TelegramConfig{
			Token:          cfg.Channels.Telegram.Token,
			AllowFrom:      cfg.Channels.Telegram.AllowFrom,
			DMMode:         cfg.Channels.Telegram.DMMode,
			GroupMode:      cfg.Channels.Telegram.GroupMode,
			PairingEnabled: cfg.Pairing.Enabled,
			PairingMessage: cfg.Pairing.Message,
		}, pairingStore))
	}
	if cfg.Channels.Discord.Enabled {
		manager.Register(channels.NewDiscordChannel(channels.DiscordConfig{
			Token:          cfg.Channels.Discord.Token,
			AllowFrom:      cfg.Channels.Discord.AllowFrom,
			DMMode:         cfg.Channels.Discord.DMMode,
			GroupMode:      cfg.Channels.Discord.GroupMode,
			PairingStore:   pairingStore,
			PairingMessage: cfg.Pairing.Message,
		}))
	}
	if cfg.Channels.Slack.Enabled {
		manager.Register(channels.NewSlackChannel(channels.SlackConfig{
			BotToken:       cfg.Channels.Slack.BotToken,
			SigningSecret:  cfg.Channels.Slack.SigningSecret,
			WebhookPort:    cfg.Channels.Slack.WebhookPort,
			AllowFrom:      cfg.Channels.Slack.AllowFrom,
			DMMode:         cfg.Channels.Slack.DMMode,
			GroupMode:      cfg.Channels.Slack.GroupMode,
			PairingStore:   pairingStore,
			PairingMessage: cfg.Pairing.Message,
		}))
	}
	if cfg.Channels.Matrix.Enabled {
		manager.Register(channels.NewMatrixChannel(channels.MatrixConfig{
			HomeServer:     cfg.Channels.Matrix.HomeServer,
			UserID:         cfg.Channels.Matrix.UserID,
			Token:          cfg.Channels.Matrix.Token,
			AllowFrom:      cfg.Channels.Matrix.AllowFrom,
			DMMode:         cfg.Channels.Matrix.DMMode,
			GroupMode:      cfg.Channels.Matrix.GroupMode,
			PairingStore:   pairingStore,
			PairingMessage: cfg.Pairing.Message,
		}))
	}
	if cfg.Channels.WhatsApp.Enabled {
		manager.Register(channels.NewWhatsAppChannel(channels.WhatsAppConfig{
			PhoneNumberID:  cfg.Channels.WhatsApp.PhoneNumberID,
			APIKey:         cfg.Channels.WhatsApp.APIKey,
			WebhookSecret:  cfg.Channels.WhatsApp.WebhookSecret,
			WebhookPort:    cfg.Channels.WhatsApp.WebhookPort,
			AllowFrom:      cfg.Channels.WhatsApp.AllowFrom,
			DMMode:         cfg.Channels.WhatsApp.DMMode,
			GroupMode:      cfg.Channels.WhatsApp.GroupMode,
			PairingStore:   pairingStore,
			PairingMessage: cfg.Pairing.Message,
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
		// Reload nötig: NewLoader() lädt Skills bevor SetIntegrations gesetzt ist.
		// Ohne Reload bleiben alle {{PLATZHALTER}} unsubstituiert bis zum ersten Dashboard-Save.
		skillsLoader.Reload()
		log.Printf("[Main] Integrationen: %d konfiguriert", len(cfg.Integrations))
	}

	sessionManager := agent.NewSessionManager(cfg.Workspace.Path)
	activeModels := getProviderModels(cfg)

	googleClient := buildGoogleClient(cfg, vault)

	// Session 42: Browser Skills – Search + Browser CDP initialisieren
	searchClient := buildSearchClient(cfg)
	browserClient := buildBrowserClient(cfg)

	// Cron-Manager: sendFn nutzt den Channel-Manager (nach dessen Start)
	cronStorePath := filepath.Join(cfg.Workspace.Path, "reminders.json")
	cronSendFn := func(channelID, chatID, text string) {
		if err := manager.SendTo(channelID, chatID, text); err != nil {
			log.Printf("[Cron] Fehler beim Senden der Erinnerung an %s/%s: %v", channelID, chatID, err)
		}
	}
	cronMgr := cronpkg.New(cronStorePath, cronSendFn)

	fluxAgent := agent.New(agent.Config{
		Provider:         aiProvider,
		Manager:          manager,
		Sessions:         sessionManager,
		SkillsLoader:     skillsLoader,
		Models:           activeModels,
		Transcriber:      transcriber,
		VoiceLang:        cfg.Voice.Language,
		Guard:            guard,
		ImageGenerators:  imageGenerators,
		ImageSize:        cfg.ImageGen.Size,
		VideoDefault:     cfg.VideoGen.Default,
		EmailSender:      emailSender,
		CalcomBaseURL:    getIntegration(cfg, "CALCOM_BASE_URL"),
		CalcomAPIKey:     getIntegration(cfg, "CALCOM_API_KEY"),
		CalcomOwnerEmail:  getIntegration(cfg, "CALCOM_OWNER_EMAIL"),
		CalcomEventTypeID: parseIntegrationInt(cfg, "CALCOM_EVENT_TYPE_ID"),
		GoogleClient:     googleClient,
		CronManager:      cronMgr,
		Soul:             soul,
		TTSSpeaker:       ttsSpeaker,
		TTSVoice:         cfg.Voice.TTSVoice,
		TTSMode:          cfg.Voice.TTSMode,
		SearchClient:     searchClient,
		BrowserClient:    browserClient,
		DangerousTools:   buildDangerousToolsCfg(cfg),
	})

	if cfg.Dashboard.Enabled {
		logPath := filepath.Join(cfg.Workspace.Path, "logs", "fluxbot.log")
		var dash *dashboard.Server

		// HMAC-Secret Ladereihenfolge:
		//  1. Secret-Provider (Keyring / Vault) – Schlüssel: "HMAC_SECRET"
		//  2. Umgebungsvariable FLUXBOT_HMAC_SECRET (CI/CD Fallback)
		dashHMACSecret := ""
		if v, err := vault.Get("HMAC_SECRET"); err == nil && v != "" {
			dashHMACSecret = v
			log.Printf("[Security] ✅ HMAC_SECRET aus %s geladen.", vault.Backend())
		} else if envSecret := os.Getenv("FLUXBOT_HMAC_SECRET"); envSecret != "" {
			dashHMACSecret = envSecret
			log.Println("[Security] ✅ HMAC_SECRET aus Umgebungsvariable geladen.")
		}

		onReload := func() {
			newCfg, err := config.Load(configPath)
			if err != nil {
				log.Printf("[Main] Reload: config.json konnte nicht geladen werden: %v", err)
				return
			}
			// Vault-Secrets in neue Config übernehmen
			applySecrets(newCfg, vault)

			fluxAgent.UpdateImageGenerators(buildImageGenerators(newCfg))
			fluxAgent.UpdateEmailSender(buildEmailSender(newCfg))
			fluxAgent.UpdateCalcomConfig(
				getIntegration(newCfg, "CALCOM_BASE_URL"),
				getIntegration(newCfg, "CALCOM_API_KEY"),
				getIntegration(newCfg, "CALCOM_OWNER_EMAIL"),
				parseIntegrationInt(newCfg, "CALCOM_EVENT_TYPE_ID"),
			)
			fluxAgent.UpdateGoogleClient(buildGoogleClient(newCfg, vault))

			// Session 42: Browser Skills Hot-Reload
			fluxAgent.UpdateSearchClient(buildSearchClient(newCfg))
			fluxAgent.UpdateBrowserClient(buildBrowserClient(newCfg))

			// P11: Dangerous-Tools Whitelist Hot-Reload
			fluxAgent.UpdateDangerousToolsConfig(buildDangerousToolsCfg(newCfg))

			// TTS Hot-Reload: aktiviert sich automatisch wenn VOICE_TTS_API_KEY gesetzt wird,
			// deaktiviert sich automatisch wenn Key entfernt wird – kein Neustart nötig.
			newTTSSpeaker, newTTSVoice := buildTTSSpeaker(newCfg, vault)
			fluxAgent.UpdateTTSSpeaker(newTTSSpeaker, newTTSVoice, newCfg.Voice.TTSMode)

			// Dashboard Hot-Reload: Passwort + Benutzername + HMAC-Secret
			if dash != nil {
				if pass, err := vault.Get("DASHBOARD_PASSWORD"); err == nil && pass != "" {
					dash.UpdatePassword(pass)
				}
				if user, err := vault.Get("DASHBOARD_USERNAME"); err == nil && user != "" {
					dash.UpdateUsername(user)
				}
				// HMAC-Secret bei Hot-Reload: Provider (Vault/Keyring) hat Vorrang vor Env
				if newHMAC, err := vault.Get("HMAC_SECRET"); err == nil && newHMAC != "" {
					dash.UpdateHMACSecret(newHMAC)
				} else if envHMAC := os.Getenv("FLUXBOT_HMAC_SECRET"); envHMAC != "" {
					dash.UpdateHMACSecret(envHMAC)
				}
			}

			// Integrationen + Skills neu laden (damit {{PLATZHALTER}} frisch substituiert werden)
			if len(newCfg.Integrations) > 0 {
				integMap := make(map[string]string, len(newCfg.Integrations))
				for _, integ := range newCfg.Integrations {
					if integ.Value != "" {
						integMap[integ.Name] = integ.Value
					}
				}
				skillsLoader.SetIntegrations(integMap)
			}
			skillsLoader.Reload()

			log.Printf("[Main] ✅ Config + Secrets + Skills neu geladen.")
		}

		// sendToChannel-Callback für Dashboard (z.B. Pairing-Bestätigung an User senden)
		sendToChannel := func(channel, chatID, text string) error {
			return manager.SendTo(channel, chatID, text)
		}

		dash = dashboard.New(
			configPath,
			cfg.Workspace.Path,
			cfg.Dashboard.Password,
			cfg.Dashboard.Username,
			cfg.Dashboard.Port,
			manager.ActiveChannels,
			logPath,
			vault,
			onReload,
			dashHMACSecret,
			skillsLoader,
			version,
			pairingStore,
			sendToChannel,
		)
		// Auto-Updater initialisieren und an Dashboard übergeben
		updater := systempkg.New(version)
		updater.StartBackgroundCheck(ctx)
		dash.SetUpdater(updater)

		go dash.Start(ctx)
	}

	defer manager.Stop()
	if err := manager.Start(ctx); err != nil {
		log.Fatalf("[Main] Fehler beim Starten der Kanäle: %v", err)
	}

	// Cron-Scheduler starten (nach Channel-Manager, damit SendFn funktioniert)
	cronMgr.Start()
	defer cronMgr.Stop()

	log.Println("[Main] FluxBot läuft.")
	fluxAgent.Run(ctx)
}

// ── Secret-Verwaltung ─────────────────────────────────────────────────────────

// extractSecrets baut die Migrations-Map aus der Config.
// Enthält alle sensitiven Felder mit ihren aktuellen Werten.
func extractSecrets(cfg *config.Config) map[string]string {
	m := make(map[string]string)
	// Kanäle
	m["TELEGRAM_TOKEN"] = cfg.Channels.Telegram.Token
	m["DISCORD_TOKEN"] = cfg.Channels.Discord.Token
	m["SLACK_BOT_TOKEN"] = cfg.Channels.Slack.BotToken
	m["SLACK_APP_TOKEN"] = cfg.Channels.Slack.AppToken
	m["SLACK_SIGNING_SECRET"] = cfg.Channels.Slack.SigningSecret
	m["MATRIX_TOKEN"] = cfg.Channels.Matrix.Token
	m["WHATSAPP_API_KEY"] = cfg.Channels.WhatsApp.APIKey
	m["WHATSAPP_WEBHOOK_SECRET"] = cfg.Channels.WhatsApp.WebhookSecret
	// AI Provider
	m["PROVIDER_OPENROUTER"] = cfg.Providers.OpenRouter.APIKey
	m["PROVIDER_ANTHROPIC"] = cfg.Providers.Anthropic.APIKey
	m["PROVIDER_OPENAI"] = cfg.Providers.OpenAI.APIKey
	m["PROVIDER_GOOGLE"] = cfg.Providers.Google.APIKey
	m["PROVIDER_XAI"] = cfg.Providers.XAI.APIKey
	m["PROVIDER_GROQ"] = cfg.Providers.Groq.APIKey
	m["PROVIDER_MISTRAL"] = cfg.Providers.Mistral.APIKey
	m["PROVIDER_TOGETHER"] = cfg.Providers.Together.APIKey
	m["PROVIDER_DEEPSEEK"] = cfg.Providers.DeepSeek.APIKey
	m["PROVIDER_PERPLEXITY"] = cfg.Providers.Perplexity.APIKey
	m["PROVIDER_COHERE"] = cfg.Providers.Cohere.APIKey
	m["PROVIDER_FIREWORKS"] = cfg.Providers.Fireworks.APIKey
	m["PROVIDER_OLLAMA"] = cfg.Providers.Ollama.APIKey // optional Bearer-Token (leer = kein Auth)
	m["PROVIDER_CUSTOM"] = cfg.Providers.Custom.APIKey
	// Voice
	m["VOICE_API_KEY"] = cfg.Voice.APIKey
	m["VOICE_TTS_API_KEY"] = cfg.Voice.TTSAPIKey
	// Bild-Generierung
	m["IMG_OPENROUTER"] = cfg.ImageGen.OpenRouter.APIKey
	m["IMG_FAL"] = cfg.ImageGen.Fal.APIKey
	m["IMG_OPENAI"] = cfg.ImageGen.OpenAI.APIKey
	m["IMG_STABILITY"] = cfg.ImageGen.Stability.APIKey
	m["IMG_TOGETHER"] = cfg.ImageGen.Together.APIKey
	m["IMG_REPLICATE"] = cfg.ImageGen.Replicate.APIKey
	// Video-Generierung
	m["VID_RUNWAY"] = cfg.VideoGen.Runway.APIKey
	m["VID_KLING"] = cfg.VideoGen.Kling.APIKey
	m["VID_LUMA"] = cfg.VideoGen.Luma.APIKey
	m["VID_PIKA"] = cfg.VideoGen.Pika.APIKey
	m["VID_HAILUO"] = cfg.VideoGen.Hailuo.APIKey
	m["VID_SORA"] = cfg.VideoGen.Sora.APIKey
	m["VID_VEO"] = cfg.VideoGen.Veo.APIKey
	// System
	m["SKILL_SECRET"] = cfg.SkillSecret
	m["VIRUSTOTAL_API_KEY"] = cfg.Security.VirusTotalAPIKey
	m["DASHBOARD_PASSWORD"] = cfg.Dashboard.Password
	m["DASHBOARD_USERNAME"] = cfg.Dashboard.Username
	// Integrationen (dynamisch)
	for _, integ := range cfg.Integrations {
		if integ.Value != "" {
			m["INTEG_"+integ.Name] = integ.Value
		}
	}
	return m
}

// applySecrets füllt sensitive Config-Felder aus dem Secret-Provider.
// Provider-Werte (Keyring / Vault) überschreiben config.json-Werte.
func applySecrets(cfg *config.Config, vault security.SecretProvider) {
	all, err := vault.GetAll()
	if err != nil || len(all) == 0 {
		return
	}
	get := func(key string) string { return all[key] }

	// Kanäle
	if v := get("TELEGRAM_TOKEN"); v != "" {
		cfg.Channels.Telegram.Token = v
	}
	if v := get("DISCORD_TOKEN"); v != "" {
		cfg.Channels.Discord.Token = v
	}
	if v := get("SLACK_BOT_TOKEN"); v != "" {
		cfg.Channels.Slack.BotToken = v
	}
	if v := get("SLACK_APP_TOKEN"); v != "" {
		cfg.Channels.Slack.AppToken = v
	}
	if v := get("SLACK_SIGNING_SECRET"); v != "" {
		cfg.Channels.Slack.SigningSecret = v
	}
	if v := get("MATRIX_TOKEN"); v != "" {
		cfg.Channels.Matrix.Token = v
	}
	if v := get("WHATSAPP_API_KEY"); v != "" {
		cfg.Channels.WhatsApp.APIKey = v
	}
	if v := get("WHATSAPP_WEBHOOK_SECRET"); v != "" {
		cfg.Channels.WhatsApp.WebhookSecret = v
	}
	// AI Provider
	if v := get("PROVIDER_OPENROUTER"); v != "" {
		cfg.Providers.OpenRouter.APIKey = v
	}
	if v := get("PROVIDER_ANTHROPIC"); v != "" {
		cfg.Providers.Anthropic.APIKey = v
	}
	if v := get("PROVIDER_OPENAI"); v != "" {
		cfg.Providers.OpenAI.APIKey = v
	}
	if v := get("PROVIDER_GOOGLE"); v != "" {
		cfg.Providers.Google.APIKey = v
	}
	if v := get("PROVIDER_XAI"); v != "" {
		cfg.Providers.XAI.APIKey = v
	}
	if v := get("PROVIDER_GROQ"); v != "" {
		cfg.Providers.Groq.APIKey = v
	}
	if v := get("PROVIDER_MISTRAL"); v != "" {
		cfg.Providers.Mistral.APIKey = v
	}
	if v := get("PROVIDER_TOGETHER"); v != "" {
		cfg.Providers.Together.APIKey = v
	}
	if v := get("PROVIDER_DEEPSEEK"); v != "" {
		cfg.Providers.DeepSeek.APIKey = v
	}
	if v := get("PROVIDER_PERPLEXITY"); v != "" {
		cfg.Providers.Perplexity.APIKey = v
	}
	if v := get("PROVIDER_COHERE"); v != "" {
		cfg.Providers.Cohere.APIKey = v
	}
	if v := get("PROVIDER_FIREWORKS"); v != "" {
		cfg.Providers.Fireworks.APIKey = v
	}
	if v := get("PROVIDER_OLLAMA"); v != "" {
		cfg.Providers.Ollama.APIKey = v // optional Bearer-Token
	}
	if v := get("PROVIDER_CUSTOM"); v != "" {
		cfg.Providers.Custom.APIKey = v
	}
	// Voice STT + TTS
	if v := get("VOICE_API_KEY"); v != "" {
		cfg.Voice.APIKey = v
	}
	if v := get("VOICE_TTS_API_KEY"); v != "" {
		cfg.Voice.TTSAPIKey = v
	}
	// Bild-Generierung
	if v := get("IMG_OPENROUTER"); v != "" {
		cfg.ImageGen.OpenRouter.APIKey = v
	}
	if v := get("IMG_FAL"); v != "" {
		cfg.ImageGen.Fal.APIKey = v
	}
	if v := get("IMG_OPENAI"); v != "" {
		cfg.ImageGen.OpenAI.APIKey = v
	}
	if v := get("IMG_STABILITY"); v != "" {
		cfg.ImageGen.Stability.APIKey = v
	}
	if v := get("IMG_TOGETHER"); v != "" {
		cfg.ImageGen.Together.APIKey = v
	}
	if v := get("IMG_REPLICATE"); v != "" {
		cfg.ImageGen.Replicate.APIKey = v
	}
	// Video-Generierung
	if v := get("VID_RUNWAY"); v != "" {
		cfg.VideoGen.Runway.APIKey = v
	}
	if v := get("VID_KLING"); v != "" {
		cfg.VideoGen.Kling.APIKey = v
	}
	if v := get("VID_LUMA"); v != "" {
		cfg.VideoGen.Luma.APIKey = v
	}
	if v := get("VID_PIKA"); v != "" {
		cfg.VideoGen.Pika.APIKey = v
	}
	if v := get("VID_HAILUO"); v != "" {
		cfg.VideoGen.Hailuo.APIKey = v
	}
	if v := get("VID_SORA"); v != "" {
		cfg.VideoGen.Sora.APIKey = v
	}
	if v := get("VID_VEO"); v != "" {
		cfg.VideoGen.Veo.APIKey = v
	}
	// System
	if v := get("SKILL_SECRET"); v != "" {
		cfg.SkillSecret = v
	}
	if v := get("VIRUSTOTAL_API_KEY"); v != "" {
		cfg.Security.VirusTotalAPIKey = v
	}
	if v := get("DASHBOARD_PASSWORD"); v != "" {
		cfg.Dashboard.Password = v
	}
	if v := get("DASHBOARD_USERNAME"); v != "" {
		cfg.Dashboard.Username = v
	}
	// Integrationen: Vault-Werte in die Liste einfügen (INTEG_* Prefix-Konvention)
	for i, integ := range cfg.Integrations {
		if v := get("INTEG_" + integ.Name); v != "" {
			cfg.Integrations[i].Value = v
		}
	}

	// Cal.com-Schlüssel: im Vault direkt ohne INTEG_-Prefix gespeichert (eigenes Dashboard-Panel).
	// Müssen in cfg.Integrations eingefügt werden damit {{CALCOM_...}} im Skill substituiert wird.
	// Nur injizieren wenn CALCOM_ENABLED != "false" (Dashboard-Toggle).
	if get("CALCOM_ENABLED") != "false" {
		for _, key := range []string{"CALCOM_BASE_URL", "CALCOM_API_KEY", "CALCOM_OWNER_EMAIL", "CALCOM_EVENT_TYPE_ID"} {
			v := get(key)
			if v == "" {
				continue
			}
			found := false
			for i, integ := range cfg.Integrations {
				if integ.Name == key {
					cfg.Integrations[i].Value = v
					found = true
					break
				}
			}
			if !found {
				cfg.Integrations = append(cfg.Integrations, config.Integration{Name: key, Value: v})
			}
		}
	} else {
		log.Println("[Config] Cal.com deaktiviert (CALCOM_ENABLED=false) – Credentials werden nicht verwendet.")
	}

	// Session 42: Browser Skills – Keys aus Vault in Config übernehmen
	if v := get("SEARCH_API_KEY"); v != "" {
		cfg.BrowserSkills.SearchAPIKey = v
	}
	if v := get("BROWSER_ENDPOINT"); v != "" {
		cfg.BrowserSkills.BrowserEndpoint = v
	}
	if v := get("BROWSER_ALLOWED_DOMAINS"); v != "" {
		cfg.BrowserSkills.AllowedDomains = v
	}
}

// clearSecretsFromConfig setzt alle sensitiven Felder der Config auf "".
// Dient zur Bereinigung der config.json nach Vault-Migration.
func clearSecretsFromConfig(cfg *config.Config) {
	cfg.Channels.Telegram.Token = ""
	cfg.Channels.Discord.Token = ""
	cfg.Channels.Slack.BotToken = ""
	cfg.Channels.Slack.AppToken = ""
	cfg.Channels.Slack.SigningSecret = ""
	cfg.Channels.Matrix.Token = ""
	cfg.Channels.WhatsApp.APIKey = ""
	cfg.Channels.WhatsApp.WebhookSecret = ""
	cfg.Providers.OpenRouter.APIKey = ""
	cfg.Providers.Anthropic.APIKey = ""
	cfg.Providers.OpenAI.APIKey = ""
	cfg.Providers.Google.APIKey = ""
	cfg.Providers.XAI.APIKey = ""
	cfg.Providers.Groq.APIKey = ""
	cfg.Providers.Mistral.APIKey = ""
	cfg.Providers.Together.APIKey = ""
	cfg.Providers.DeepSeek.APIKey = ""
	cfg.Providers.Perplexity.APIKey = ""
	cfg.Providers.Cohere.APIKey = ""
	cfg.Providers.Fireworks.APIKey = ""
	cfg.Providers.Custom.APIKey = ""
	cfg.Voice.APIKey = ""
	cfg.Voice.TTSAPIKey = ""
	cfg.ImageGen.OpenRouter.APIKey = ""
	cfg.ImageGen.Fal.APIKey = ""
	cfg.ImageGen.OpenAI.APIKey = ""
	cfg.ImageGen.Stability.APIKey = ""
	cfg.ImageGen.Together.APIKey = ""
	cfg.ImageGen.Replicate.APIKey = ""
	cfg.VideoGen.Runway.APIKey = ""
	cfg.VideoGen.Kling.APIKey = ""
	cfg.VideoGen.Luma.APIKey = ""
	cfg.VideoGen.Pika.APIKey = ""
	cfg.VideoGen.Hailuo.APIKey = ""
	cfg.VideoGen.Sora.APIKey = ""
	cfg.VideoGen.Veo.APIKey = ""
	cfg.SkillSecret = ""
	cfg.Security.VirusTotalAPIKey = ""
	cfg.Dashboard.Password = ""
	for i := range cfg.Integrations {
		cfg.Integrations[i].Value = ""
	}
}

// ── Provider-Hilfsfunktionen ──────────────────────────────────────────────────

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
	case "ollama":
		models = cfg.Providers.Ollama.Models
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

// buildTTSSpeaker erstellt einen TTS-Speaker aus der Konfiguration.
// Gibt (nil, "") zurück wenn nicht konfiguriert oder ttsEnabled explizit false.
//
// Aktivierungslogik:
//   - ttsEnabled: false (Dashboard-Toggle) → immer aus
//   - Google: OAuth2-Credentials (GOOGLE_CLIENT_ID/SECRET/REFRESH_TOKEN) haben Vorrang
//     vor VOICE_TTS_API_KEY. Falls weder OAuth2 noch API Key → aus.
//   - Azure/OpenAI: VOICE_TTS_API_KEY benötigt
//   - ttsEnabled: true aber kein Key → Log-Hinweis, kein Fehler
//
// Gibt den effektiven Stimmen-Namen zurück (mit Provider-Default falls nicht konfiguriert).
func buildTTSSpeaker(cfg *config.Config, vault security.SecretProvider) (voice.TTSSpeaker, string) {
	// Toggle im Dashboard hat Vorrang: ttsEnabled=false → immer aus
	if !cfg.Voice.TTSEnabled {
		return nil, ""
	}

	ttsVoice := cfg.Voice.TTSVoice
	var speaker voice.TTSSpeaker

	switch cfg.Voice.TTSProvider {
	case "google":
		// ── Vertex AI TTS (Chirp 3 HD) via OAuth2 ─────────────────────────────
		// Google Cloud TTS und Vertex AI TTS unterstützen KEINE API Keys.
		// Authentifizierung immer über OAuth2 Bearer Token.
		// Scope benötigt: https://www.googleapis.com/auth/cloud-platform
		// → Einmalig Google-Konto im Dashboard (Google-Tab) neu verbinden.
		if vault != nil {
			clientID, _ := vault.Get("GOOGLE_CLIENT_ID")
			clientSecret, _ := vault.Get("GOOGLE_CLIENT_SECRET")
			refreshToken, _ := vault.Get("GOOGLE_REFRESH_TOKEN")
			if clientID != "" && clientSecret != "" && refreshToken != "" {
				if ttsVoice == "" {
					ttsVoice = "de-AT-Chirp3-HD-Aoede" // Chirp 3 HD Default (Österreichisch)
				}
				speaker = voice.NewVertexTTSSpeakerOAuth(clientID, clientSecret, refreshToken)
				log.Printf("[TTS] Aktiviert: google/Vertex AI Chirp 3 HD (OAuth2) | Stimme: %s | Mode: %s", ttsVoice, cfg.Voice.TTSMode)
				return speaker, ttsVoice
			}
		}
		log.Printf("[TTS] Google TTS: Google-Konto im Dashboard verbinden (Google-Tab → Autorisieren)")
		return nil, ""

	case "azure":
		if ttsVoice == "" {
			ttsVoice = "de-AT-IngridNeural"
		}
		ttsAPIKey := cfg.Voice.TTSAPIKey
		if ttsAPIKey == "" {
			log.Printf("[TTS] Azure TTS aktiviert aber kein API-Key – VOICE_TTS_API_KEY im Dashboard eintragen")
			return nil, ""
		}
		s, err := voice.NewAzureTTSSpeaker(ttsAPIKey, cfg.Voice.TTSAzureRegion)
		if err != nil {
			log.Printf("[TTS] Azure init fehlgeschlagen: %v", err)
			return nil, ""
		}
		speaker = s

	default: // "openai" oder leer
		if ttsVoice == "" {
			ttsVoice = "alloy"
		}
		ttsAPIKey := cfg.Voice.TTSAPIKey
		// Fallback auf STT-Key NUR wenn beide denselben Provider nutzen
		if ttsAPIKey == "" && cfg.Voice.Provider == cfg.Voice.TTSProvider {
			ttsAPIKey = cfg.Voice.APIKey
		}
		if ttsAPIKey == "" {
			log.Printf("[TTS] OpenAI TTS aktiviert aber kein API-Key – VOICE_TTS_API_KEY im Dashboard eintragen")
			return nil, ""
		}
		speaker = voice.NewOpenAITTSSpeaker(ttsAPIKey)
	}

	log.Printf("[TTS] Aktiviert: %s | Stimme: %s | Mode: %s", speaker.Name(), ttsVoice, cfg.Voice.TTSMode)
	return speaker, ttsVoice
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
	// HMAC-Secret-Prüfung erfolgt jetzt in runBot() nach Provider-Initialisierung.
	// Die Umgebungsvariable FLUXBOT_HMAC_SECRET dient nur noch als Fallback (CI/CD).
	// Primärquelle: Secret-Provider (Vault-Key "HMAC_SECRET") → sicherer als Env-Variable.
}

// buildGoogleClient erstellt einen Google API-Client aus dem Secret-Provider.
func buildGoogleClient(cfg *config.Config, vault security.SecretProvider) *googleapi.Client {
	clientID, _ := vault.Get("GOOGLE_CLIENT_ID")
	clientSecret, _ := vault.Get("GOOGLE_CLIENT_SECRET")
	refreshToken, _ := vault.Get("GOOGLE_REFRESH_TOKEN")
	if clientID == "" || clientSecret == "" || refreshToken == "" {
		return nil
	}
	client := googleapi.New(clientID, clientSecret, refreshToken)
	log.Println("[Main] ✅ Google API aktiviert (Calendar, Docs, Sheets, Drive, Gmail).")
	return client
}

// getIntegration sucht einen Integrations-Wert in der Config nach Name.
func getIntegration(cfg *config.Config, name string) string {
	for _, integ := range cfg.Integrations {
		if integ.Name == name {
			return integ.Value
		}
	}
	return ""
}

// parseIntegrationInt liest einen Integrations-Wert als int.
func parseIntegrationInt(cfg *config.Config, name string) int {
	v := getIntegration(cfg, name)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

// ── SESSION 42: Browser Skills ─────────────────────────────────────────────

// buildSearchClient erstellt den Tavily-Such-Client (nil wenn kein Key konfiguriert).
func buildSearchClient(cfg *config.Config) *searchpkg.Client {
	key := cfg.BrowserSkills.SearchAPIKey
	if key == "" {
		return nil
	}
	client := searchpkg.New(key)
	log.Println("[Main] ✅ Web-Suche (Tavily) aktiviert.")
	return client
}

// buildBrowserClient erstellt den Playwright-Browser-Client.
// Mit Playwright wird der Browser automatisch gestartet – kein externer Endpoint nötig.
func buildBrowserClient(cfg *config.Config) *browser.Client {
	// Browser Skills sind immer verfügbar mit Playwright
	var domains []string
	if cfg.BrowserSkills.AllowedDomains != "" {
		for _, d := range strings.Split(cfg.BrowserSkills.AllowedDomains, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				domains = append(domains, d)
			}
		}
	}
	// BrowserType aus Config lesen (default: chromium)
	browserType := cfg.BrowserSkills.BrowserType
	if browserType == "" {
		browserType = "chromium"
	}

	// Playwright startet den Browser automatisch im Hintergrund
	client := browser.New("", domains, browserType)
	if len(domains) > 0 {
		log.Printf("[Main] ✅ Browser Skills aktiviert (Playwright/%s) | Whitelist: %v", browserType, domains)
	} else {
		log.Printf("[Main] ⚠️ Browser Skills aktiviert (Playwright/%s) | KEINE Domain-Whitelist – alle URLs erlaubt!", browserType)
	}
	return client
}

// ── P11: DANGEROUS-TOOLS ───────────────────────────────────────────────────

// buildDangerousToolsCfg erstellt die Dangerous-Tools Konfiguration für den Agent.
func buildDangerousToolsCfg(cfg *config.Config) *agent.DangerousToolsCfg {
	dt := cfg.DangerousTools
	if !dt.Enabled {
		return nil
	}
	return &agent.DangerousToolsCfg{
		Enabled:  true,
		AdminIDs: dt.AdminIDs,
		Blocked:  dt.Blocked,
	}
}
