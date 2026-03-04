package voice

import "context"

// TTSSpeaker ist das Interface für alle Text-to-Speech Provider.
// Gibt rohe Audio-Bytes zurück (OGG/Opus für Telegram, MP3 für andere).
type TTSSpeaker interface {
	// Name gibt den Provider-Namen zurück (z.B. "openai")
	Name() string

	// Speak wandelt Text in Audio um und gibt die Bytes zurück.
	// voiceName: Provider-spezifischer Stimmen-Name (z.B. "alloy", "nova")
	// Leer = Provider-Default.
	Speak(ctx context.Context, text string, voiceName string) ([]byte, error)
}
