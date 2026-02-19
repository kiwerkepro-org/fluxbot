package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const openRouterImageBase = "https://openrouter.ai/api/v1/images/generations"

// OpenRouterImageGenerator nutzt OpenRouter für Bildgenerierung.
//
// Kostenlose Modelle (Stand 2026):
//   - black-forest-labs/flux.2-pro  (beste Qualität)
//   - bytedance/seedream-4.5        (fotorealistisch)
//
// API Docs: https://openrouter.ai/docs#images
// API Key: https://openrouter.ai/keys (derselbe Key wie für Chat-Modelle)
type OpenRouterImageGenerator struct {
	apiKey      string
	model       string
	displayName string // Anzeigename in der Provider-Auswahl
}

// NewOpenRouterImageGenerator erstellt einen neuen OpenRouter-Bildgenerator.
// model: z.B. "black-forest-labs/flux.2-pro", "bytedance/seedream-4.5"
// displayName: Anzeigename in der Auswahl (leer = model ID)
func NewOpenRouterImageGenerator(apiKey, model, displayName string) *OpenRouterImageGenerator {
	if model == "" {
		model = "black-forest-labs/flux.2-pro"
	}
	if displayName == "" {
		displayName = model
	}
	return &OpenRouterImageGenerator{apiKey: apiKey, model: model, displayName: displayName}
}

func (g *OpenRouterImageGenerator) Name() string { return g.displayName }

func (g *OpenRouterImageGenerator) Generate(ctx context.Context, prompt, size string) (*Image, error) {
	w, h := mapOpenRouterSize(size)
	sizeStr := fmt.Sprintf("%dx%d", w, h)

	payload := map[string]interface{}{
		"model":  g.model,
		"prompt": prompt,
		"n":      1,
		"size":   sizeStr,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", openRouterImageBase, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openrouter-img: request-Fehler: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("HTTP-Referer", "https://github.com/ki-werke/fluxbot")
	req.Header.Set("X-Title", "FluxBot")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter-img: API nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openrouter-img: API Fehler %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			URL           string `json:"url"`
			B64JSON       string `json:"b64_json"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openrouter-img: JSON-Parse-Fehler: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("openrouter-img: keine Bilder in der Antwort")
	}

	item := result.Data[0]
	img := &Image{
		URL:     item.URL,
		Format:  "png",
		Width:   w,
		Height:  h,
		Revised: item.RevisedPrompt,
	}

	// Fallback: base64-kodiertes Bild als Data-URL
	if img.URL == "" && item.B64JSON != "" {
		img.URL = "data:image/png;base64," + item.B64JSON
	}

	if img.URL == "" {
		return nil, fmt.Errorf("openrouter-img: kein Bild-URL in der Antwort")
	}

	return img, nil
}

// mapOpenRouterSize mappt Größenangaben auf Pixel-Dimensionen für OpenRouter
func mapOpenRouterSize(size string) (int, int) {
	switch size {
	case "square", "1:1", "1024x1024":
		return 1024, 1024
	case "portrait", "9:16", "768x1344":
		return 768, 1344
	case "landscape", "16:9", "1344x768":
		return 1344, 768
	case "4:3", "1024x768":
		return 1024, 768
	default:
		return 1344, 768 // Standard: Querformat
	}
}
