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

// maxTTSChunkLen ist die maximale Zeichenanzahl pro TTS-Chunk.
// Google Vertex Chirp 3 HD hat ein internes Satzlängen-Limit.
// "Satz" = alles zwischen Satzzeichen (. ! ?). Zeilen ohne Satzzeichen am Ende
// (z.B. Kalender-Auflistungen) gelten als ein einziger Satz → 400 Fehler.
const maxTTSChunkLen = 300

// splitIntoTTSChunks teilt Text in Chirp-kompatible Chunks auf.
//
// Strategie (speziell für Chirp "sentence too long" Problem):
//  1. Zuerst an Newlines splitten (Kalender-Listen, Aufzählungen)
//  2. Jede Zeile bekommt einen Punkt am Ende wenn keiner da ist
//     (sonst sieht Chirp mehrere Zeilen als einen einzigen Satz)
//  3. Kurze Zeilen werden in Chunks bis maxLen zusammengefasst
//  4. Einzelne Zeilen > maxLen werden an Satzgrenzen weiter aufgespalten
func splitIntoTTSChunks(text string, maxLen int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	// Phase 1: An Newlines aufteilen
	rawLines := strings.Split(text, "\n")

	// Phase 2: Zeilen bereinigen + Satzzeichen sicherstellen
	var lines []string
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Satzzeichen am Ende ergänzen wenn nötig (Chirp-Requirement).
		// Doppelpunkt (:) zählt als Satzende bei Chirp.
		last := line[len(line)-1]
		if last != '.' && last != '!' && last != '?' && last != ':' {
			line += "."
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return []string{text}
	}

	// Phase 3: Zeilen in Chunks bis maxLen zusammenfassen
	var chunks []string
	var buf strings.Builder

	for _, line := range lines {
		// Einzelne Zeile schon zu lang? → weiter splitten
		if len(line) > maxLen {
			// Puffer flushen
			if buf.Len() > 0 {
				chunks = append(chunks, buf.String())
				buf.Reset()
			}
			// Lange Zeile an Satzgrenzen (. ! ?) splitten
			subChunks := splitLongSentence(line, maxLen)
			chunks = append(chunks, subChunks...)
			continue
		}

		// Würde diese Zeile den Chunk sprengen? → Puffer flushen
		if buf.Len() > 0 && buf.Len()+1+len(line) > maxLen {
			chunks = append(chunks, buf.String())
			buf.Reset()
		}

		if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(line)
	}

	if buf.Len() > 0 {
		chunks = append(chunks, buf.String())
	}

	if len(chunks) == 0 {
		return []string{text}
	}
	return chunks
}

// splitLongSentence teilt eine einzelne lange Zeile an Satzgrenzen (. ! ?) auf.
// Fallback: Komma, dann Leerzeichen (Wortgrenze), dann Hard-Cut.
func splitLongSentence(line string, maxLen int) []string {
	var chunks []string
	remaining := line

	for len(remaining) > maxLen {
		searchIn := remaining[:maxLen]
		splitAt := -1

		// 1. Satzende rückwärts suchen
		for i := len(searchIn) - 1; i >= maxLen/5; i-- {
			if searchIn[i] == '.' || searchIn[i] == '!' || searchIn[i] == '?' {
				splitAt = i + 1
				break
			}
		}

		// 2. Komma
		if splitAt == -1 {
			for i := len(searchIn) - 1; i >= maxLen/5; i-- {
				if searchIn[i] == ',' {
					splitAt = i + 1
					break
				}
			}
		}

		// 3. Leerzeichen (Wortgrenze)
		if splitAt == -1 {
			for i := len(searchIn) - 1; i >= maxLen/3; i-- {
				if searchIn[i] == ' ' {
					splitAt = i + 1
					break
				}
			}
		}

		// 4. Hard-Cut
		if splitAt == -1 {
			splitAt = maxLen
		}

		chunk := strings.TrimSpace(remaining[:splitAt])
		if chunk != "" {
			// Satzzeichen am Ende sicherstellen
			last := chunk[len(chunk)-1]
			if last != '.' && last != '!' && last != '?' {
				chunk += "."
			}
			chunks = append(chunks, chunk)
		}
		remaining = strings.TrimSpace(remaining[splitAt:])
	}

	if remaining != "" {
		chunks = append(chunks, remaining)
	}
	return chunks
}

// speakChunk schickt einen einzelnen Text-Chunk an die Vertex TTS API.
// Interner Helper – wird von Speak() für jeden Chunk aufgerufen.
func (v *VertexTTSSpeaker) speakChunk(ctx context.Context, text, voiceName, langCode, token string) ([]byte, error) {
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

// Speak sendet Text an Vertex AI TTS (Chirp 3 HD) und gibt OGG/Opus-Bytes zurück.
// Lange Texte werden automatisch in Chunks aufgeteilt (max. 300 Zeichen pro Chunk)
// um den Vertex-AI Fehler "sentences that are too long" (HTTP 400) zu vermeiden.
// Strategie: Newline-Split → fehlende Satzzeichen ergänzen → Chunks zusammenfassen.
// Die Audio-Chunks werden als chained OGG/Opus zurückgegeben (Telegram-kompatibel).
func (v *VertexTTSSpeaker) Speak(ctx context.Context, text string, voiceName string) ([]byte, error) {
	if voiceName == "" {
		voiceName = "de-DE-Chirp3-HD-Aoede"
	}

	// Sprachcode ableiten: "de-AT-Chirp3-HD-Aoede" → "de-AT"
	langCode := "de-DE"
	if parts := strings.Split(voiceName, "-"); len(parts) >= 2 {
		langCode = parts[0] + "-" + parts[1]
	}

	token, err := v.getAccessToken()
	if err != nil {
		return nil, err
	}

	// Text in TTS-fähige Chunks aufteilen (verhindert "sentences too long" Fehler)
	chunks := splitIntoTTSChunks(text, maxTTSChunkLen)

	if len(chunks) == 1 {
		// Kurztext: einfacher API-Call
		return v.speakChunk(ctx, chunks[0], voiceName, langCode, token)
	}

	// Langtext: jeden Chunk einzeln synthetisieren, Bytes concatenieren
	var combined []byte
	for i, chunk := range chunks {
		chunkAudio, err := v.speakChunk(ctx, chunk, voiceName, langCode, token)
		if err != nil {
			return nil, fmt.Errorf("vertex tts: chunk %d/%d fehlgeschlagen: %w", i+1, len(chunks), err)
		}
		combined = append(combined, chunkAudio...)
	}
	return combined, nil
}
