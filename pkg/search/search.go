// Package search bietet Web-Suche via Tavily API für FluxBot.
// Dokumentation: https://docs.tavily.com
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const tavilyURL = "https://api.tavily.com/search"

// Client ist der Tavily-HTTP-Client.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// SearchOptions steuert das Suchverhalten.
type SearchOptions struct {
	MaxResults    int    // Standard: 5
	IncludeAnswer bool   // KI-Zusammenfassung einbeziehen (Standard: true)
	SearchDepth   string // "basic" oder "advanced" (Standard: "basic")
}

// SearchResult ist ein einzelnes Suchergebnis.
type SearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// SearchResponse ist die vollständige Tavily-Antwort.
type SearchResponse struct {
	Answer  string         `json:"answer"`
	Results []SearchResult `json:"results"`
	Query   string         `json:"query"`
}

// New erstellt einen neuen Tavily-Client.
func New(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsConfigured gibt zurück ob ein API-Key gesetzt ist.
func (c *Client) IsConfigured() bool {
	return c != nil && c.apiKey != ""
}

// Search führt eine Web-Suche durch und gibt die Ergebnisse zurück.
func (c *Client) Search(ctx context.Context, query string, opts ...SearchOptions) (*SearchResponse, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("Tavily API-Key nicht konfiguriert (Vault-Key: SEARCH_API_KEY)")
	}

	opt := SearchOptions{
		MaxResults:    5,
		IncludeAnswer: true,
		SearchDepth:   "basic",
	}
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.MaxResults <= 0 {
		opt.MaxResults = 5
	}

	payload := map[string]interface{}{
		"api_key":        c.apiKey,
		"query":          query,
		"max_results":    opt.MaxResults,
		"include_answer": opt.IncludeAnswer,
		"search_depth":   opt.SearchDepth,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("JSON-Marshaling: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilyURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("Request erstellen: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP-Fehler: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Antwort lesen: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Tavily API Fehler %d: %s", resp.StatusCode, string(respBody))
	}

	var result SearchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("JSON-Parsing: %w", err)
	}

	log.Printf("[Search] ✅ Query: %q – %d Ergebnisse", query, len(result.Results))
	return &result, nil
}

// FormatResults gibt eine lesbare Markdown-Zusammenfassung zurück.
func FormatResults(resp *SearchResponse) string {
	if resp == nil || (len(resp.Results) == 0 && resp.Answer == "") {
		return "❌ Keine Suchergebnisse gefunden."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 *Suchergebnisse für: %s*\n\n", resp.Query))

	if resp.Answer != "" {
		sb.WriteString("💡 *Zusammenfassung:*\n")
		sb.WriteString(resp.Answer)
		sb.WriteString("\n\n")
	}

	if len(resp.Results) > 0 {
		sb.WriteString("📰 *Quellen:*\n")
		for i, r := range resp.Results {
			if i >= 5 {
				break
			}
			// Content auf max. 200 Zeichen kürzen
			content := r.Content
			if len(content) > 200 {
				content = content[:197] + "..."
			}
			sb.WriteString(fmt.Sprintf("\n*%d. %s*\n%s\n🔗 %s\n", i+1, r.Title, content, r.URL))
		}
	}

	return sb.String()
}
