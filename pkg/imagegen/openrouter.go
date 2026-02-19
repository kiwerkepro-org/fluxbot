package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenRouter liefert Bilder über die Chat-Completions-API.
//
// Unterstützte Bild-Modelle (Stand 2026):
//   - black-forest-labs/flux-1.1-pro          (kostenpflichtig)
//   - black-forest-labs/flux-schnell:free      (kostenlos)
//   - google/gemini-2.0-flash-exp:free         (kostenlos, multimodal)
//
// OpenRouter hat KEINEN /images/generations Endpunkt.
// Stattdessen: /chat/completions – manche Modelle geben Bild-URLs zurück.

const openRouterChatBase = "https://openrouter.ai/api/v1/chat/completions"

type OpenRouterImageGenerator struct {
	apiKey      string
	model       string
	displayName string
}

func NewOpenRouterImageGenerator(apiKey, model, displayName string) *OpenRouterImageGenerator {
	if model == "" {
		model = "black-forest-labs/flux-schnell:free"
	}
	if displayName == "" {
		displayName = model
	}
	return &OpenRouterImageGenerator{apiKey: apiKey, model: model, displayName: displayName}
}

func (g *OpenRouterImageGenerator) Name() string { return g.displayName }

func (g *OpenRouterImageGenerator) Generate(ctx context.Context, prompt, size string) (*Image, error) {
	w, h := mapOpenRouterSize(size)

	// OpenRouter Bildmodelle werden über Chat-Completions angesteuert.
	// Der Prompt enthält die Bildanforderung, die Antwort enthält eine Bild-URL.
	payload := map[string]interface{}{
		"model": g.model,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": fmt.Sprintf("Generate an image: %s\nSize: %dx%d", prompt, w, h),
			},
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", openRouterChatBase, bytes.NewReader(body))
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

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		// Kurze Fehlermeldung – kein HTML-Dump
		msg := string(respBody)
		if strings.HasPrefix(strings.TrimSpace(msg), "<") {
			msg = fmt.Sprintf("HTTP %d (kein JSON – falscher Endpunkt oder Modell nicht verfügbar)", resp.StatusCode)
		} else if len(msg) > 300 {
			msg = msg[:300] + "…"
		}
		return nil, fmt.Errorf("openrouter-img: API Fehler %d: %s", resp.StatusCode, msg)
	}

	// Chat-Completions Antwort parsen
	var result struct {
		Choices []struct {
			Message struct {
				Content interface{} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("openrouter-img: JSON-Parse-Fehler: %w", err)
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("openrouter-img: keine Antwort vom Modell")
	}

	// Bild-URL aus dem Content extrahieren
	content := result.Choices[0].Message.Content
	imageURL := extractImageURL(content)
	if imageURL == "" {
		return nil, fmt.Errorf(
			"openrouter-img: Modell '%s' lieferte keine Bild-URL.\n"+
				"Tipp: Nutze einen dedizierten Bild-Provider (fal.ai, Together AI) im Dashboard.",
			g.model,
		)
	}

	return &Image{
		URL:    imageURL,
		Format: "png",
		Width:  w,
		Height: h,
	}, nil
}

// extractImageURL sucht eine Bild-URL in einer Chat-Completions-Antwort.
// OpenRouter gibt bei Bildmodellen entweder eine URL als String oder
// ein content-Array mit image_url-Objekten zurück.
func extractImageURL(content interface{}) string {
	switch v := content.(type) {
	case string:
		// Direkte URL
		if strings.HasPrefix(v, "http") {
			return strings.TrimSpace(v)
		}
		// URL irgendwo im Text
		for _, word := range strings.Fields(v) {
			if strings.HasPrefix(word, "https://") {
				clean := strings.Trim(word, ".,)")
				if isImageURL(clean) {
					return clean
				}
			}
		}
	case []interface{}:
		// Content-Array (multimodal)
		for _, part := range v {
			if m, ok := part.(map[string]interface{}); ok {
				if m["type"] == "image_url" {
					if iu, ok := m["image_url"].(map[string]interface{}); ok {
						if url, ok := iu["url"].(string); ok {
							return url
						}
					}
				}
			}
		}
	}
	return ""
}

func isImageURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.Contains(lower, ".png") ||
		strings.Contains(lower, ".jpg") ||
		strings.Contains(lower, ".jpeg") ||
		strings.Contains(lower, ".webp") ||
		strings.Contains(lower, "image") ||
		strings.Contains(lower, "cdn") ||
		strings.Contains(lower, "storage")
}

// mapOpenRouterSize mappt Größenangaben auf Pixel-Dimensionen
func mapOpenRouterSize(size string) (int, int) {
	switch size {
	case "square", "1:1", "1024x1024":
		return 1024, 1024
	case "portrait", "9:16", "768x1344":
		return 768, 1344
	case "landscape", "16:9", "1344x768":
		return 1344, 768
	default:
		return 1344, 768
	}
}
