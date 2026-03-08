// Package browser steuert einen Browser via Playwright für Web-Automatisierung.
// Playwright startet einen Chromium-Browser im Hintergrund (headless).
// Dokumentation: https://github.com/microsoft/playwright-go
package browser

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

// Client steuert einen Browser via Playwright.
type Client struct {
	browser        playwright.Browser // Playwright Browser-Instanz
	allowedDomains []string           // Whitelist (leer = alle erlaubt – Warnung im Log)
	timeout        time.Duration      // Timeout pro Aktion (Standard: 60s)
	headless       bool               // Headless-Modus (Standard: true)
	browserType    string             // "chromium" (default), "firefox", "webkit"
}

// FillField beschreibt ein Formularfeld zum Ausfüllen.
type FillField struct {
	Selector string `json:"selector"` // CSS-Selektor, z.B. "#name" oder "input[type=email]"
	Value    string `json:"value"`    // Einzutragender Wert
}

// New erstellt einen neuen Browser-Client mit Playwright.
// endpoint: Wird ignoriert (Playwright startet seinen eigenen Browser)
// allowedDomains: Whitelist-Domains (leer = alles erlaubt, nicht empfohlen)
// browserType: "chromium" (default), "firefox", "webkit"
func New(endpoint string, allowedDomains []string, browserType string) *Client {
	if browserType == "" {
		browserType = "chromium"
	}
	log.Printf("[Browser] Playwright Client initialisiert | Engine: %s | headless: true", browserType)
	return &Client{
		browser:        nil,
		allowedDomains: allowedDomains,
		timeout:        60 * time.Second,
		headless:       true,
		browserType:    browserType,
	}
}

// IsConfigured gibt zurück ob der Client einsatzbereit ist.
func (c *Client) IsConfigured() bool {
	return c != nil
}

// IsAllowed prüft ob eine URL laut Whitelist erlaubt ist.
func (c *Client) IsAllowed(url string) bool {
	if len(c.allowedDomains) == 0 {
		log.Printf("[Browser] ⚠️ Keine Domain-Whitelist konfiguriert – alle URLs erlaubt. Empfehlung: BROWSER_ALLOWED_DOMAINS setzen.")
		return true
	}
	for _, domain := range c.allowedDomains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		if strings.Contains(url, domain) {
			return true
		}
	}
	return false
}

// ensureBrowser startet einen Browser, falls noch nicht geschehen.
func (c *Client) ensureBrowser(ctx context.Context) (playwright.Browser, error) {
	if c.browser != nil {
		return c.browser, nil
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("Playwright nicht installiert. Führe aus: 'go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps': %w", err)
	}

	opts := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(c.headless),
	}

	var browser playwright.Browser
	switch strings.ToLower(c.browserType) {
	case "firefox":
		browser, err = pw.Firefox.Launch(opts)
	case "webkit":
		browser, err = pw.WebKit.Launch(opts)
	default:
		browser, err = pw.Chromium.Launch(opts)
	}
	if err != nil {
		return nil, fmt.Errorf("Browser-Start fehlgeschlagen (%s): %w", c.browserType, err)
	}

	c.browser = browser
	log.Printf("[Browser] ✅ %s gestartet (headless=%v)", c.browserType, c.headless)
	return browser, nil
}

// newPage öffnet eine neue Browser-Seite.
func (c *Client) newPage(ctx context.Context) (playwright.Page, error) {
	browser, err := c.ensureBrowser(ctx)
	if err != nil {
		return nil, err
	}

	page, err := browser.NewPage()
	if err != nil {
		return nil, fmt.Errorf("Neue Seite konnte nicht geöffnet werden: %w", err)
	}

	// Timeout setzen: float64 in Millisekunden
	timeoutMs := float64(c.timeout.Milliseconds())
	page.SetDefaultTimeout(timeoutMs)
	page.SetDefaultNavigationTimeout(timeoutMs)

	return page, nil
}

// ReadPage öffnet eine URL und extrahiert den sichtbaren Text.
func (c *Client) ReadPage(url string) (string, error) {
	if !c.IsAllowed(url) {
		return "", fmt.Errorf("🚫 URL nicht in Whitelist erlaubt: %s\n(Bitte BROWSER_ALLOWED_DOMAINS im Dashboard konfigurieren)", url)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	page, err := c.newPage(ctx)
	if err != nil {
		return "", err
	}
	defer page.Close()

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return "", fmt.Errorf("Seite laden fehlgeschlagen (%s): %w", url, err)
	}

	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	}); err != nil {
		return "", fmt.Errorf("Seite ladet nicht vollständig (%s): %w", url, err)
	}

	text, err := page.TextContent("body")
	if err != nil {
		return "", fmt.Errorf("Text-Extraktion fehlgeschlagen (%s): %w", url, err)
	}

	if len(text) > 4000 {
		text = text[:3997] + "..."
	}

	log.Printf("[Browser] ✅ ReadPage: %s (%d Zeichen)", url, len(text))
	return text, nil
}

