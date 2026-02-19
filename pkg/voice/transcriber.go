package voice

import "context"

// Transcriber ist das Interface für alle Sprach-zu-Text Provider.
// Unterstützt: Groq Whisper (kostenlos), OpenAI Whisper, Ollama (lokal).
type Transcriber interface {
	// Name gibt den Provider-Namen zurück
	Name() string

	// Transcribe transkribiert eine Audiodatei und gibt den Text zurück.
	// audioPath ist der lokale Pfad zur Audiodatei (z.B. /tmp/voice_xxx.ogg).
	// language ist der ISO-639-1 Sprachcode (z.B. "de", "en") – leer = auto-detect.
	Transcribe(ctx context.Context, audioPath string, language string) (string, error)
}
