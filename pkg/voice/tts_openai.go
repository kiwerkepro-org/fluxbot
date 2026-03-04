package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const openAITTSURL = "https://api.openai.com/v1/audio/speech"

// OpenAITTSSpeaker nutzt die OpenAI TTS API für Sprachausgabe.
// Liefert OGG/Opus direkt – keine Konvertierung nötig, funktioniert nativ mit Telegram.
// Verfügbare Stimmen: alloy, echo, fable, onyx, nova, shimmer
// Model: tts-1 (schnell) oder tts-1-hd (besser)
type OpenAITTSSpeaker struct {
	apiKey string
	model  string
	client *http.Client
}

// NewOpenAITTSSpeaker erstellt einen neuen OpenAI TTS Speaker.
// model: "tts-1" (Standard) oder "tts-1-hd" (höhere Qualität, langsamer)
func NewOpenAITTSSpeaker(apiKey string) *OpenAITTSSpeaker {
	return &OpenAITTSSpeaker{
		apiKey: apiKey,
		model:  "tts-1",
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (o *OpenAITTSSpeaker) Name() string { return "openai" }

// Speak sendet Text an die OpenAI TTS API und gibt OGG/Opus-Bytes zurück.
// voiceName: "alloy" (Standard), "echo", "fable", "onyx", "nova", "shimmer"
func (o *OpenAITTSSpeaker) Speak(ctx context.Context, text string, voiceName string) ([]byte, error) {
	if voiceName == "" {
		voiceName = "alloy"
	}

	// Langer Text wird auf 4096 Zeichen begrenzt (OpenAI API Limit)
	if len([]rune(text)) > 4096 {
		runes := []rune(text)
		text = string(runes[:4096])
	}

	reqBody := map[string]string{
		"model":           o.model,
		"input":           text,
		"voice":           voiceName,
		"response_format": "opus", // OGG/Opus → direkt für Telegram Voice Notes
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("tts: fehler beim Serialisieren: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", openAITTSURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("tts: fehler beim Erstellen des Requests: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tts: OpenAI API nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tts: fehler beim Lesen der Antwort: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Fehler-Antwort parsen
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("tts: OpenAI Fehler %d: %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("tts: OpenAI HTTP %d", resp.StatusCode)
	}

	return body, nil
}
