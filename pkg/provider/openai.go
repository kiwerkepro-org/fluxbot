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

// OpenAICompat ist ein universeller Provider für alle OpenAI-kompatiblen APIs.
// Funktioniert mit: OpenAI, Groq, Together, Mistral, LM Studio, etc.
type OpenAICompat struct {
	name    string
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewOpenAI erstellt einen Provider für die offizielle OpenAI API (api.openai.com).
func NewOpenAI(apiKey string) *OpenAICompat {
	return &OpenAICompat{
		name:    "openai",
		apiKey:  apiKey,
		baseURL: "https://api.openai.com/v1/chat/completions",
		client:  &http.Client{Timeout: 180 * time.Second},
	}
}

// NewGroq erstellt einen Provider für Groq (sehr schnelle Inference, kostenloser Tier verfügbar).
func NewGroq(apiKey string) *OpenAICompat {
	return &OpenAICompat{
		name:    "groq",
		apiKey:  apiKey,
		baseURL: "https://api.groq.com/openai/v1/chat/completions",
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// NewOpenAICompat erstellt einen Provider für eine beliebige OpenAI-kompatible API.
func NewOpenAICompat(name, apiKey, baseURL string) *OpenAICompat {
	return &OpenAICompat{
		name:    name,
		apiKey:  apiKey,
		baseURL: baseURL,
		client:  &http.Client{Timeout: 180 * time.Second},
	}
}

func (o *OpenAICompat) Name() string { return o.name }

// Complete sendet eine Anfrage an die OpenAI-kompatible API und gibt die Antwort zurück.
func (o *OpenAICompat) Complete(ctx context.Context, req Request) (string, error) {
	messages := make([]map[string]string, 0, len(req.Messages)+1)

	if req.System != "" {
		messages = append(messages, map[string]string{
			"role":    "system",
			"content": req.System,
		})
	}

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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("fehler beim Erstellen des HTTP-Requests: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("HTTP-Anfrage fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("fehler beim Parsen der Antwort: %w", err)
	}

	if result.Error.Message != "" {
		return "", fmt.Errorf("%s API Fehler: %s", o.name, result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("keine Antwort vom Modell erhalten")
	}

	return result.Choices[0].Message.Content, nil
}
