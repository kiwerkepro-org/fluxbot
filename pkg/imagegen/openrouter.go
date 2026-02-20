package imagegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// OpenRouter liefert Bilder über die Chat-Completions-API.
//
// Unterstützte Bild-Modelle (Stand 2026):
//   - black-forest-labs/flux.2-pro          (kostenpflichtig)
//   - bytedance-seed/seedream-4.5            (kostenpflichtig)
//   - black-forest-labs/flux-schnell:free   (kostenlos)
//
// OpenRouter gibt bei Bildmodellen die Bild-URL im content-Feld zurück,
// entweder als direkter String, als Markdown-Bild oder als URL-Array.

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

	// OpenRouter Bildmodelle: Anfrage über Chat-Completions.
	// Die Bild-URL kommt im content-Feld der Antwort zurück.
	payload := map[string]interface{}{
		"model": g.model,
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": fmt.Sprintf("Generate an image: %s\nSize: %dx%d pixels", prompt, w, h),
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

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter-img: API nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		msg := string(respBody)
		// HTML-Dump verhindern
		if strings.HasPrefix(strings.TrimSpace(msg), "<") {
			msg = fmt.Sprintf("HTTP %d – ungültiger Endpunkt oder Modell nicht verfügbar", resp.StatusCode)
		} else if len(msg) > 400 {
			msg = msg[:400] + "…"
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
		Error *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("[OpenRouter-IMG] JSON-Parse-Fehler. Raw response (500 chars): %.500s", string(respBody))
		return nil, fmt.Errorf("openrouter-img: JSON-Parse-Fehler: %w", err)
	}

	// API-Fehler im Body prüfen (HTTP 200 aber Fehlerinhalt)
	if result.Error != nil {
		return nil, fmt.Errorf("openrouter-img: API-Fehler: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		log.Printf("[OpenRouter-IMG] Keine Choices in Antwort. Raw: %.500s", string(respBody))
		return nil, fmt.Errorf("openrouter-img: keine Antwort vom Modell erhalten")
	}

	// Bild-URL aus dem Content extrahieren
	content := result.Choices[0].Message.Content
	log.Printf("[OpenRouter-IMG] Modell %s – Content-Typ: %T – Wert: %v", g.model, content, content)

	imageURL := extractImageURL(content)
	if imageURL == "" {
		// Rohe Antwort für Debugging loggen
		log.Printf("[OpenRouter-IMG] Keine URL gefunden. Roher Content: %v", content)
		return nil, fmt.Errorf(
			"openrouter-img: Modell '%s' hat geantwortet, aber keine Bild-URL geliefert.\n"+
				"Tipp: Nutze fal.ai oder Together AI als Bild-Provider im Dashboard.",
			g.model,
		)
	}

	log.Printf("[OpenRouter-IMG] Bild-URL gefunden: %s", imageURL)

	return &Image{
		URL:    imageURL,
		Format: "png",
		Width:  w,
		Height: h,
	}, nil
}

// extractImageURL sucht eine Bild-URL in einer Chat-Completions-Antwort.
// Unterstützt: direkte URL, Markdown-Bild ![](url), URL im Text, content-Array.
func extractImageURL(content interface{}) string {
	switch v := content.(type) {
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return ""
		}

		// 1. Direkter URL-String (nur URL, kein Leerzeichen)
		if strings.HasPrefix(v, "http") && !strings.ContainsAny(v, " \n\t") {
			return strings.TrimRight(v, ".,)")
		}

		// 2. Markdown-Bild: ![alt](url)
		if idx := strings.Index(v, "!["); idx >= 0 {
			rest := v[idx+2:]
			if closeBracket := strings.Index(rest, "]("); closeBracket >= 0 {
				urlPart := rest[closeBracket+2:]
				if closeParen := strings.Index(urlPart, ")"); closeParen >= 0 {
					u := strings.TrimSpace(urlPart[:closeParen])
					if strings.HasPrefix(u, "http") {
						return u
					}
				}
			}
		}

		// 3. Erste https-URL im Text suchen (wortweise)
		for _, word := range strings.Fields(v) {
			word = strings.Trim(word, ".,()\"'<>\n\r")
			if strings.HasPrefix(word, "https://") || strings.HasPrefix(word, "http://") {
				return word
			}
		}

		// 4. Letzte Chance: irgendwo "http" im String?
		if idx := strings.Index(v, "https://"); idx >= 0 {
			sub := v[idx:]
			// Bis zum nächsten Leerzeichen oder Anführungszeichen
			end := strings.IndexAny(sub, " \n\t\"'<>)")
			if end < 0 {
				return strings.TrimRight(sub, ".,)")
			}
			return strings.TrimRight(sub[:end], ".,)")
		}
		if idx := strings.Index(v, "http://"); idx >= 0 {
			sub := v[idx:]
			end := strings.IndexAny(sub, " \n\t\"'<>)")
			if end < 0 {
				return strings.TrimRight(sub, ".,)")
			}
			return strings.TrimRight(sub[:end], ".,)")
		}

	case []interface{}:
		// Content-Array (multimodal)
		for _, part := range v {
			if m, ok := part.(map[string]interface{}); ok {
				switch m["type"] {
				case "image_url":
					if iu, ok := m["image_url"].(map[string]interface{}); ok {
						if url, ok := iu["url"].(string); ok && url != "" {
							return url
						}
					}
				case "image":
					// Einige Provider geben base64 oder URL direkt
					if src, ok := m["source"].(map[string]interface{}); ok {
						if url, ok := src["url"].(string); ok && url != "" {
							return url
						}
					}
				case "text":
					if text, ok := m["text"].(string); ok {
						if url := extractImageURL(text); url != "" {
							return url
						}
					}
				}
			}
		}
	}
	return ""
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
