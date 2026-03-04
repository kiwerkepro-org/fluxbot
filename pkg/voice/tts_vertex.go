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

// vertexTTSEndpoint – Vertex AI TTS (Chirp 3 HD) via OAuth2 Bearer Token.
//
// Hinweis: Google Cloud TTS und Vertex AI TTS unterstützen KEINE API Keys.
// Auch auf aiplatform.googleapis.com ist für TTS/Chirp OAuth2 erforderlich.
// Nur Gemini-Modelle akzeptieren API Keys auf aiplatform.googleapis.com.
//
// Authentifizierung: OAuth2 Bearer Token (aus GOOGLE_REFRESH_TOKEN im Vault).
// Scope benötigt: https://www.googleapis.com/auth/cloud-platform
// → Einmalig Google-Konto im Dashboard (Google-Tab) neu verbinden.
// Cloud TTS v1 Endpoint – unterstützt Chirp 3 HD Stimmen via OAuth2 Bearer Token.
// (Vertex AI :predict Format liefert 400 "Invalid resource field value".)
const vertexTTSEndpoint = "https://texttospeech.googleapis.com/v1beta1/text:synthesize"

// VertexTTSSpeaker nutzt die Google Vertex AI TTS API (Chirp 3 HD) via OAuth2.
//
// Empfohlene deutsche Stimmen (Chirp 3 HD – weiblich):
//
//	de-AT-Chirp3-HD-Aoede   (Österreichisch, warm) ⭐
//	de-DE-Chirp3-HD-Aoede   (Hochdeutsch, warm)
//	de-DE-Chirp3-HD-Kore    (neutral, klar)
//	de-DE-Chirp3-HD-Leda    (hell, energisch)
//	de-DE-Chirp3-HD-Zephyr  (dynamisch)
//
// Empfohlene deutsche Stimmen (Chirp 3 HD – männlich):
//
//	de-DE-Chirp3-HD-Charon  (klar, professionell)
//	de-DE-Chirp3-HD-Fenrir  (tief, markant)
//	de-DE-Chirp3-HD-Orus    (autoritär)
//	de-DE-Chirp3-HD-Puck    (jung, freundlich)
type VertexTTSSpeaker struct {
	clientID     string
	clientSecret string
	refreshToken string

	// Gecachter Access-Token (automatisch erneuert)
	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time

	client *http.Client
}

// NewVertexTTSSpeakerOAuth erstellt einen Vertex AI TTS Speaker mit OAuth2.
// clientID, clientSecret, refreshToken: Google OAuth2-Credentials aus dem Vault.
// Scope cloud-platform wird benötigt – ggf. Google-Konto im Dashboard neu verbinden.
func NewVertexTTSSpeakerOAuth(clientID, clientSecret, refreshToken string) *VertexTTSSpeaker {
	return &VertexTTSSpeaker{
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		client:       &http.Client{Timeout: 60 * time.Second},
	}
}

func (v *VertexTTSSpeaker) Name() string { return "google" }

// getAccessToken liefert einen gültigen Bearer Token (erneuert automatisch bei Ablauf).
func (v *VertexTTSSpeaker) getAccessToken() (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Noch gültig (60s Puffer)?
	if v.accessToken != "" && time.Now().Before(v.tokenExpiry.Add(-60*time.Second)) {
		return v.accessToken, nil
	}

	data := url.Values{}
	data.Set("client_id", v.clientID)
	data.Set("client_secret", v.clientSecret)
	data.Set("refresh_token", v.refreshToken)
	data.Set("grant_type", "refresh_token")

	resp, err := v.client.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return "", fmt.Errorf("vertex tts: token refresh fehlgeschlagen: %w", err)
	}
	defer resp.Body.Close()

	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("vertex tts: token decode: %w", err)
	}
	if tok.Error != "" {
		return "", fmt.Errorf("vertex tts: OAuth2 Fehler: %s – %s → Google-Konto im Dashboard neu verbinden", tok.Error, tok.ErrorDesc)
	}

	v.accessToken = tok.AccessToken
	v.tokenExpiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return v.accessToken, nil
}

// Speak sendet Text an Vertex AI TTS (Chirp 3 HD) und gibt OGG/Opus-Bytes zurück.
func (v *VertexTTSSpeaker) Speak(ctx context.Context, text string, voiceName string) ([]byte, error) {
	if voiceName == "" {
		voiceName = "de-DE-Chirp3-HD-Aoede"
	}

	// Sprachcode ableiten: "de-AT-Chirp3-HD-Aoede" → "de-AT"
	langCode := "de-DE"
	if parts := strings.Split(voiceName, "-"); len(parts) >= 2 {
		langCode = parts[0] + "-" + parts[1]
	}

	// Cloud TTS v1 Format (standard synthesize request)
	reqBody := map[string]interface{}{
		"input": map[string]string{
			"text": text,
		},
		"voice": map[string]string{
			"languageCode": langCode,
			"name":         voiceName,
		},
		"audioConfig": map[string]string{
			"audioEncoding": "OGG_OPUS",
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("vertex tts: serialisierung: %w", err)
	}

	token, err := v.getAccessToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", vertexTTSEndpoint, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("vertex tts: request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vertex tts: API nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vertex tts: antwort lesen: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error.Message != "" {
			msg := errResp.Error.Message
			if strings.Contains(msg, "PERMISSION_DENIED") || strings.Contains(msg, "insufficient") {
				msg += " → Google-Konto im Dashboard neu verbinden (cloud-platform Scope erforderlich)"
			}
			return nil, fmt.Errorf("vertex tts: fehler %d: %s", errResp.Error.Code, msg)
		}
		limit := len(body)
		if limit > 300 {
			limit = 300
		}
		return nil, fmt.Errorf("vertex tts: HTTP %d: %s", resp.StatusCode, string(body)[:limit])
	}

	// Cloud TTS Antwort: {"audioContent":"base64..."}
	var result struct {
		AudioContent string `json:"audioContent"`
	}
	if jsonErr := json.Unmarshal(body, &result); jsonErr == nil && result.AudioContent != "" {
		audioData, err := base64.StdEncoding.DecodeString(result.AudioContent)
		if err != nil {
			return nil, fmt.Errorf("vertex tts: base64 decode: %w", err)
		}
		return audioData, nil
	}

	limit := len(body)
	if limit > 300 {
		limit = 300
	}
	return nil, fmt.Errorf("vertex tts: keine Audiodaten in Antwort – Body: %s", string(body)[:limit])
}