// Screenshot macht einen Viewport-Screenshot einer URL (1280x800).
// Kein Full-Page, da Telegram Bilder mit extremen Dimensionen ablehnt (PHOTO_INVALID_DIMENSIONS).
func (c *Client) Screenshot(url string) ([]byte, error) {
	if !c.IsAllowed(url) {
		return nil, fmt.Errorf("🚫 URL nicht in Whitelist erlaubt: %s", url)
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	browser, err := c.ensureBrowser(ctx)
	if err != nil {
		return nil, err
	}

	// Viewport auf 1280x800 setzen (Desktop-Ansicht, Telegram-kompatibel)
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		Viewport: &playwright.Size{Width: 1280, Height: 800},
	})
	if err != nil {
		return nil, fmt.Errorf("Neue Seite konnte nicht geöffnet werden: %w", err)
	}
	defer page.Close()

	timeoutMs := float64(c.timeout.Milliseconds())
	page.SetDefaultTimeout(timeoutMs)
	page.SetDefaultNavigationTimeout(timeoutMs)

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return nil, fmt.Errorf("Seite laden fehlgeschlagen (%s): %w", url, err)
	}

	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	}); err != nil {
		return nil, fmt.Errorf("Seite ladet nicht vollständig (%s): %w", url, err)
	}

	// Viewport-Screenshot (nicht FullPage) – passt in Telegram-Limits
	buf, err := page.Screenshot(playwright.PageScreenshotOptions{
		FullPage: playwright.Bool(false),
		Type:     playwright.ScreenshotTypePng,
	})
	if err != nil {
		return nil, fmt.Errorf("Screenshot fehlgeschlagen (%s): %w", url, err)
	}

	log.Printf("[Browser] 📸 Screenshot: %s (%d bytes, 1280x800 viewport)", url, len(buf))
	return buf, nil
}

// FillForm navigiert zu einer URL, füllt Formularfelder aus und klickt optional auf Submit.
func (c *Client) FillForm(url string, fields []FillField, submitSelector string) (string, error) {
	if !c.IsAllowed(url) {
		return "", fmt.Errorf("🚫 URL nicht in Whitelist erlaubt: %s", url)
	}

	for _, f := range fields {
		if strings.Contains(strings.ToLower(f.Selector), "password") ||
			strings.Contains(strings.ToLower(f.Selector), "passwd") ||
			strings.Contains(strings.ToLower(f.Selector), "pwd") {
			return "", fmt.Errorf("🔒 Sicherheitsregel: Passwort-Felder dürfen nicht automatisch ausgefüllt werden (Selektor: %s)", f.Selector)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	page, err := c.newPage(ctx)
	if err != nil {
		return "", err
	}
	defer page.Close()

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return "", fmt.Errorf("Seite laden fehlgeschlagen (%s): %w", url, err)
	}

	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	}); err != nil {
		return "", fmt.Errorf("Seite ladet nicht vollständig (%s): %w", url, err)
	}

	timeoutMs := playwright.Float(float64(c.timeout.Milliseconds()))
	for _, f := range fields {
		// WaitForSelector gibt (ElementHandle, error) zurück
		if _, err := page.WaitForSelector(f.Selector, playwright.PageWaitForSelectorOptions{
			Timeout: timeoutMs,
		}); err != nil {
			return "", fmt.Errorf("Feld nicht gefunden (%s): %w", f.Selector, err)
		}

		if err := page.Fill(f.Selector, f.Value); err != nil {
			return "", fmt.Errorf("Feld ausfüllen fehlgeschlagen (%s): %w", f.Selector, err)
		}
	}

	var resultText string
	if submitSelector != "" {
		if err := page.Click(submitSelector); err != nil {
			return "", fmt.Errorf("Submit-Button klicken fehlgeschlagen (%s): %w", submitSelector, err)
		}

		if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
			State: playwright.LoadStateNetworkidle,
		}); err != nil {
			return "", fmt.Errorf("Nach Submit nicht geladen (%s): %w", url, err)
		}

		text, err := page.TextContent("body")
		if err == nil {
			resultText = text
		}
	}

	if len(resultText) > 500 {
		resultText = resultText[:497] + "..."
	}

	log.Printf("[Browser] ✅ FillForm: %s (%d Felder ausgefüllt)", url, len(fields))
	return resultText, nil
}

// Close schließt den Browser.
func (c *Client) Close() error {
	if c.browser == nil {
		return nil
	}
	err := c.browser.Close()
	c.browser = nil
	if err != nil {
		log.Printf("[Browser] ⚠️ Browser-Close fehlgeschlagen: %v", err)
		return err
	}
	log.Printf("[Browser] ✅ Browser geschlossen")
	return nil
}
