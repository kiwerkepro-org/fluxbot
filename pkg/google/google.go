// Package google implementiert den Google API-Client für FluxBot.
// Unterstützt: Calendar, Docs, Sheets, Drive, Gmail.
// Auth: OAuth2 mit Refresh-Token (kein SDK, nur Raw-HTTP).
package google

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Scopes die FluxBot benötigt (alle optional, werden vom Nutzer gewährt).
const (
	ScopeCalendar      = "https://www.googleapis.com/auth/calendar"
	ScopeDocs          = "https://www.googleapis.com/auth/documents"
	ScopeSheets        = "https://www.googleapis.com/auth/spreadsheets"
	ScopeDrive         = "https://www.googleapis.com/auth/drive"
	ScopeGmailSend     = "https://www.googleapis.com/auth/gmail.send"
	ScopeGmailRead     = "https://www.googleapis.com/auth/gmail.readonly"
	ScopeCloudPlatform = "https://www.googleapis.com/auth/cloud-platform" // Google Cloud TTS, Vertex AI
)

// AllScopes enthält alle benötigten Scopes für FluxBot.
// Enthält cloud-platform für Google Cloud TTS / Vertex AI TTS.
// → Nach Scope-Erweiterung: Google-Konto im Dashboard neu verbinden (einmalig).
var AllScopes = []string{
	ScopeCalendar,
	ScopeDocs,
	ScopeSheets,
	ScopeDrive,
	ScopeGmailSend,
	ScopeGmailRead,
	ScopeCloudPlatform,
}

// Client ist der Google API OAuth2-Client.
type Client struct {
	mu           sync.Mutex
	clientID     string
	clientSecret string
	refreshToken string
	accessToken  string
	tokenExpiry  time.Time
	httpClient   *http.Client
}

// tokenResponse entspricht der Antwort von /o/oauth2/token.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"` // Nur beim ersten Code-Exchange
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// New erstellt einen neuen Google API-Client.
func New(clientID, clientSecret, refreshToken string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// IsConfigured gibt true zurück wenn alle Auth-Daten vorhanden sind.
func (c *Client) IsConfigured() bool {
	return c.clientID != "" && c.clientSecret != "" && c.refreshToken != ""
}

// GetAuthURL gibt die OAuth2-Autorisierungs-URL zurück (für Dashboard-Onboarding).
func GetAuthURL(clientID, redirectURI string) string {
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", strings.Join(AllScopes, " "))
	params.Set("access_type", "offline")
	params.Set("prompt", "consent") // Erzwingt Refresh-Token-Ausgabe
	return "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()
}

// ExchangeCode tauscht einen Authorization-Code gegen Tokens.
func ExchangeCode(clientID, clientSecret, code, redirectURI string) (accessToken, refreshToken string, err error) {
	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("redirect_uri", redirectURI)
	data.Set("grant_type", "authorization_code")

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return "", "", fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", "", fmt.Errorf("token decode: %w", err)
	}
	if tok.Error != "" {
		return "", "", fmt.Errorf("google auth: %s – %s", tok.Error, tok.ErrorDesc)
	}
	return tok.AccessToken, tok.RefreshToken, nil
}

// accessToken erneuert den Access-Token wenn nötig.
func (c *Client) getAccessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Noch gültig (mit 60s Puffer)?
	if c.accessToken != "" && time.Now().Before(c.tokenExpiry.Add(-60*time.Second)) {
		return c.accessToken, nil
	}

	// Refresh via Refresh-Token.
	data := url.Values{}
	data.Set("client_id", c.clientID)
	data.Set("client_secret", c.clientSecret)
	data.Set("refresh_token", c.refreshToken)
	data.Set("grant_type", "refresh_token")

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return "", fmt.Errorf("token refresh: %w", err)
	}
	defer resp.Body.Close()

	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("token decode: %w", err)
	}
	if tok.Error != "" {
		return "", fmt.Errorf("google refresh: %s – %s", tok.Error, tok.ErrorDesc)
	}

	c.accessToken = tok.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	log.Printf("[Google] 🔑 Access-Token erneuert (gültig %ds)", tok.ExpiresIn)
	return c.accessToken, nil
}

