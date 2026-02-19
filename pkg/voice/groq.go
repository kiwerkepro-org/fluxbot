package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const groqTranscribeURL = "https://api.groq.com/openai/v1/audio/transcriptions"

// GroqTranscriber nutzt Groq Whisper für schnelle, kostenlose Transkription.
// Kostenloses Tier: 7200 Sekunden Audio pro Tag.
// Unterstützte Formate: flac, mp3, mp4, mpeg, mpga, m4a, ogg, opus, wav, webm
type GroqTranscriber struct {
	apiKey string
	model  string
	client *http.Client
}

// NewGroqTranscriber erstellt einen neuen Groq Whisper Transcriber.
// model: "whisper-large-v3-turbo" (schneller) oder "whisper-large-v3" (genauer)
func NewGroqTranscriber(apiKey string) *GroqTranscriber {
	return &GroqTranscriber{
		apiKey: apiKey,
		model:  "whisper-large-v3-turbo",
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (g *GroqTranscriber) Name() string { return "groq" }

// Transcribe sendet eine Audiodatei an Groq Whisper und gibt den transkribierten Text zurück.
func (g *GroqTranscriber) Transcribe(ctx context.Context, audioPath string, language string) (string, error) {
	// Audiodatei öffnen
	file, err := os.Open(audioPath)
	if err != nil {
		return "", fmt.Errorf("audiodatei konnte nicht geöffnet werden: %w", err)
	}
	defer file.Close()

	// Multipart-Form aufbauen
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Datei-Feld
	part, err := writer.CreateFormFile("file", filepath.Base(audioPath))
	if err != nil {
		return "", fmt.Errorf("fehler beim Erstellen des Datei-Felds: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("fehler beim Kopieren der Audiodaten: %w", err)
	}

	// Modell-Feld
	if err := writer.WriteField("model", g.model); err != nil {
		return "", err
	}

	// Sprache (optional – leer = auto-detect)
	if language != "" {
		if err := writer.WriteField("language", language); err != nil {
			return "", err
		}
	}

	// Antwortformat
	if err := writer.WriteField("response_format", "json"); err != nil {
		return "", err
	}

	writer.Close()

	// HTTP-Request erstellen
	req, err := http.NewRequestWithContext(ctx, "POST", groqTranscribeURL, &buf)
	if err != nil {
		return "", fmt.Errorf("fehler beim Erstellen des HTTP-Requests: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Request senden
	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("groq API nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("groq API Fehler %d: %s", resp.StatusCode, string(body))
	}

	// Antwort parsen
	var result struct {
		Text  string `json:"text"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("fehler beim Parsen der Groq-Antwort: %w", err)
	}

	if result.Error.Message != "" {
		return "", fmt.Errorf("groq Fehler: %s", result.Error.Message)
	}

	return result.Text, nil
}
