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
	browser        playwright.Browser // Playwright Browser-Instanz (headless, für Screenshots etc.)
	visibleBrowser playwright.Browser // Sichtbare Browser-Instanz (für "öffne Seite")
	visiblePW      *playwright.Playwright // Playwright-Instanz für sichtbaren Browser
	allowedDomains []string           // Whitelist (leer = alle erlaubt – Warnung im Log)
	timeout        time.Duration      // Timeout pro Aktion (Standard: 60s)
	headless       bool               // Headless-Modus (Standard: true)
	browserType    string             // "chromium" (default), "firefox", "webkit"
}

// realisticUA ist ein aktueller Chrome User-Agent um Bot-Erkennung zu vermeiden.
const realisticUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

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
		Args: []string{
			"--disable-blink-features=AutomationControlled",
			"--no-first-run",
			"--no-default-browser-check",
		},
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

// stealthInit injiziert Anti-Detection Scripts auf einer neuen Seite.
// Verwendet AddInitScript damit das Script vor jedem Page-Load ausgeführt wird.
func stealthInit(page playwright.Page) {
	script := `
		Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
		Object.defineProperty(navigator, 'languages', {get: () => ['de-DE', 'de', 'en-US', 'en']});
		Object.defineProperty(navigator, 'plugins', {get: () => [1, 2, 3, 4, 5]});
		window.chrome = {runtime: {}};
	`
	if err := page.AddInitScript(playwright.Script{Content: &script}); err != nil {
		log.Printf("[Browser] ⚠️ Stealth-Init fehlgeschlagen: %v", err)
	}
}

// dismissCookieBanner versucht gängige Cookie-Consent-Banner wegzuklicken.
// Wartet kurz (max 2s) und klickt auf bekannte "Ablehnen"/"Reject"-Buttons.
// Fehler werden ignoriert – wenn kein Banner da ist, passiert nichts.
func dismissCookieBanner(page playwright.Page) {
	// Gängige Selektoren für "Alle ablehnen" / "Reject all" / "Decline" Buttons
	selectors := []string{
		// Google Consent
		`button:has-text("Alle ablehnen")`,
		`button:has-text("Reject all")`,
		`button:has-text("Alle ablehnen")`,
		// Generische Cookie-Banner
		`button:has-text("Decline")`,
		`button:has-text("Ablehnen")`,
		`button:has-text("Nur notwendige")`,
		`button:has-text("Only necessary")`,
		`button:has-text("Reject")`,
		`[id*="reject" i]`,
		`[id*="decline" i]`,
		`[class*="reject" i]`,
		// CMP (Consent Management Platforms)
		`.cmpboxbtn[onclick*="reject"]`,
		`#onetrust-reject-all-handler`,
		`[data-testid="reject-all"]`,
		`.cc-deny`,
	}

	timeout := float64(2000) // 2 Sekunden max
	for _, sel := range selectors {
		el, err := page.WaitForSelector(sel, playwright.PageWaitForSelectorOptions{
			Timeout: &timeout,
			State:   playwright.WaitForSelectorStateVisible,
		})
		if err == nil && el != nil {
			if err := el.Click(); err == nil {
				log.Printf("[Browser] 🍪 Cookie-Banner geschlossen via: %s", sel)
				// Kurz warten bis Banner-Animation fertig
				page.WaitForTimeout(500)
				return
			}
		}
	}
}

// OpenVisible öffnet eine URL in einem sichtbaren Browserfenster.
// Der Browser bleibt offen – der User kann interagieren.
func (c *Client) OpenVisible(url string) error {
	if !c.IsAllowed(url) {
		return fmt.Errorf("🚫 URL nicht in Whitelist erlaubt: %s", url)
	}

	// Sichtbaren Browser starten falls noch nicht vorhanden
	if c.visibleBrowser == nil {
		pw, err := playwright.Run()
		if err != nil {
			return fmt.Errorf("Playwright nicht verfügbar: %w", err)
		}
		c.visiblePW = pw

		opts := playwright.BrowserTypeLaunchOptions{
			Headless: playwright.Bool(false),
			Args: []string{
				"--disable-blink-features=AutomationControlled",
				"--no-first-run",
				"--no-default-browser-check",
				"--start-maximized",
			},
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
			return fmt.Errorf("Sichtbarer Browser konnte nicht gestartet werden: %w", err)
		}
		c.visibleBrowser = browser
		log.Printf("[Browser] ✅ Sichtbarer %s gestartet", c.browserType)
	}

	// Neue Seite öffnen (mit 0x0 Viewport = maximiert)
	ua := realisticUA
	page, err := c.visibleBrowser.NewPage(playwright.BrowserNewPageOptions{
		UserAgent:      &ua,
		NoViewport:     playwright.Bool(true),
	})
	if err != nil {
		return fmt.Errorf("Neue Seite konnte nicht geöffnet werden: %w", err)
	}
	stealthInit(page)

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		page.Close()
		return fmt.Errorf("Seite laden fehlgeschlagen: %w", err)
	}
	dismissCookieBanner(page)

	log.Printf("[Browser] 🌐 Sichtbare Seite geöffnet: %s", url)
	// Seite bleibt offen – NICHT page.Close()!
	return nil
}