// doRequest führt einen authentifizierten Google API-Request durch.
func (c *Client) doRequest(method, apiURL string, body interface{}) ([]byte, int, error) {
	token, err := c.getAccessToken()
	if err != nil {
		return nil, 0, err
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, apiURL, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

// ─────────────────────────────────────────────
// GOOGLE CALENDAR
// ─────────────────────────────────────────────

// CalendarEvent beschreibt einen Kalender-Eintrag.
type CalendarEvent struct {
	Title       string `json:"title"`
	Start       string `json:"start"`       // RFC3339 z.B. "2026-02-25T10:00:00+01:00"
	End         string `json:"end"`         // RFC3339
	Description string `json:"description"` // Optional
	Location    string `json:"location"`    // Optional
	CalendarID  string `json:"calendarId"`  // Optional, Default: "primary"
}

// CalendarCreateResult ist das Ergebnis einer Kalender-Erstellung.
type CalendarCreateResult struct {
	ID      string
	HtmlURL string
	Title   string
}

// CalendarCreate erstellt einen neuen Kalender-Eintrag.
func (c *Client) CalendarCreate(ev CalendarEvent) (*CalendarCreateResult, error) {
	calID := ev.CalendarID
	if calID == "" {
		calID = "primary"
	}

	payload := map[string]interface{}{
		"summary":     ev.Title,
		"description": ev.Description,
		"location":    ev.Location,
		"start":       map[string]string{"dateTime": ev.Start, "timeZone": "Europe/Vienna"},
		"end":         map[string]string{"dateTime": ev.End, "timeZone": "Europe/Vienna"},
	}

	apiURL := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events", url.PathEscape(calID))
	body, status, err := c.doRequest("POST", apiURL, payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("Google Calendar Fehler (%d): %s", status, string(body))
	}

	var result struct {
		ID      string `json:"id"`
		HtmlURL string `json:"htmlLink"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("calendar decode: %w", err)
	}
	return &CalendarCreateResult{ID: result.ID, HtmlURL: result.HtmlURL, Title: result.Summary}, nil
}

// CalendarListEvent ist ein einzelner Termin in der Liste.
type CalendarListEvent struct {
	Title       string
	Start       string
	End         string
	Description string
	Location    string
}

// CalendarList listet bevorstehende Termine auf.
func (c *Client) CalendarList(calendarID string, maxResults int) ([]CalendarListEvent, error) {
	if calendarID == "" {
		calendarID = "primary"
	}
	if maxResults <= 0 {
		maxResults = 10
	}

	params := url.Values{}
	params.Set("timeMin", time.Now().UTC().Format(time.RFC3339))
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))
	params.Set("singleEvents", "true")
	params.Set("orderBy", "startTime")

	apiURL := fmt.Sprintf("https://www.googleapis.com/calendar/v3/calendars/%s/events?%s",
		url.PathEscape(calendarID), params.Encode())

	body, status, err := c.doRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("Google Calendar Fehler (%d): %s", status, string(body))
	}

	var result struct {
		Items []struct {
			Summary     string `json:"summary"`
			Description string `json:"description"`
			Location    string `json:"location"`
			Start       struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"end"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("calendar list decode: %w", err)
	}

	var events []CalendarListEvent
	for _, item := range result.Items {
		start := item.Start.DateTime
		if start == "" {
			start = item.Start.Date
		}
		end := item.End.DateTime
		if end == "" {
			end = item.End.Date
		}
		events = append(events, CalendarListEvent{
			Title:       item.Summary,
			Start:       start,
			End:         end,
			Description: item.Description,
			Location:    item.Location,
		})
	}
	return events, nil
}

// ─────────────────────────────────────────────
// GOOGLE DOCS
// ─────────────────────────────────────────────

// DocsCreateResult ist das Ergebnis eines neuen Dokuments.
type DocsCreateResult struct {
	DocID string
	URL   string
	Title string
}

// DocsCreate erstellt ein neues Google Docs-Dokument.
func (c *Client) DocsCreate(title, content string) (*DocsCreateResult, error) {
	// Erst Dokument erstellen.
	payload := map[string]string{"title": title}
	body, status, err := c.doRequest("POST", "https://docs.googleapis.com/v1/documents", payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("Google Docs Fehler (%d): %s", status, string(body))
	}

	var doc struct {
		DocumentID string `json:"documentId"`
		Title      string `json:"title"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("docs create decode: %w", err)
	}

	// Inhalt einfügen wenn vorhanden.
	if content != "" {
		if err := c.docsInsertText(doc.DocumentID, content); err != nil {
			return nil, err
		}
	}

	return &DocsCreateResult{
		DocID: doc.DocumentID,
		URL:   fmt.Sprintf("https://docs.google.com/document/d/%s/edit", doc.DocumentID),
		Title: doc.Title,
	}, nil
}

// docsInsertText fügt Text in ein Dokument ein (am Anfang, Index 1).
func (c *Client) docsInsertText(docID, text string) error {
	payload := map[string]interface{}{
		"requests": []map[string]interface{}{
			{
				"insertText": map[string]interface{}{
					"location": map[string]int{"index": 1},
					"text":     text,
				},
			},
		},
	}
	apiURL := fmt.Sprintf("https://docs.googleapis.com/v1/documents/%s:batchUpdate", docID)
	body, status, err := c.doRequest("POST", apiURL, payload)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("Google Docs Insert Fehler (%d): %s", status, string(body))
	}
	return nil
}

// DocsAppend fügt Text am Ende eines bestehenden Dokuments an.
func (c *Client) DocsAppend(docID, text string) error {
	// Erst aktuellen End-Index ermitteln.
	apiURL := fmt.Sprintf("https://docs.googleapis.com/v1/documents/%s", docID)
	body, status, err := c.doRequest("GET", apiURL, nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("Google Docs Lesen Fehler (%d): %s", status, string(body))
	}

	var doc struct {
		Body struct {
			Content []struct {
				EndIndex int `json:"endIndex"`
			} `json:"content"`
		} `json:"body"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("docs read decode: %w", err)
	}

	endIndex := 1
	if len(doc.Body.Content) > 0 {
		endIndex = doc.Body.Content[len(doc.Body.Content)-1].EndIndex - 1
		if endIndex < 1 {
			endIndex = 1
		}
	}

	payload := map[string]interface{}{
		"requests": []map[string]interface{}{
			{
				"insertText": map[string]interface{}{
					"location": map[string]int{"index": endIndex},
					"text":     "\n" + text,
				},
			},
		},
	}
	appendURL := fmt.Sprintf("https://docs.googleapis.com/v1/documents/%s:batchUpdate", docID)
	b, s, err := c.doRequest("POST", appendURL, payload)
	if err != nil {
		return err
	}
	if s >= 400 {
		return fmt.Errorf("Google Docs Append Fehler (%d): %s", s, string(b))
	}
	return nil
}

// DocsRead liest den Textinhalt eines Dokuments.
func (c *Client) DocsRead(docID string) (string, string, error) {
	apiURL := fmt.Sprintf("https://docs.googleapis.com/v1/documents/%s", docID)
	body, status, err := c.doRequest("GET", apiURL, nil)
	if err != nil {
		return "", "", err
	}
	if status >= 400 {
		return "", "", fmt.Errorf("Google Docs Lesen Fehler (%d): %s", status, string(body))
	}

	var doc struct {
		Title string `json:"title"`
		Body  struct {
			Content []struct {
				Paragraph *struct {
					Elements []struct {
						TextRun *struct {
							Content string `json:"content"`
						} `json:"textRun"`
					} `json:"elements"`
				} `json:"paragraph"`
			} `json:"content"`
		} `json:"body"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", "", fmt.Errorf("docs read decode: %w", err)
	}

	var sb strings.Builder
	for _, elem := range doc.Body.Content {
		if elem.Paragraph != nil {
			for _, pe := range elem.Paragraph.Elements {
				if pe.TextRun != nil {
					sb.WriteString(pe.TextRun.Content)
				}
			}
		}
	}
	return doc.Title, sb.String(), nil
}

// ─────────────────────────────────────────────
// GOOGLE SHEETS
// ─────────────────────────────────────────────

// SheetsCreateResult ist das Ergebnis eines neuen Sheets.
type SheetsCreateResult struct {
	SheetID string
	URL     string
	Title   string
}

// SheetsCreate erstellt ein neues Google Sheets-Dokument.
func (c *Client) SheetsCreate(title string) (*SheetsCreateResult, error) {
	payload := map[string]interface{}{
		"properties": map[string]string{"title": title},
	}
	body, status, err := c.doRequest("POST", "https://sheets.googleapis.com/v4/spreadsheets", payload)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("Google Sheets Fehler (%d): %s", status, string(body))
	}

	var sheet struct {
		SpreadsheetID string `json:"spreadsheetId"`
		Properties    struct {
			Title string `json:"title"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(body, &sheet); err != nil {
		return nil, fmt.Errorf("sheets create decode: %w", err)
	}

	return &SheetsCreateResult{
		SheetID: sheet.SpreadsheetID,
		URL:     fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/edit", sheet.SpreadsheetID),
		Title:   sheet.Properties.Title,
	}, nil
}

// SheetsRead liest Werte aus einem Bereich.
func (c *Client) SheetsRead(sheetID, rangeStr string) ([][]string, error) {
	if rangeStr == "" {
		rangeStr = "A1:Z1000"
	}
	apiURL := fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s/values/%s",
		sheetID, url.PathEscape(rangeStr))
	body, status, err := c.doRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("Google Sheets Lesen Fehler (%d): %s", status, string(body))
	}

	var result struct {
		Values [][]interface{} `json:"values"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("sheets read decode: %w", err)
	}

	var rows [][]string
	for _, row := range result.Values {
		var cols []string
		for _, cell := range row {
			cols = append(cols, fmt.Sprintf("%v", cell))
		}
		rows = append(rows, cols)
	}
	return rows, nil
}

// SheetsWrite schreibt Werte in einen Bereich (ersetzt bestehende Werte).
func (c *Client) SheetsWrite(sheetID, rangeStr string, values [][]interface{}) error {
	if rangeStr == "" {
		rangeStr = "A1"
	}
	payload := map[string]interface{}{
		"range":          rangeStr,
		"majorDimension": "ROWS",
		"values":         values,
	}
	params := url.Values{}
	params.Set("valueInputOption", "USER_ENTERED")
	apiURL := fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s/values/%s?%s",
		sheetID, url.PathEscape(rangeStr), params.Encode())
	body, status, err := c.doRequest("PUT", apiURL, payload)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("Google Sheets Schreiben Fehler (%d): %s", status, string(body))
	}
	return nil
}

