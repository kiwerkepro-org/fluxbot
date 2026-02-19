package imagegen

import "context"

// Generator ist das Interface für alle Bildgenerierungs-Provider
type Generator interface {
	Name() string
	Generate(ctx context.Context, prompt string, size string) (*Image, error)
}

// Image enthält das Ergebnis der Bildgenerierung
type Image struct {
	URL     string // Direkt-URL zum Bild (CDN)
	Data    []byte // Rohdaten (falls kein URL, z.B. Stability AI)
	Format  string // "png", "jpg"
	Width   int
	Height  int
	Revised string // Vom Modell überarbeiteter Prompt (DALL-E)
}
