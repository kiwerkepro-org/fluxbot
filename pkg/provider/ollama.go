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

// OllamaDefaultBaseURL ist die Standard-URL für eine lokale Ollama-Instanz.
const OllamaDefaultBaseURL = "http://localhost:11434"

// OllamaProvider ist ein Provider für lokale Ollama-Instanzen.
// Ollama nutzt die OpenAI-kompatible API (/v1/chat/completions) – kein API-Key nötig.
// Optional kann ein Bearer-Token gesetzt werden, wenn Ollama hinter einem Auth-Proxy läuft.
type OllamaProvider struct {
	baseURL     string
	bearerToken string // optional; leer = kein Authorization-Header
	client      *http.Client
}

// NewOllama erstellt einen Provider für eine lokale Ollama-Instanz.
//
//	baseURL:     z.B. "http://localhost:11434" – wird automatisch um /v1/... erweitert
//	bearerToken: optional, leer = kein Auth-Header (Standard-Ollama braucht keinen)
func NewOllama(baseURL, bearerToken string) *OllamaProvider {
	if baseURL == "" {
		baseURL = OllamaDefaultBaseURL
	}
	return &OllamaProvider{
		baseURL:     baseURL,
		bearerToken: bearerToken,
		// Ollama läuft lokal und kann bei großen Modellen länger brauchen → großzügiger Timeout
		client: &http.Client{Timeout: 300 * time.Second},
	}
}

func (o *OllamaProvider) Name() string { return "ollama" }

// Complete sendet eine Anfrage an die lokale Ollama-Instanz und gibt die Antwort zurück.
func (o *OllamaProvider) Complete(ctx context.Context, req Request) (string, error) {
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
		"stream":   false,
	})
	if err != nil {
		return "", fmt.Errorf("ollama: fehler beim Erstellen der Anfrage: %w", err)
	}

	endpoint := o.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("ollama: fehler beim Erstellen des HTTP-Requests: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	// Authorization-Header nur setzen wenn Bearer-Token vorhanden (Standard-Ollama braucht keinen)
	if o.bearerToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.bearerToken)
	}

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ollama: nicht erreichbar unter %s – läuft Ollama lokal? (%w)", o.baseURL, err)
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
		return "", fmt.Errorf("ollama: fehler beim Parsen der Antwort: %w", err)
	}

	if result.Error.Message != "" {
		return "", fmt.Errorf("ollama API Fehler: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("ollama: keine Antwort erhalten – ist das Modell '%s' heruntergeladen?", req.Model)
	}

	return result.Choices[0].Message.Content, nil
}

// PingOllama prüft ob eine Ollama-Instanz unter baseURL erreichbar ist.
// Gibt nil zurück wenn Ollama antwortet, einen Fehler wenn nicht.
// Kein Absturz – FluxBot startet auch ohne erreichbares Ollama (Warnung im Log).
func PingOllama(baseURL string) error {
	if baseURL == "" {
		baseURL = OllamaDefaultBaseURL
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/api/tags")
	if err != nil {
		return fmt.Errorf("ollama nicht erreichbar unter %s: %w", baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("ollama antwortet mit HTTP %d unter %s", resp.StatusCode, baseURL)
	}
	return nil
}
