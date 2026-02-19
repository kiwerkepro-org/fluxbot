package voice

import (
	"context"
	"fmt"
)

// OpenAITranscriber nutzt die OpenAI Whisper API für Transkription.
// Wird in Phase 2.x vollständig implementiert.
type OpenAITranscriber struct {
	apiKey string
}

// NewOpenAITranscriber erstellt einen neuen OpenAI Whisper Transcriber.
func NewOpenAITranscriber(apiKey string) *OpenAITranscriber {
	return &OpenAITranscriber{apiKey: apiKey}
}

func (o *OpenAITranscriber) Name() string { return "openai" }

func (o *OpenAITranscriber) Transcribe(_ context.Context, _ string, _ string) (string, error) {
	return "", fmt.Errorf("openai voice noch nicht implementiert – bitte 'groq' als Provider verwenden")
}
