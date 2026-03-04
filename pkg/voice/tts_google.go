package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const googleTTSURL = "https://texttospeech.googleapis.com/v1/text:synthesize"

// GoogleTTSSpeaker nutzt die Google Cloud Text-to-Speech API.
// Liefert OGG/Opus direkt – kein ffmpeg nötig.
//
// Empfohlene deutsche Stimmen:
//
//	de-DE-Neural2-C  (weiblich, sehr natürlich)
//	de-DE-Neural2-F  (weiblich, modern)
//	de-DE-Neural2-B  (männlich)
//	de-DE-Wavenet-C  (weiblich, günstigere Alternative)
//	de-DE-Standard-C (weiblich, Basis-Qualität, kostenlos)
//
// Free Tier: 1 Mio. WaveNet/Neural2-Zeichen pro Monat.
//
// Authentifizierung (Priorität):
//  1. OAuth2 (clientID + clientSecret + refreshToken) → Bearer Token, automatisch erneuert
//  2. API Key (nur als Fallback; Google Cloud TTS lehnt einfache Keys oft ab)
//
// OAuth2-Scope benötigt: https://www.googleapis.com/auth/cloud-platform
// → Dashboard: Google-Konto neu verbinden wenn TTS vorher noch nicht erlaubt war.
type GoogleTTSSpeaker struct {
	// OAuth2-Credentials (bevorzugt)
	clientID     string
	clientSecret string
	refreshToken string

	// Gecachter Access-Token (OAuth2)
	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time

	// API-Key (Fallback)
	apiKey string

	language string
	client   *http.Client
}

// NewGoogleTTSSpeaker erstellt einen Google TTS Speaker mit API Key (Fallback-Modus).
// Bevorzuge NewGoogleTTSSpeakerOAuth wenn OAuth2-Credentials verfügbar sind.
func NewGoogleTTSSpeaker(apiKey, language string) *GoogleTTSSpeaker {
	if language == "" {
		language = "de-DE"
	}
	return &GoogleTTSSpeaker{
		apiKey:   apiKey,
		language: language,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

// NewGoogleTTSSpeakerOAuth erstellt einen Google TTS Speaker mit OAuth2-Authentifizierung.
// Dieser Modus funktioniert zuverlässig – API Keys werden von der Google Cloud TTS API
// oft abgelehnt ("API keys are not supported by this API").
//
// Voraussetzung: Der OAuth2-Refresh-Token muss den Scope
// https://www.googleapis.com/auth/cloud-platform enthalten.
// Falls nicht: Google-Konto im Dashboard neu verbinden.
func NewGoogleTTSSpeakerOAuth(clientID, clientSecret, refreshToken, language string) *GoogleTTSSpeaker {
	if language == "" {
		language = "de-DE"
	}
	return &GoogleTTSSpeaker{
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		language:     language,
		client:       &http.Client{Timeout: 60 * time.Second},
	}
}

func (g *GoogleTTSSpeaker) Name() string { return "google" }

// getOAuthToken liefert einen gültigen Access-Token (erneuert bei Bedarf).
func (g *GoogleTTSSpeaker) getOAuthToken() (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Noch gültig (mit 60s Puffer)?
	if g.accessToken != "" && time.Now().Before(g.tokenExpiry.Add(-60*time.Second)) {
		return g.accessToken, nil
	}

	// Token per Refresh-Token erneuern
	data := url.Values{}
	data.Set("client_id", g.clientID)
	data.Set("client_secret", g.clientSecret)
	data.Set("refresh_token", g.refreshToken)
	data.Set("grant_type", "refresh_token")

	resp, err := g.client.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return "", fmt.Errorf("google tts: token refresh: %w", err)
	}
	defer resp.Body.Close()

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("google tts: token decode: %w", err)
	}
	if tok.Error != "" {
		return "", fmt.Errorf("google tts: OAuth2-Fehler: %s – %s (Scope cloud-platform vorhanden?)", tok.Error, tok.ErrorDesc)
	}

	g.accessToken = tok.AccessToken
	g.tokenExpiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return g.accessToken, nil
}

// Speak sendet Text an die Google Cloud TTS API und gibt OGG/Opus-Bytes zurück.
// voiceName: z.B. "de-DE-Neural2-C", "de-DE-Wavenet-F", "de-DE-Standard-A"
func (g *GoogleTTSSpeaker) Speak(ctx context.Context, text string, voiceName string) ([]byte, error) {
	if voiceName == "" {
		voiceName = "de-DE-Neural2-C"
	}

	// Sprachcode aus voiceName ableiten: "de-DE-Neural2-C" → "de-DE"
	langCode := g.language
	if parts := strings.Split(voiceName, "-"); len(parts) >= 2 {
		langCode = parts[0] + "-" + parts[1]
	}

	reqBody := map[string]interface{}{
		"input": map[string]string{
			"text": text,
		},
		"voice": map[string]string{
			"languageCode": langCode,
			"name":         voiceName,
		},
		"audioConfig": map[string]string{
			"audioEncoding": "OGG_OPUS", // Direkt für Telegram, kein ffmpeg nötig
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("google tts: serialisierung: %w", err)
	}

	// Authentifizierungsstrategie bestimmen
	var reqURL string
	var bearerToken string

	if g.clientID != "" && g.clientSecret != "" && g.refreshToken != "" {
		// OAuth2 (bevorzugt) – kein ?key= Parameter
		token, err := g.getOAuthToken()
		if err != nil {
			return nil, err
		}
		bearerToken = token
		reqURL = googleTTSURL
	} else if g.apiKey != "" {
		// API Key (Fallback)
		reqURL = googleTTSURL + "?key=" + g.apiKey
	} else {
		return nil, fmt.Errorf("google tts: keine Authentifizierung konfiguriert (OAuth2 oder API Key benötigt)")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("google tts: request fehler: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google tts: API nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("google tts: antwort lesen: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error.Message != "" {
			// Hilfreiche Hinweise bei bekannten Fehlern
			msg := errResp.Error.Message
			if strings.Contains(msg, "PERMISSION_DENIED") || strings.Contains(msg, "cloud-platform") {
				msg += " → Google-Konto im Dashboard neu verbinden (Scope cloud-platform erforderlich)"
			}
			return nil, fmt.Errorf("google tts: fehler %d: %s", errResp.Error.Code, msg)
		}
		return nil, fmt.Errorf("google tts: HTTP %d", resp.StatusCode)
	}

	// Antwort parsen: audioContent ist base64-kodiert
	var result struct {
		AudioContent string `json:"audioContent"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("google tts: antwort parsen: %w", err)
	}

	audioData, err := base64.StdEncoding.DecodeString(result.AudioContent)
	if err != nil {
		return nil, fmt.Errorf("google tts: base64 decode: %w", err)
	}

	return audioData, nil
}