// SheetsAppend fügt Zeilen am Ende eines Sheets an.
func (c *Client) SheetsAppend(sheetID, rangeStr string, values [][]interface{}) error {
	if rangeStr == "" {
		rangeStr = "A1"
	}
	payload := map[string]interface{}{
		"range":          rangeStr,
		"majorDimension": "ROWS",
		"values":         values,
	}
	params := url.Values{}
	params.Set("valueInputOption", "USER_ENTERED")
	params.Set("insertDataOption", "INSERT_ROWS")
	apiURL := fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s/values/%s:append?%s",
		sheetID, url.PathEscape(rangeStr), params.Encode())
	body, status, err := c.doRequest("POST", apiURL, payload)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("Google Sheets Append Fehler (%d): %s", status, string(body))
	}
	return nil
}

// ─────────────────────────────────────────────
// GOOGLE DRIVE
// ─────────────────────────────────────────────

// DriveFile beschreibt eine Drive-Datei.
type DriveFile struct {
	ID       string
	Name     string
	MimeType string
	URL      string
	Modified string
}

// DriveList listet Dateien in Google Drive auf.
func (c *Client) DriveList(query string, maxResults int) ([]DriveFile, error) {
	if maxResults <= 0 {
		maxResults = 20
	}
	params := url.Values{}
	params.Set("pageSize", fmt.Sprintf("%d", maxResults))
	params.Set("fields", "files(id,name,mimeType,webViewLink,modifiedTime)")
	if query != "" {
		params.Set("q", query)
	}
	apiURL := "https://www.googleapis.com/drive/v3/files?" + params.Encode()
	body, status, err := c.doRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("Google Drive Fehler (%d): %s", status, string(body))
	}

	var result struct {
		Files []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			MimeType    string `json:"mimeType"`
			WebViewLink string `json:"webViewLink"`
			ModifiedTime string `json:"modifiedTime"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("drive list decode: %w", err)
	}

	var files []DriveFile
	for _, f := range result.Files {
		files = append(files, DriveFile{
			ID:       f.ID,
			Name:     f.Name,
			MimeType: f.MimeType,
			URL:      f.WebViewLink,
			Modified: f.ModifiedTime,
		})
	}
	return files, nil
}

// ─────────────────────────────────────────────
// GMAIL
// ─────────────────────────────────────────────

// GmailMessage beschreibt eine zu sendende E-Mail.
type GmailMessage struct {
	To      string
	Subject string
	Body    string
}

// GmailSend sendet eine E-Mail über die Gmail API.
func (c *Client) GmailSend(msg GmailMessage) error {
	// RFC 2822 Format aufbauen.
	raw := fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		msg.To, msg.Subject, msg.Body)

	// Base64url-kodieren (Gmail API erwartet das).
	encoded := base64URLEncode([]byte(raw))

	payload := map[string]string{"raw": encoded}
	body, status, err := c.doRequest("POST", "https://gmail.googleapis.com/gmail/v1/users/me/messages/send", payload)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("Gmail Senden Fehler (%d): %s", status, string(body))
	}
	log.Printf("[Google] 📧 E-Mail gesendet an %s (Betreff: %s)", msg.To, msg.Subject)
	return nil
}

// GmailListResult ist eine E-Mail in der Liste.
type GmailListResult struct {
	ID      string
	From    string
	Subject string
	Date    string
	Snippet string
}

// GmailList listet die letzten ungelesenen E-Mails.
func (c *Client) GmailList(query string, maxResults int) ([]GmailListResult, error) {
	if maxResults <= 0 {
		maxResults = 10
	}
	if query == "" {
		query = "is:unread"
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("maxResults", fmt.Sprintf("%d", maxResults))

	listURL := "https://gmail.googleapis.com/gmail/v1/users/me/messages?" + params.Encode()
	body, status, err := c.doRequest("GET", listURL, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("Gmail List Fehler (%d): %s", status, string(body))
	}

	var listResult struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &listResult); err != nil {
		return nil, fmt.Errorf("gmail list decode: %w", err)
	}

	var results []GmailListResult
	for _, m := range listResult.Messages {
		msgURL := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages/%s?format=metadata&metadataHeaders=From&metadataHeaders=Subject&metadataHeaders=Date", m.ID)
		msgBody, msgStatus, err := c.doRequest("GET", msgURL, nil)
		if err != nil || msgStatus >= 400 {
			continue
		}
		var msg struct {
			Snippet string `json:"snippet"`
			Payload struct {
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(msgBody, &msg); err != nil {
			continue
		}
		r := GmailListResult{ID: m.ID, Snippet: msg.Snippet}
		for _, h := range msg.Payload.Headers {
			switch h.Name {
			case "From":
				r.From = h.Value
			case "Subject":
				r.Subject = h.Value
			case "Date":
				r.Date = h.Value
			}
		}
		results = append(results, r)
	}
	return results, nil
}

// base64URLEncode kodiert Bytes in Base64url (ohne Padding).
func base64URLEncode(data []byte) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	encoded := make([]byte, 0, (len(data)+2)/3*4)
	for i := 0; i < len(data); i += 3 {
		b0 := data[i]
		b1 := byte(0)
		b2 := byte(0)
		if i+1 < len(data) {
			b1 = data[i+1]
		}
		if i+2 < len(data) {
			b2 = data[i+2]
		}
		encoded = append(encoded,
			chars[b0>>2],
			chars[(b0&0x3)<<4|b1>>4],
			chars[(b1&0xf)<<2|b2>>6],
			chars[b2&0x3f],
		)
	}
	// Padding entfernen
	switch len(data) % 3 {
	case 1:
		encoded = encoded[:len(encoded)-2]
	case 2:
		encoded = encoded[:len(encoded)-1]
	}
	return string(encoded)
}
