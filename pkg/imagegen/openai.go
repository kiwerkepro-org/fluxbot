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

// OpenAIImageGenerator nutzt DALL-E 3 für Bildgenerierung.
//
// API Key: https://platform.openai.com
// Kosten: ~0,04 $/Bild (Standard) oder ~0,08 $/Bild (HD)
type OpenAIImageGenerator struct {
	apiKey  string
	quality string // "standard" oder "hd"
}

// NewOpenAIImageGenerator erstellt einen neuen DALL-E 3 Generator.
func NewOpenAIImageGenerator(apiKey, quality string) *OpenAIImageGenerator {
	if quality == "" {
		quality = "standard"
	}
	return &OpenAIImageGenerator{apiKey: apiKey, quality: quality}
}

func (o *OpenAIImageGenerator) Name() string { return "dall-e-3" }

func (o *OpenAIImageGenerator) Generate(ctx context.Context, prompt, size string) (*Image, error) {
	dalleSize := mapDalleSize(size)

	payload := map[string]interface{}{
		"model":   "dall-e-3",
		"prompt":  prompt,
		"n":       1,
		"size":    dalleSize,
		"quality": o.quality,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.openai.com/v1/images/generations",
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("dall-e: request-Fehler: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dall-e: API nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("dall-e: API Fehler %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			URL           string `json:"url"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("dall-e: JSON-Parse-Fehler: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("dall-e: keine Bilder in der Antwort")
	}

	return &Image{
		URL:     result.Data[0].URL,
		Format:  "png",
		Revised: result.Data[0].RevisedPrompt,
	}, nil
}

// mapDalleSize mappt Größenangaben auf DALL-E-3-Formate
func mapDalleSize(size string) string {
	switch size {
	case "square", "1:1", "1024x1024":
		return "1024x1024"
	case "landscape", "16:9", "1792x1024":
		return "1792x1024"
	case "portrait", "9:16", "1024x1792":
		return "1024x1792"
	default:
		return "1792x1024" // Standard: Querformat
	}
}
