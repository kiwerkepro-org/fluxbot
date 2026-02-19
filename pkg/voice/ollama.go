package voice

import (
	"context"
	"fmt"
)

// OllamaTranscriber nutzt ein lokales Ollama-Modell für Transkription.
// Wird in Phase 2.x vollständig implementiert.
type OllamaTranscriber struct {
	url string
}

// NewOllamaTranscriber erstellt einen neuen Ollama Transcriber.
func NewOllamaTranscriber(url string) *OllamaTranscriber {
	return &OllamaTranscriber{url: url}
}

func (o *OllamaTranscriber) Name() string { return "ollama" }

func (o *OllamaTranscriber) Transcribe(_ context.Context, _ string, _ string) (string, error) {
	return "", fmt.Errorf("ollama voice noch nicht implementiert – bitte 'groq' als Provider verwenden")
}
