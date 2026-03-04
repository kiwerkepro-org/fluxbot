package voice

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"
)

// azureSSMLTemplate ist das SSML-Template für Azure TTS.
// Azure erwartet SSML (Speech Synthesis Markup Language) als Request-Body.
const azureSSMLTemplate = `<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="{{.LangCode}}">` +
	`<voice name="{{.VoiceName}}">{{.Text}}</voice>` +
	`</speak>`

// AzureTTSSpeaker nutzt Microsoft Azure Cognitive Services Text-to-Speech.
// Liefert OGG/Opus direkt – kein ffmpeg nötig.
//
// Empfohlene Stimmen (Deutsch/Österreich):
//
//	de-AT-IngridNeural   (österreichisch, weiblich) ⭐ Empfehlung für JJ
//	de-AT-JonasNeural    (österreichisch, männlich)
//	de-DE-KatjaNeural    (hochdeutsch, weiblich)
//	de-DE-ConradNeural   (hochdeutsch, männlich)
//	de-DE-AmalaNeural    (hochdeutsch, weiblich, modern)
//	de-DE-BerndNeural    (hochdeutsch, männlich)
//	de-DE-LouisaNeural   (hochdeutsch, weiblich)
//
// Setup:
//  1. Azure-Konto erstellen: portal.azure.com
//  2. "Speech Service" Ressource erstellen (Free Tier F0: 500.000 Zeichen/Monat)
//  3. Subscription Key aus "Keys and Endpoint" kopieren → Vault: VOICE_TTS_AZURE_KEY
//  4. Region notieren (z.B. "westeurope") → config.json: ttsAzureRegion
type AzureTTSSpeaker struct {
	subscriptionKey string
	region          string
	client          *http.Client
	tmpl            *template.Template
}

// NewAzureTTSSpeaker erstellt einen neuen Azure TTS Speaker.
// subscriptionKey: Azure Cognitive Services Subscription Key (Key1 oder Key2)
// region: Azure-Region, z.B. "westeurope", "northeurope", "eastus"
func NewAzureTTSSpeaker(subscriptionKey, region string) (*AzureTTSSpeaker, error) {
	if region == "" {
		region = "westeurope"
	}
	tmpl, err := template.New("ssml").Parse(azureSSMLTemplate)
	if err != nil {
		return nil, fmt.Errorf("azure tts: SSML template fehler: %w", err)
	}
	return &AzureTTSSpeaker{
		subscriptionKey: subscriptionKey,
		region:          region,
		client:          &http.Client{Timeout: 60 * time.Second},
		tmpl:            tmpl,
	}, nil
}

func (a *AzureTTSSpeaker) Name() string { return "azure" }

// Speak sendet Text an die Azure TTS API und gibt OGG/Opus-Bytes zurück.
// voiceName: z.B. "de-AT-IngridNeural", "de-DE-KatjaNeural"
// Leer = "de-AT-IngridNeural" (österreichisches Deutsch, weiblich).
func (a *AzureTTSSpeaker) Speak(ctx context.Context, text string, voiceName string) ([]byte, error) {
	if voiceName == "" {
		voiceName = "de-AT-IngridNeural"
	}

	// Sprachcode aus voiceName ableiten: "de-AT-IngridNeural" → "de-AT"
	langCode := "de-AT"
	if parts := strings.Split(voiceName, "-"); len(parts) >= 2 {
		langCode = parts[0] + "-" + parts[1]
	}

	// XML-Sonderzeichen im Text escapen (SSML ist XML)
	safeText := escapeSSML(text)

	// SSML aufbauen
	var ssmlBuf bytes.Buffer
	if err := a.tmpl.Execute(&ssmlBuf, struct {
		LangCode  string
		VoiceName string
		Text      string
	}{langCode, voiceName, safeText}); err != nil {
		return nil, fmt.Errorf("azure tts: SSML generierung fehlgeschlagen: %w", err)
	}

	endpoint := fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1", a.region)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, &ssmlBuf)
	if err != nil {
		return nil, fmt.Errorf("azure tts: request fehler: %w", err)
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", a.subscriptionKey)
	req.Header.Set("Content-Type", "application/ssml+xml")
	// OGG/Opus direkt anfordern – kein ffmpeg nötig
	req.Header.Set("X-Microsoft-OutputFormat", "ogg-24khz-16bit-mono-opus")
	req.Header.Set("User-Agent", "FluxBot")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("azure tts: API nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("azure tts: antwort lesen fehlgeschlagen: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure tts: HTTP %d – %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// escapeSSML ersetzt XML-Sonderzeichen die in SSML nicht erlaubt sind.
func escapeSSML(text string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(text)
}