// newPage öffnet eine neue Browser-Seite.
func (c *Client) newPage(ctx context.Context) (playwright.Page, error) {
	browser, err := c.ensureBrowser(ctx)
	if err != nil {
		return nil, err
	}

	ua := realisticUA
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		UserAgent: &ua,
	})
	if err != nil {
		return nil, fmt.Errorf("Neue Seite konnte nicht geöffnet werden: %w", err)
	}

	// Timeout setzen: float64 in Millisekunden
	timeoutMs := float64(c.timeout.Milliseconds())
	page.SetDefaultTimeout(timeoutMs)
	page.SetDefaultNavigationTimeout(timeoutMs)
	stealthInit(page)

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
	dismissCookieBanner(page)

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
	ua := realisticUA
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		Viewport:  &playwright.Size{Width: 1280, Height: 800},
		UserAgent: &ua,
	})
	if err != nil {
		return nil, fmt.Errorf("Neue Seite konnte nicht geöffnet werden: %w", err)
	}
	defer page.Close()

	timeoutMs := float64(c.timeout.Milliseconds())
	page.SetDefaultTimeout(timeoutMs)
	page.SetDefaultNavigationTimeout(timeoutMs)
	stealthInit(page)

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return nil, fmt.Errorf("Seite laden fehlgeschlagen (%s): %w", url, err)
	}
	dismissCookieBanner(page)

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
	dismissCookieBanner(page)

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

// WebAction beschreibt eine einzelne Browser-Aktion in einer Sequenz.
type WebAction struct {
	Action   string `json:"action"`             // "goto", "fill", "click", "screenshot", "read", "wait", "select"
	URL      string `json:"url,omitempty"`      // für "goto"
	Selector string `json:"selector,omitempty"` // für "fill", "click", "wait", "select", "read"
	Value    string `json:"value,omitempty"`    // für "fill", "select"
	WaitMs   int    `json:"wait,omitempty"`     // Millisekunden für "wait" (max 10000)
}

// ActionResult enthält das kombinierte Ergebnis von RunActions.
type ActionResult struct {
	Text       string   // Akkumulierter Text von "read"-Aktionen
	Screenshot []byte   // Screenshot-Bytes von der letzten "screenshot"-Aktion
	Log        []string // Schritt-für-Schritt-Protokoll
}

