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

const falAPIBase = "https://fal.run"

// FalGenerator nutzt fal.ai (Flux Schnell / Flux Dev) für Bildgenerierung.
//
// API Key: https://fal.ai/dashboard/keys (kostenloses Kontingent vorhanden)
// Standard-Modell: fal-ai/flux/schnell (sehr schnell, ~0,003 $/Bild)
type FalGenerator struct {
	apiKey string
	model  string
}

// NewFalGenerator erstellt einen neuen fal.ai Generator.
// model: z.B. "fal-ai/flux-2/klein/4b", "fal-ai/flux-2/klein/9b", "fal-ai/flux-2/pro"
func NewFalGenerator(apiKey, model string) *FalGenerator {
	if model == "" {
		model = "fal-ai/flux-2/klein/4b"
	}
	return &FalGenerator{apiKey: apiKey, model: model}
}

func (f *FalGenerator) Name() string { return "fal.ai (" + f.model + ")" }

func (f *FalGenerator) Generate(ctx context.Context, prompt, size string) (*Image, error) {
	imageSize := mapFalSize(size)

	payload := map[string]interface{}{
		"prompt":               prompt,
		"image_size":           imageSize,
		"num_inference_steps":  4,
		"num_images":           1,
		"enable_safety_checker": true,
	}
	body, _ := json.Marshal(payload)

	url := fmt.Sprintf("%s/%s", falAPIBase, f.model)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("fal: request-Fehler: %w", err)
	}
	req.Header.Set("Authorization", "Key "+f.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fal: API nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fal: API Fehler %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Images []struct {
			URL    string `json:"url"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"images"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("fal: JSON-Parse-Fehler: %w", err)
	}

	if len(result.Images) == 0 {
		return nil, fmt.Errorf("fal: keine Bilder in der Antwort")
	}

	img := result.Images[0]
	return &Image{
		URL:    img.URL,
		Format: "jpg",
		Width:  img.Width,
		Height: img.Height,
	}, nil
}

// mapFalSize mappt gängige Größenangaben auf fal.ai-Formate
func mapFalSize(size string) string {
	switch size {
	case "square", "1:1", "1024x1024":
		return "square_hd"
	case "landscape", "16:9", "1920x1080", "1280x720":
		return "landscape_16_9"
	case "portrait", "9:16", "1080x1920":
		return "portrait_16_9"
	case "4:3", "landscape_4_3":
		return "landscape_4_3"
	default:
		return "landscape_4_3" // Standard
	}
}
