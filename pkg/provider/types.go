package provider

import (
	"context"
	"strings"
)

// Provider ist das Interface für alle KI-Provider (OpenRouter, Anthropic, OpenAI, Ollama)
type Provider interface {
	// Name gibt den Provider-Namen zurück
	Name() string

	// Complete sendet einen Prompt und gibt die KI-Antwort zurück
	Complete(ctx context.Context, req Request) (string, error)
}

// Request ist eine Anfrage an einen KI-Provider
type Request struct {
	Model    string    // Modell-ID (z.B. "anthropic/claude-sonnet-4-5-20250929")
	System   string    // System-Prompt
	Messages []Message // Chatverlauf
}

// Message ist eine Nachricht im Chatverlauf
type Message struct {
	Role    string // "user", "assistant", "system"
	Content string
}

// RouteModel wählt das passende Modell anhand von Keywords im Benutzer-Input.
// Logik aus alter main.go (routeModel-Funktion) übernommen und erweitert.
func RouteModel(input string, models map[string]string) (modelID, modelName string) {
	lower := strings.ToLower(input)

	switch {
	case containsAny(lower, "suche", "news", "aktuell", "heute", "wetter"):
		return models["search"], "Perplexity Sonar Pro"
	case containsAny(lower, "lies", "ocr", "erkenn", "scan", "bild lesen"):
		return models["ocr"], "Mistral Pixtral OCR"
	case containsAny(lower, "opus", "code", "analysier", "programmier", "debugge", "refactor"):
		return models["opus"], "Claude Opus"
	case containsAny(lower, "bild", "foto", "video", "image", "zeichne", "erstelle bild"):
		return models["image"], "Gemini 2.0 Flash"
	default:
		return models["default"], "Claude Sonnet"
	}
}

func containsAny(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
