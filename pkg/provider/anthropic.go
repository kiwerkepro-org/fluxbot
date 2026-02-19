package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const anthropicBaseURL = "https://api.anthropic.com/v1/messages"
const anthropicVersion = "2023-06-01"

// Anthropic implementiert den Provider für die direkte Anthropic API (api.anthropic.com).
// Nutzt das native Messages-Format (abweichend von OpenAI).
type Anthropic struct {
	apiKey string
	client *http.Client
}

// NewAnthropic erstellt einen Provider für die direkte Anthropic Claude API.
func NewAnthropic(apiKey string) *Anthropic {
	return &Anthropic{
		apiKey: apiKey,
		client: &http.Client{Timeout: 180 * time.Second},
	}
}

func (a *Anthropic) Name() string { return "anthropic" }

// Complete sendet eine Anfrage an die Anthropic Messages API.
func (a *Anthropic) Complete(ctx context.Context, req Request) (string, error) {
	messages := make([]map[string]string, 0, len(req.Messages))
	for _, msg := range req.Messages {
		messages = append(messages, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	body := map[string]interface{}{
		"model":      req.Model,
		"max_tokens": 8192,
		"messages":   messages,
	}

	if req.System != "" {
		body["system"] = req.System
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("fehler beim Erstellen der Anfrage: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicBaseURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("fehler beim Erstellen des HTTP-Requests: %w", err)
	}

	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("HTTP-Anfrage fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("fehler beim Parsen der Antwort: %w", err)
	}

	if result.Error.Message != "" {
		return "", fmt.Errorf("anthropic API Fehler: %s", result.Error.Message)
	}

	for _, block := range result.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("keine Textantwort vom Modell erhalten")
}
