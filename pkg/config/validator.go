package config

import (
	"fmt"
	"strings"
)

// ValidationError sammelt alle Konfigurationsfehler in einer übersichtlichen Liste
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	pad := 14 - len(fmt.Sprintf("%d", len(e.Errors)))
	if pad < 0 {
		pad = 0
	}
	return fmt.Sprintf(
		"\n╔══════════════════════════════════════════════╗\n"+
			"║  FluxBot – Konfigurationsfehler (%d gefunden)%s║\n"+
			"╚══════════════════════════════════════════════╝\n"+
			"%s\n"+
			"→ Bitte workspace/config.json prüfen (Vorlage: workspace/config.example.json)",
		len(e.Errors),
		strings.Repeat(" ", pad),
		strings.Join(e.Errors, "\n"),
	)
}

func (e *ValidationError) add(field, hint string) {
	e.Errors = append(e.Errors, fmt.Sprintf("  ❌ %-40s  → %s", field, hint))
}

func (e *ValidationError) hasErrors() bool {
	return len(e.Errors) > 0
}

// Validate prüft die Konfiguration vollständig
func (c *Config) Validate() error {
	ve := &ValidationError{}

	hasChannel := c.Channels.Telegram.Enabled ||
		c.Channels.Discord.Enabled ||
		c.Channels.Slack.Enabled ||
		c.Channels.Matrix.Enabled ||
		c.Channels.WhatsApp.Enabled

	if !hasChannel {
		ve.add("channels.*", "Kein Channel aktiv – mindestens einen auf 'enabled: true' setzen")
	}

	if c.Channels.Telegram.Enabled {
		if c.Channels.Telegram.Token == "" {
			ve.add("channels.telegram.token", "Bot-Token von @BotFather")
		}
	}

	if c.Channels.Discord.Enabled {
		if c.Channels.Discord.Token == "" {
			ve.add("channels.discord.token", "Bot-Token aus dem Discord Developer Portal")
		}
	}

	if c.Channels.Slack.Enabled {
		if c.Channels.Slack.BotToken == "" {
			ve.add("channels.slack.botToken", "xoxb-... Token (Bot OAuth Token)")
		}
		if c.Channels.Slack.SigningSecret == "" {
			ve.add("channels.slack.signingSecret", "Signing Secret aus api.slack.com → App → Basic Information")
		}
	}

	if c.Channels.Matrix.Enabled {
		if c.Channels.Matrix.HomeServer == "" {
			ve.add("channels.matrix.homeServer", "z.B. https://matrix.org")
		}
		if c.Channels.Matrix.UserID == "" {
			ve.add("channels.matrix.userId", "z.B. @fluxbot:matrix.org")
		}
		if c.Channels.Matrix.Token == "" {
			ve.add("channels.matrix.token", "Access Token (Element → Einstellungen → Hilfe → Access Token)")
		}
	}

	if c.Channels.WhatsApp.Enabled {
		switch c.Channels.WhatsApp.Provider {
		case "":
			ve.add("channels.whatsapp.provider", "'meta' (Business Cloud API)")
		case "meta":
			if c.Channels.WhatsApp.APIKey == "" {
				ve.add("channels.whatsapp.apiKey", "Meta Access Token (developers.facebook.com → Token generieren)")
			}
			if c.Channels.WhatsApp.PhoneNumberID == "" {
				ve.add("channels.whatsapp.phoneNumberId", "Phone Number ID aus Meta Developer Portal (nicht die Rufnummer!)")
			}
			if c.Channels.WhatsApp.WebhookSecret == "" {
				ve.add("channels.whatsapp.webhookSecret", "Beliebiger Geheimschlüssel für HMAC-Verifizierung")
			}
		default:
			ve.add("channels.whatsapp.provider",
				fmt.Sprintf("'%s' unbekannt – derzeit unterstützt: meta", c.Channels.WhatsApp.Provider))
		}
	}

	if c.Providers.OpenRouter.APIKey == "" {
		ve.add("providers.openrouter.apiKey", "API Key von openrouter.ai → Keys")
	}

	if c.Voice.Enabled {
		switch c.Voice.Provider {
		case "":
			ve.add("voice.provider", "'groq' (kostenlos), 'openai' oder 'ollama' (lokal)")
		case "groq":
			if c.Voice.APIKey == "" {
				ve.add("voice.apiKey", "Groq API Key von console.groq.com (kostenlos)")
			}
		case "openai":
			if c.Voice.APIKey == "" {
				ve.add("voice.apiKey", "OpenAI API Key von platform.openai.com")
			}
		case "ollama":
			// Ollama läuft lokal, kein API Key nötig
		default:
			ve.add("voice.provider",
				fmt.Sprintf("'%s' unbekannt – erlaubt: groq, openai, ollama", c.Voice.Provider))
		}
	}

	// ImageGen: Provider und Key nur prüfen wenn Default explizit gesetzt ist
	if c.ImageGen.Default != "" && c.ImageGen.Default != "openrouter-shared" {
		switch c.ImageGen.Default {
		case "openrouter":
			if c.ImageGen.OpenRouter.APIKey == "" {
				ve.add("imageGen.openrouter.apiKey", "OpenRouter API Key von openrouter.ai/keys")
			}
		case "fal":
			if c.ImageGen.Fal.APIKey == "" {
				ve.add("imageGen.fal.apiKey", "fal.ai API Key von fal.ai/dashboard/keys")
			}
		case "openai":
			if c.ImageGen.OpenAI.APIKey == "" {
				ve.add("imageGen.openai.apiKey", "OpenAI API Key von platform.openai.com")
			}
		case "stability":
			if c.ImageGen.Stability.APIKey == "" {
				ve.add("imageGen.stability.apiKey", "Stability AI API Key von platform.stability.ai")
			}
		case "together":
			if c.ImageGen.Together.APIKey == "" {
				ve.add("imageGen.together.apiKey", "Together AI API Key von api.together.xyz")
			}
		case "replicate":
			if c.ImageGen.Replicate.APIKey == "" {
				ve.add("imageGen.replicate.apiKey", "Replicate API Key von replicate.com/account")
			}
		}
	}

	if ve.hasErrors() {
		return ve
	}
	return nil
}