// RunActions führt eine Sequenz von Browser-Aktionen auf einer einzelnen Seite aus.
// Maximal 20 Aktionen pro Aufruf. Gesamttimeout: 120 Sekunden.
func (c *Client) RunActions(actions []WebAction) (*ActionResult, error) {
	if len(actions) == 0 {
		return nil, fmt.Errorf("keine Aktionen angegeben")
	}
	if len(actions) > 20 {
		return nil, fmt.Errorf("maximal 20 Aktionen erlaubt (erhalten: %d)", len(actions))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	browser, err := c.ensureBrowser(ctx)
	if err != nil {
		return nil, err
	}

	ua := realisticUA
	page, err := browser.NewPage(playwright.BrowserNewPageOptions{
		Viewport:  &playwright.Size{Width: 1280, Height: 800},
		UserAgent: &ua,
	})
	if err != nil {
		return nil, fmt.Errorf("Neue Seite konnte nicht geöffnet werden: %w", err)
	}
	defer page.Close()

	timeoutMs := float64(c.timeout.Milliseconds())
	page.SetDefaultTimeout(timeoutMs)
	page.SetDefaultNavigationTimeout(timeoutMs)
	stealthInit(page)

	result := &ActionResult{}

	for i, action := range actions {
		step := fmt.Sprintf("Schritt %d/%d: %s", i+1, len(actions), action.Action)
		log.Printf("[Browser] %s", step)

		switch action.Action {
		case "goto":
			if action.URL == "" {
				return result, fmt.Errorf("%s: URL fehlt", step)
			}
			if !c.IsAllowed(action.URL) {
				return result, fmt.Errorf("%s: 🚫 URL nicht erlaubt: %s", step, action.URL)
			}
			if _, err := page.Goto(action.URL, playwright.PageGotoOptions{
				WaitUntil: playwright.WaitUntilStateNetworkidle,
			}); err != nil {
				return result, fmt.Errorf("%s: Seite laden fehlgeschlagen: %w", step, err)
			}
			dismissCookieBanner(page)
			result.Log = append(result.Log, fmt.Sprintf("✅ %s aufgerufen", action.URL))

		case "fill":
			if action.Selector == "" {
				return result, fmt.Errorf("%s: Selektor fehlt", step)
			}
			// Passwort-Felder blockieren
			selectorLower := strings.ToLower(action.Selector)
			if strings.Contains(selectorLower, "password") || strings.Contains(selectorLower, "passwd") || strings.Contains(selectorLower, "pwd") {
				return result, fmt.Errorf("%s: 🔒 Passwort-Felder dürfen nicht automatisch ausgefüllt werden", step)
			}
			if err := page.Fill(action.Selector, action.Value); err != nil {
				return result, fmt.Errorf("%s: Feld ausfüllen fehlgeschlagen (%s): %w", step, action.Selector, err)
			}
			result.Log = append(result.Log, fmt.Sprintf("✅ Feld %s ausgefüllt", action.Selector))

		case "click":
			if action.Selector == "" {
				return result, fmt.Errorf("%s: Selektor fehlt", step)
			}
			if err := page.Click(action.Selector); err != nil {
				return result, fmt.Errorf("%s: Klick fehlgeschlagen (%s): %w", step, action.Selector, err)
			}
			// Nach Klick auf Seitenladung warten
			page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
				State: playwright.LoadStateNetworkidle,
			})
			result.Log = append(result.Log, fmt.Sprintf("✅ Klick auf %s", action.Selector))

		case "screenshot":
			buf, err := page.Screenshot(playwright.PageScreenshotOptions{
				FullPage: playwright.Bool(false),
				Type:     playwright.ScreenshotTypePng,
			})
			if err != nil {
				return result, fmt.Errorf("%s: Screenshot fehlgeschlagen: %w", step, err)
			}
			result.Screenshot = buf
			result.Log = append(result.Log, fmt.Sprintf("📸 Screenshot (%d bytes)", len(buf)))

		case "read":
			selector := action.Selector
			if selector == "" {
				selector = "body"
			}
			text, err := page.TextContent(selector)
			if err != nil {
				return result, fmt.Errorf("%s: Text lesen fehlgeschlagen (%s): %w", step, selector, err)
			}
			if len(text) > 4000 {
				text = text[:3997] + "..."
			}
			result.Text += text
			result.Log = append(result.Log, fmt.Sprintf("✅ Text gelesen (%d Zeichen)", len(text)))

		case "wait":
			waitMs := action.WaitMs
			if waitMs <= 0 {
				waitMs = 1000
			}
			if waitMs > 10000 {
				waitMs = 10000
			}
			time.Sleep(time.Duration(waitMs) * time.Millisecond)
			result.Log = append(result.Log, fmt.Sprintf("⏳ %dms gewartet", waitMs))

		case "select":
			if action.Selector == "" {
				return result, fmt.Errorf("%s: Selektor fehlt", step)
			}
			if _, err := page.SelectOption(action.Selector, playwright.SelectOptionValues{Values: &[]string{action.Value}}); err != nil {
				return result, fmt.Errorf("%s: Auswahl fehlgeschlagen (%s): %w", step, action.Selector, err)
			}
			result.Log = append(result.Log, fmt.Sprintf("✅ Option '%s' gewählt in %s", action.Value, action.Selector))

		default:
			return result, fmt.Errorf("%s: Unbekannte Aktion '%s'", step, action.Action)
		}
	}

	log.Printf("[Browser] ✅ RunActions: %d Aktionen ausgeführt", len(actions))
	return result, nil
}

// Close schließt den Browser.
func (c *Client) Close() error {
	if c.visibleBrowser != nil {
		c.visibleBrowser.Close()
		c.visibleBrowser = nil
	}
	if c.visiblePW != nil {
		c.visiblePW.Stop()
		c.visiblePW = nil
	}
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
