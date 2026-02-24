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

const openRouterBaseURL = "https://openrouter.ai/api/v1/chat/completions"

// OpenRouter implementiert den Provider für OpenRouter.ai.
// Unterstützt alle Modelle: Claude, GPT, Gemini, Perplexity, Mistral, Ollama, etc.
// Basiert auf der callOpenRouter-Funktion aus der alten main.go.
type OpenRouter struct {
	apiKey string
	client *http.Client
}

// NewOpenRouter erstellt einen neuen OpenRouter-Provider
func NewOpenRouter(apiKey string) *OpenRouter {
	return &OpenRouter{
		apiKey: apiKey,
		client: &http.Client{Timeout: 180 * time.Second},
	}
}

func (o *OpenRouter) Name() string { return "openrouter" }

// Complete sendet eine Anfrage an OpenRouter und gibt die Antwort zurück.
// Direkt portiert aus der alten callOpenRouter-Funktion.
func (o *OpenRouter) Complete(ctx context.Context, req Request) (string, error) {
	// Messages für API aufbauen
	messages := make([]map[string]string, 0, len(req.Messages)+1)

	// System-Prompt als erste Message
	if req.System != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": req.System,
		})
	}

	// Benutzer-Nachrichten
	for _, msg := range req.Messages {
		messages = append(messages, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"model":    req.Model,
		"messages": messages,
	})
	if err != nil {
		return "", fmt.Errorf("fehler beim Erstellen der Anfrage: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", openRouterBaseURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("fehler beim Erstellen des HTTP-Requests: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "https://github.com/ki-werke/fluxbot")
	httpReq.Header.Set("X-Title", "FluxBot")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("HTTP-Anfrage fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// HTTP-Status prüfen
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		json.Unmarshal(body, &errResp)
		errMsg := errResp.Error.Message
		if errMsg == "" {
			errMsg = string(body)
		}
		return "", fmt.Errorf("openRouter HTTP %d: %s", resp.StatusCode, errMsg)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("fehler beim Parsen der Antwort: %w", err)
	}

	if result.Error.Message != "" {
		return "", fmt.Errorf("openRouter API Fehler: %s (Code: %s)", result.Error.Message, result.Error.Code)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("keine Antwort vom Modell erhalten")
	}

	return result.Choices[0].Message.Content, nil
}
