# FluxBot – Session-Protokolle

> Ausgelagert aus CLAUDE.md. Stand: Session 30 (2026-03-04).
> Chronologisches Log aller Arbeitssessions.

---

## Session 55 – Terminal-Unabhängigkeit + Restart-Fix (2026-03-09)

**Fokus:** FluxBot stirbt nicht mehr wenn Terminal geschlossen wird; Restart-Button funktioniert zuverlässig

### Root Cause (endlich gefunden):
Der Updater hatte im Hintergrund v1.2.2 von GitHub heruntergeladen und `fluxbot.exe` überschrieben.
v1.2.2 war mit `-H windowsgui` gebaut → crashte sofort bei Start (ungültige stderr/stdout-Handles) mit Exit-Code 2.
Jeder Neustart-Versuch startete die kaputte Binary → Seite nicht erreichbar.

### Fixes:
1. **`-H windowsgui` entfernt** aus Makefile + release.yml – verursachte Crash wegen ungültigem stderr-Handle
2. **`FreeConsole()` syscall** in `cmd/fluxbot/console_windows.go` – neues Platform-File
   - In `main()` aufgerufen nach `printBanner()`, wenn nicht `--debug`
   - Löst Prozess vom Eltern-Terminal → stirbt nicht wenn Konsolenfenster geschlossen wird
   - No-Op wenn kein Terminal vorhanden (Task Scheduler, Windows-Dienst)
3. **Log nur in Datei** (kein `io.MultiWriter` mit os.Stdout) nach `detachConsole()` – stdout ist danach ungültig
4. **Debug-Modus** behält Konsolenausgabe (`--debug` überspringt FreeConsole)
5. **`runBot()` Signatur** erweitert: `debugMode bool` Parameter
6. **v1.2.3 released** – korrekte Binary auf GitHub, Updater installiert nie mehr kaputte v1.2.2

**Neue Dateien:** `cmd/fluxbot/console_windows.go`, `cmd/fluxbot/console_other.go`
**Geänderte Dateien:** cmd/fluxbot/main.go, service_windows.go, Makefile, .github/workflows/release.yml, CLAUDE.md

---

## Session 54 – P11 Dangerous-Tools Whitelist (2026-03-08)

**Highlights:**
- P11 Dangerous-Tools implementiert: 5 Kategorien, 80+ DE+EN Muster, Admin-Bypass, Hot-Reload
- Dashboard: neuer Tab "Dangerous Tools" mit Stats-Karten + Konfig-UI
- Speichern-Button statt Auto-Save (Auto-Save hatte keine Funktion wegen falschem fetch-Aufruf)
- Bugfix: Keyring-Crash `CredFreeW not found in Advapi32.dll` – `Find()` vor `.Call()` in `keyring_windows.go`
- Dashboard-Styling: Textarea + Checkbox-Labels jetzt mit korrekten CSS-Variablen

**Neue Dateien:** `pkg/security/dangerous_tools.go`
**Geänderte Dateien:** config.go, agent.go, main.go, api.go, server.go, dashboard.html, keyring_windows.go

---

## Session 53 – Update/Restart UI Fixes (2026-03-08)

**Fokus:** Neustart-Button + Update-Panel zuverlässig sichtbar + Restart-Flow

### Fixes:
1. **Restart-Button immer sichtbar:** Button im Update-Panel, `display:none` vom Panel entfernt
2. **Auto-Restart entfernt:** Kein komplexes Polling mehr – einfach Signal senden + 5s warten + `location.reload()`
3. **Windows-Restart:** `restart_windows.go` mit `CREATE_BREAKAWAY_FROM_JOB` + `restart_other.go`
4. **Cache-Control: no-store** im Dashboard-Handler – Browser lädt immer frische HTML
5. **Root Cause:** Browser-Cache zeigte alte HTML nach FluxBot-Neustarts → Ctrl+Shift+R hat geholfen, `no-store` verhindert das dauerhaft

### Commits: `7a80ee5`, `3e50939`, `fea6ffa`, `22fe00c`, `78a1d1b`, `c6150dc`

---

## Session 52 – P10 Granulare DM-Policy (2026-03-08)

**Fokus:** Zentrale Zugriffskontrolle für alle 5 Kanäle (open/allowlist/pairing)

### Neue Features:
1. **`pkg/channels/access.go`** (neu) – Zentrale `CheckAccess()` Funktion
   - `AccessResult`: `AccessAllowed`, `AccessDenied`, `AccessPending`
   - `AccessConfig`: Channel, SenderID, UserName, ChatID, IsDM, DMMode, GroupMode, AllowFrom, PairingStore, PairingMessage, SendFn
   - Stufe 1: AllowFrom (statisch) → immer erlaubt
   - Stufe 2: open/allowlist/pairing Modus

2. **`pkg/config/config.go`** – `DMMode`/`GroupMode` zu allen 5 Channel-Configs hinzugefügt
   - `AccessMode` Type + Konstanten: `AccessOpen`, `AccessAllowlist`, `AccessPairing`

3. **`pkg/channels/telegram.go`** – `CheckAccess()` statt isAllowed(), Legacy `PairingEnabled` erhalten
4. **`pkg/channels/discord.go`** – `CheckAccess()` + DM-Erkennung via `GuildID == ""`
5. **`pkg/channels/slack.go`** – `CheckAccess()` + DM-Erkennung via Channel-Prefix "D"
6. **`pkg/channels/matrix.go`** – `CheckAccess()` in `processSync()` (Gruppen-Modus, kein DM-Detect)
7. **`pkg/channels/whatsapp.go`** – `CheckAccess()` + `pairing`-Import, `allow`-Map entfernt (IsDM=true)
8. **`cmd/fluxbot/main.go`** – Alle 5 Channel-Konstruktoren mit DMMode/GroupMode/PairingStore
   - Slack, Matrix, WhatsApp Kanal-Registrierungen neu hinzugefügt
9. **`pkg/dashboard/dashboard.html`** – DMMode/GroupMode Dropdowns für alle 5 Kanäle
   - Laden (`setVal`) + Speichern (`getVal`) für alle 10 neuen Felder

### Geänderte Dateien:
- `pkg/channels/access.go` (neu)
- `pkg/channels/telegram.go`, `discord.go`, `slack.go`, `matrix.go`, `whatsapp.go`
- `pkg/config/config.go`
- `cmd/fluxbot/main.go`
- `pkg/dashboard/dashboard.html`

---

## Session 51 – P0 Installation & Update System (2026-03-08)

**Fokus:** Plattformübergreifende Installation + Auto-Update System

### Neue Features:

1. **`pkg/system/updater.go`** – Go Auto-Updater (neu)
   - `CheckUpdate()`: GitHub Releases API → Version vergleichen (Semver)
   - `InstallUpdate(url)`: Binary herunterladen, Backup `.bak`, ersetzen
   - `StartBackgroundCheck(ctx)`: alle 6h automatischer Check im Hintergrund
   - `platformAssetName()`: OS/Arch-Erkennung → `fluxbot-windows-amd64.exe`, `fluxbot-linux-amd64`, `fluxbot-darwin-arm64` etc.
   - `isNewerVersion()`: Semver-Vergleich `vMAJOR.MINOR.PATCH`

2. **`install.ps1`** – Windows Installer erweitert (Nativ + Docker, Menü)
   - **Nativ-Modus:** GitHub Release herunterladen, Playwright-Browser installieren, Task Scheduler Autostart, Desktop-Verknüpfung
   - **Docker-Modus:** wie vorher (docker-compose pull + up)

3. **`install.sh`** – Linux/macOS Installer erweitert (Nativ + Docker, Menü)
   - **Linux Nativ:** Binary + systemd User-Unit (`~/.config/systemd/user/fluxbot.service`)
   - **macOS Nativ:** Binary + LaunchAgent (`~/Library/LaunchAgents/de.ki-werke.fluxbot.plist`)
   - **Docker:** wie vorher

4. **Dashboard-Endpoints** (`pkg/dashboard/api.go` + `server.go`):
   - `GET /api/system/version` – Version-Info + letzter Check-Zeitpunkt
   - `POST /api/system/check-update` – sofortiger GitHub-Check
   - `POST /api/system/install-update` – 1-Klick-Update (HMAC-signiert)

5. **Update-Panel im Status-Tab** (`dashboard.html`):
   - Zeigt: Aktuelle Version ↔ Neueste Version
   - Buttons: "🔍 Auf Updates prüfen", "⬇️ Update installieren"
   - Release-Notes anzeigbar
   - Automatisch ausgeblendet wenn `/api/system/version` nicht verfügbar

6. **`--install-playwright` Flag** in `main.go`:
   - `fluxbot.exe --install-playwright` → Browser installieren + beenden
   - Wird von `install.ps1` / `install.sh` aufgerufen

7. **Makefile normalisiert:**
   - macOS-Assets: `fluxbot-darwin-arm64` / `fluxbot-darwin-amd64` (war `fluxbot-macos-*`)

8. **Version-Fallback korrigiert:** `v1.1.9` → `v1.2.1`

### Geänderte Dateien:
- `pkg/system/updater.go` (neu)
- `pkg/dashboard/api.go` – handleSystemVersion/CheckUpdate/InstallUpdate
- `pkg/dashboard/server.go` – updater-Feld + SetUpdater() + 3 neue Routen
- `pkg/dashboard/dashboard.html` – Update-Panel + JS-Funktionen
- `cmd/fluxbot/main.go` – playwright-Import, --install-playwright Flag, Updater-Init
- `install.ps1`, `install.sh` – komplett neu (Nativ + Docker Menü)
- `Makefile` – macOS Asset-Namen normalisiert

### Commit: `ddad7e8`

---

## Session 47 – Browser Actions, Stealth, Cookie-Banner, OpenVisible, Lucide Fix (2026-03-08)

**Fokus:** Dynamische Browser-Aktionen, Anti-Bot-Detection, Cookie-Banner Auto-Dismiss, sichtbarer Browser

### Neue Features:
1. **Dynamische Browser-Aktionen (`RunActions`):**
   - `pkg/browser/browser.go`: `WebAction`, `ActionResult` Structs + `RunActions()` Methode
   - 7 Aktionstypen: goto, fill, click, screenshot, read, wait, select
   - Max 20 Aktionen, 120s Gesamttimeout, Domain-Whitelist + Passwort-Block
   - `pkg/agent/agent.go`: `handleBrowserActions()` Handler + `__BROWSER_ACTIONS__` Marker
   - Beide JSON-Formate: Array `[...]` und Objekt `{actions:[...]}`
   - `workspace/skills/browser-actions.md` Skill-Datei erstellt + signiert

2. **Anti-Bot-Detection (Stealth):**
   - Browser-Launch: `--disable-blink-features=AutomationControlled`
   - `stealthInit()`: `navigator.webdriver=undefined`, realistische `languages`, `plugins`, `window.chrome`
   - Realistischer User-Agent: Chrome 131 auf Windows 10
   - `AddInitScript()` statt einmaligem `Evaluate()` → wirkt bei jedem Page-Load

3. **Cookie-Banner Auto-Dismiss:**
   - `dismissCookieBanner()` Funktion: 16 Selektoren für gängige Banner
   - Google ("Alle ablehnen"), OneTrust, CMP, generische (.cc-deny, etc.)
   - Max 2s Wartezeit, kein Fehler wenn kein Banner da
   - Wird nach jedem `Goto` in allen Browser-Funktionen aufgerufen

4. **OpenVisible – Sichtbarer Browser:**
   - `OpenVisible(url)`: Öffnet echtes Chromium-Fenster (headless=false)
   - Eigene Browser-Instanz (`visibleBrowser`), bleibt offen zum Interagieren
   - Maximiert, Stealth + Cookie-Dismiss aktiv
   - Routing: "rufe X auf" / "öffne X" → sichtbares Browserfenster

5. **Browser-Routing überarbeitet:**
   - Default von `browser-read` auf `browser-screenshot` geändert
   - "Rufe auf" / "Öffne" → `OpenVisible()` (sichtbarer Browser)
   - "lies" / "text" / "inhalt" → `browser-read` (explizit)
   - Multi-Step ("klick", "dann", "danach") → `browser-actions`
   - `isBrowserContext()` erweitert: "aufrufen", "ruf", "geh auf", .at/.ch/.io/.eu
   - Domain-Erkennung ohne zusätzliches Keyword (`.com` allein reicht)
   - `isImageRequest()` vereinfacht: ruft `isBrowserContext()` auf statt doppelter Logik
   - `extractURL()` Hilfsfunktion: erkennt URLs, www-Domains, nackte Domains

6. **Lucide Icons im Dashboard:**
   - CDN-Script fehlte komplett → `<script src="https://unpkg.com/lucide@latest/dist/umd/lucide.min.js">` eingebunden
   - Nav-Icons (Status, Konfiguration, etc.) wieder sichtbar

7. **Unsignierte Skills im Dashboard:**
   - `pkg/skills/loader.go`: `isSignatureInvalid()` gibt jetzt `true` zurück wenn `.sig` fehlt
   - Neue Skills erscheinen mit `⚠️ name (neu signieren!)` im Dashboard
   - Leerzeichen zwischen Icon und Name verbessert

### Code-Änderungen:
- `pkg/browser/browser.go`: +RunActions, +OpenVisible, +stealthInit, +dismissCookieBanner, +realisticUA, Close() erweitert
- `pkg/agent/agent.go`: +handleBrowserActions, +extractURL, isBrowserContext/isImageRequest überarbeitet, Browser-Routing erweitert
- `pkg/skills/loader.go`: isSignatureInvalid() – unsignierte Skills als NeedsResigning markiert
- `pkg/dashboard/dashboard.html`: Lucide CDN eingebunden, Skill-Label Spacing
- `workspace/skills/browser-actions.md`: Neuer Skill (signiert)
- `go.mod`: `go-openai` bleibt drin (wird für OpenAI-Provider gebraucht)

### Status: ✅ GETESTET
- Google-Suche Screenshot ohne Bot-Detection ✅
- Cookie-Banner automatisch geschlossen ✅
- ki-werke.at Screenshot ✅
- Lucide Icons im Dashboard ✅
- Unsignierte Skills Warnung ✅

---

## Session 46 – Browser Screenshot Bug ENDGÜLTIG GEFIXT (2026-03-08)

**Fokus:** Browser-Screenshot endgültig zum Laufen bringen – kein Trial-and-Error mehr

### Root Causes (4 Stück):
1. **`splitAndTrim()` YAML-Brackets-Bug** – `[screenshot, ...]` → erste/letzte Tags bekamen `[`/`]` angehängt → Score = 0 → Skill nie gematcht
2. **Generischer Skill-Matcher False Positive** – "google.com" matchte GDocs-Skill (wegen "google" Tag) statt browser-screenshot
3. **Docker-Container Telegram-Conflict** – `fluxbot_ai` Container lief parallel und pollte denselben Bot-Token → native Binary empfing keine Nachrichten (`Conflict: terminated by other getUpdates request`)
4. **Full-Page Screenshot zu groß** – 7.6 MB PNG mit extremen Dimensionen → Telegram `PHOTO_INVALID_DIMENSIONS`

### Fixes:
- `pkg/skills/loader.go`: `splitAndTrim()` → `strings.Trim(s, "[] ")` + neue `GetByName()` Methode
- `pkg/agent/agent.go`: Direktes Browser-Skill-Routing wenn `isBrowserContext()` TRUE → Matcher umgangen
- Docker: `fluxbot_ai`, `fluxbot_tailscale`, `fluxbot-container` gestoppt und entfernt
- `pkg/browser/browser.go`: Viewport-Screenshot (1280x800) statt Full-Page, `BrowserNewPageOptions{Viewport: &Size{1280, 800}}`

### Wichtige Learnings:
- **Telegram Long-Polling:** Nach Prozess-Kill 60+ Sekunden warten bevor Neustart (Timeout der alten Verbindung)
- **Session-History:** Alte Fehlermeldungen in der History können AI verwirren → bei hartnäckigen Bugs History leeren
- **YAML-Frontmatter:** `splitAndTrim()` muss `[]` Brackets handhaben – betrifft ALLE Skills mit Bracket-Notation

### Status: ✅ GEFIXT & GETESTET
- `"Mache einen Screenshot von bild.de"` → `📸 Screenshot von https://bild.de wurde aufgenommen.`

---

## Session 43 – Browser Skills Debugging & `/tmp/` Path Fix (2026-03-07)

**Fokus:** Bug-Fix für Browser Screenshots (false positive "Bildgenerierung nicht aktiviert")

### Accomplishments:
- ✅ **Bug analysiert:** `isImageRequest()` false positive auf "bild.de" (enthält "bild" + "von ")
- ✅ **isBrowserContext() Funktion implementiert** (pkg/agent/agent.go Zeilen 2148–2167)
  - Keywords: screenshot, webseite, http://, https://, www., bild.de, browser, etc.
  - Logik: `if a.isImageRequest(text) && !a.isBrowserContext(text)` → nur dann Bild-API aufrufen
- ✅ **PhotoBytesChannel Pattern** implementiert (aus Session 42 Docker-Build-Error)
  - types.go, telegram.go, manager.go → für raw PNG bytes statt URL
  - handleBrowserScreenshot() fixed → ReplyPhotoBytes()
- ✅ **Windows /tmp/ Path Bug** gefunden und gefixt:
  - `pkg/channels/utils.go` (saveTempFile): `/tmp/` → `filepath.Join(os.TempDir(), ...)`
  - `pkg/channels/discord.go` (SaveTempFileFromData): `/tmp/` → `filepath.Join(os.TempDir(), ...)`
  - **Root Cause:** Windows hat kein `/tmp/`, nur Windows Temp-Dir (AppData/Local/Temp)
  - **Impact:** Screenshots konnten nicht als Temp-Datei gespeichert werden
- ✅ **Documentation updated:** 05-bugreports.md → Bug #7 hinzugefügt mit Status "ZURÜCKGESTELLT BIS DEPLOYMENT-TEST"

### Code Changes:
1. **pkg/agent/agent.go:**
   - Neue Funktion `isBrowserContext(text string) bool` (Zeilen 2148–2167)
   - Line 648: `if a.isImageRequest(text) && !a.isBrowserContext(text)`

2. **pkg/channels/utils.go:**
   - Import `path/filepath` hinzugefügt
   - Line 13: `tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("fluxbot_media_%d%s", time.Now().UnixNano(), ext))`

3. **pkg/channels/discord.go:**
   - Import `path/filepath` hinzugefügt
   - Line 263: `tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("fluxbot_media_%d%s", time.Now().UnixNano(), ext))`

### Nächste Schritte (User muss testen):
1. `cd C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT && go build -o fluxbot.exe ./cmd/fluxbot`
2. Prozess neustarten + testen: `"Mache einen Screenshot von bild.de"`
3. Falls immer noch fehlgeschlagen: Debug-Logging in isImageRequest() + isBrowserContext() hinzufügen

### Status:
🔴 ZURÜCKGESTELLT – User extrem frustriert nach 3 fehlgeschlagenen Deployments
- Alle Code-Fixes abgeschlossen
- Deployment-Test noch ausstehend
- Wenn nach 4. Versuch immer noch fehlschlägt: tieferes Debugging nötig (Console-Logs, Marker-Parsen, etc.)

---

## Session 42 – Browser Skills: Phase 1 (Web-Suche) + Phase 2 (Browser CDP) (2026-03-06)

**Fokus:** Option D – Browser Skills implementieren (PRIORITÄT 10)

### Accomplishments:
- ✅ **Phase 1: Web-Suche via Tavily API**
  - `pkg/search/search.go` – Tavily HTTP-Client mit FormatResults()
  - Vault-Key: `SEARCH_API_KEY`
  - Marker: `__WEB_SEARCH__` / `__WEB_SEARCH_END__`
  - Handler: `handleWebSearch()` in agent.go
- ✅ **Phase 2: Browser-Steuerung via Chrome CDP**
  - `pkg/browser/browser.go` – chromedp-Client
  - ReadPage(), Screenshot(), FillForm() mit Domain-Whitelist + Passwort-Blocker
  - Vault-Keys: `BROWSER_ENDPOINT`, `BROWSER_ALLOWED_DOMAINS`
  - Marker: `__BROWSER_READ__`, `__BROWSER_SCREENSHOT__`, `__BROWSER_FILL__`
  - Handler: handleBrowserRead(), handleBrowserScreenshot(), handleBrowserFill()
- ✅ **4 neue Skills erstellt:**
  - `workspace/skills/web-search.md`
  - `workspace/skills/browser-read.md`
  - `workspace/skills/browser-screenshot.md`
  - `workspace/skills/browser-fill.md`
- ✅ **Hot-Reload:** UpdateSearchClient() + UpdateBrowserClient() in agent.go
- ✅ **Config:** BrowserSkillsConfig in pkg/config/config.go
- ✅ **go.mod:** `github.com/chromedp/chromedp v0.9.5` hinzugefügt
- ✅ **main.go:** buildSearchClient(), buildBrowserClient(), applySecrets() erweitert
- ✅ **Signier-Script:** `sign-browser-skills.py`

### Nächste Schritte (vor dem Deployment):
1. `python3 sign-browser-skills.py <SKILL_SECRET>` ausführen
2. `go mod tidy` (holt chromedp-Abhängigkeiten)
3. `docker compose down && docker compose up -d --build`
4. Im Dashboard: `SEARCH_API_KEY` (Tavily) + `BROWSER_ENDPOINT` (`ws://localhost:9222`) eintragen

### Nicht implementiert (Phase 3):
- Playwright headless (für SaaS, Docker-intern) – bleibt für spätere Session

---

## Session 37 – OpenClaw Security Research (2026-03-05)

**Fokus:** OpenClaw als Referenz-Implementation studieren, Security-Patterns für FluxBot übernehmen

### Accomplishments:
- ✅ **OpenClaw Repository analysiert** – TypeScript/Node.js Multi-Channel Agent mit WebSocket-Gateway
- ✅ **5 kritische Security-Fehler dokumentiert:**
  1. **ClawJacked** (HIGH) – Malicious Websites konnten lokal OpenClaw hijacken (schwache Gateway-Auth)
  2. **Moltbook Data Leak** (CRITICAL) – 35.000 Emails + 1.5M Agent Tokens exposed (Supabase nicht gesichert)
  3. **Credential Theft** (HIGH) – API-Keys sichtbar in UI
  4. **Malicious Skills** (CRITICAL) – Atomic Stealer macOS Malware in SKILL.md versteckt, blind ausgeführt
  5. **Massive Exposure** (CRITICAL) – 40.000+ OpenClaw Instanzen Internet-exposed ohne Auth

### Key Learnings:
- **Trust-First Design:** Pairing Mode (8-Zeichen Code, 1h TTL, max 3 pending Requests)
- **Granulare DM-Policy:** Separate Access Control für DMs vs. Groups (allowFrom, groupAllowFrom, storeAllowFrom)
- **Comprehensive Auditing:** Channel Audit, Extra Audit, File System Audit, Skill Scanner
- **Skill Security:** Keine blinde Ausführung – Scanner VOR Ausführung (Injection Detection, API Whitelist)
- **Loopback-Only Default:** Internet-Binding ist Sicherheitsrisiko (40K+ Instanzen exposed)

### FluxBot Competitive Advantage:
| Problem | OpenClaw ❌ | FluxBot ✅ |
|---------|-----------|----------|
| Secrets Storage | Cloud (Moltbook) | LOCAL ONLY: AES-256-GCM Vault |
| Gateway Auth | Schwach, brute-forcebar | Token + HMAC, rate-limited |
| Network Binding | Internet default | Loopback-only (127.0.0.1) default |
| Skill Execution | Blind, keine Validierung | Scanner VOR Ausführung |
| Secrets in UI | API-Keys sichtbar | NIEMALS sichtbar |
| Token Management | In Cloud synced | NIEMALS gesynced, lokal only |
| Malicious Skills | Undetected (Atomic Stealer) | Scanner + Injection-Detection |
| DM Pairing | Gehijackt möglich | Pairing + HMAC-signed |

### Memory-Dateien:
- 📄 **07-openclaw-research.md** – 550 Zeilen komprehensive Analyse (Architektur, Security-Patterns, Learnings)
- 📄 **CLAUDE.md** – Registrierung von 07-openclaw-research.md in Dokumentations-Tabelle

### Nächste Phasen (Session 38+):
- **Phase 1:** DM-Pairing Mode für Telegram/Discord
- **Phase 2:** Granulare DM-Policy in config.json
- **Phase 3:** Dangerous-Tools Whitelist (system.run, eval(), file.delete)

---

## Session 38 – P9 DM-Pairing Mode Implementation (2026-03-05)

**Fokus:** DM-Pairing Mode (P9) vollständig implementieren – Backend, API, Dashboard UI

### Accomplishments:
- ✅ **pkg/pairing/store.go** (NEU) – Thread-safe JSON-Store mit Mutex, Pending/Approved/Blocked Status, Auto-Persist
  - Methoden: `IsPaired()`, `IsBlocked()`, `RequestPairing()`, `Approve()`, `Block()`, `Remove()`, `SetNote()`, `List()`, `Stats()`, `GetEntry()`
  - Map-Key: `"channel:userId"` (z.B. `"telegram:123456789"`)
- ✅ **pkg/config/config.go** – `PairingConfig{Enabled bool, Message string}` Struct hinzugefügt
- ✅ **pkg/channels/telegram.go** – 3-stufige Zugriffskontrolle:
  1. AllowFrom-Whitelist (statisch, Vorrang)
  2. PairingStore (dynamisch, Dashboard-Approval)
  3. Offener Fallback (Abwärtskompatibilität)
  - `isAllowed()` gibt jetzt `false` bei leerer Whitelist (Breaking Change, bewusst)
- ✅ **cmd/fluxbot/main.go** – PairingStore-Init (`workspace/pairing.json`), `sendToChannel` Callback
- ✅ **pkg/dashboard/api.go** – `handlePairing()` (GET + POST), `handlePairingStats()`
  - POST-Aktionen: approve (mit Telegram-Benachrichtigung), block, remove, note
- ✅ **pkg/dashboard/server.go** – Routen mit HMAC-Schutz für POST
- ✅ **pkg/dashboard/dashboard.html** – Vollständiger Pairing-Tab:
  - Sidebar-Eintrag mit Pending-Badge (rot, dynamisch)
  - 4 Stats-Karten (Gepairt, Wartend, Blockiert, Gesamt)
  - Inaktiv-Banner wenn Pairing disabled
  - Filter-Tabs (Alle/Wartend/Gepairt/Blockiert)
  - Tabelle mit Status-Badges + Aktions-Buttons
  - JS: `loadPairingData()`, `pairingAction()`, `filterPairing()`, `updatePairingBadge()`

### Design-Entscheidungen:
- **User-ID statt Pairing-Code:** Telegram liefert unique User-IDs → kein 8-Zeichen Code nötig, kein TTL, einfacher
- **sendToChannel Callback:** Dashboard kann Telegram-User direkt benachrichtigen bei Approval
- **HMAC auf POST:** Pairing-Aktionen sind sicherheitskritisch → HMAC-SHA256 Signierung erforderlich

### Bugs gefixed:
- ❌ **Dashboard nicht erreichbar (Session 39):** Windows Service hatte "service controller" Fehler → Lösung: Direkt `fluxbot.exe` starten, nicht als Service
- ❌ **config.json fehlte Pairing-Section:** Wurde hinzugefügt mit `"enabled": true` und Default-Message

### Version & Release:
- **v1.1.8 → v1.1.9** in `cmd/fluxbot/main.go` bumped
- **Release-Status:** ✅ VOLLSTÄNDIG GEPUSHT
- **Deployment:** ✅ Läuft nativ auf Windows (fluxbot.exe), Dashboard erreichbar http://localhost:9090
- **Git Commit:** ✅ Durchgeführt (Commit Hash: 736af0e)
  - Files: INSTALL-Service.ps1, QUICK-START.txt, START-FluxBot.ps1
  - Message: "Session 39: Windows Service installer scripts + quick start guide"
- **GitHub Tag v1.1.9:** ✅ Existiert und ist gepusht

### Abgeschlossene Steps:
- [x] Git commit + Push (Windows durchgeführt, Commit: 736af0e)
- [x] Tag v1.1.9 setzen und zu GitHub pushen
- [x] Windows Service Installation Skripte in Repo aufgenommen

---

## Session 39 – P9 Deployment & Troubleshooting (2026-03-05)

**Fokus:** P9 DM-Pairing Mode deployen, Service-Fehler diagnostizieren & beheben

### Accomplishments:
- ✅ **Service-Diagnose:** Logs analysiert → Dashboard lief erfolgreich, Service war nur nicht gestartet
- ✅ **config.json aktualisiert:** `"pairing": {"enabled": true, "message": "..."}` hinzugefügt
- ✅ **START-FluxBot.ps1 erstellt:** PowerShell-Startskript für benutzerfreundliche exe-Ausführung
- ✅ **v1.1.9 erfolgreich deployed:** Dashboard lädt, Pairing-Features aktiv

### Problem & Lösung:
| Problem | Befund | Lösung |
|---------|--------|--------|
| Dashboard nicht erreichbar | Service lief nicht, Logs waren leer | fluxbot.exe direkt doppel-klicken |
| config.json fehlte Pairing-Section | Code war bereit, aber Config fehlte | Pairing-Block hinzugefügt + Hot-Reload |
| Windows Service-Fehler | "service controller" Error um 15:24 | Nicht kritisch – Exe-Ausführung ist stabiler |

### Status P9 DM-Pairing Mode:
- ✅ Backend komplett (pkg/pairing/store.go)
- ✅ API komplett (handlePairing, handlePairingStats)
- ✅ Dashboard UI komplett (Pairing-Tab, Filter, Actions)
- ✅ Config aktualisiert
- ✅ **LIVE & FUNKTIONAL** auf Windows

### Nächste Phase (Session 40):
**Option A: P9 Live-Testing** – Pairing-Mode verifizieren (Telegram User akzeptieren/blocken)
**Option B: UI-Upgrade (PRIORITÄT 8.5)** – Lucide Icons statt Emojis
**Option C: Self-Extend (PRIORITÄT 9)** – Bot schreibt eigene Skills
**Option D: Chrome-Skill (PRIORITÄT 10)** – Bot steuert Browser
**Option E: System-Testing** – Cal.com, VirusTotal, OAuth2 live testen

### Bugfixes:
- 🐛 HMAC-Schutz auf `/api/pairing` Route fehlte initial → hinzugefügt (POST mit `hmacVerify()`)

### Memory-Dateien aktualisiert:
- 📄 **01-features.md** – P9 als "DEPLOYED" markiert
- 📄 **03-session-log.md** – Session 38-39 vollständig dokumentiert
- 📄 **CLAUDE.md** – Aktueller Stand auf v1.1.9 gesetzt

---

## Session 40 – Self-Extend Feature Implementation (2026-03-05)

**Fokus:** Option A (P9 Live-Testing) + Option C (Self-Extend Feature)

### Accomplishments:
- ✅ **Option A: P9 Live-Testing PASSED** – Dashboard-Test erfolgreich, Stats korrekt (Pending/Approved/Blocked = 0)
- ✅ **Option C: Self-Extend Feature COMPLETED** – 3 Stufen implementiert

### Self-Extend Feature (3 Stufen):
- ✅ **Stufe 1: self-skill-writer.md** – Bot verfasst neue Skills (HMAC signiert: `c4ac31d00026d5a456256efee149ed3cb35c7442fc587f22bd1b972430550c7a`)
- ✅ **Stufe 2: self-code-reader.md + handleSourceCode() API** – Bot liest eigenen Go-Code via whitelist-basiertem API-Handler (HMAC signiert: `372e012a6f8621c13df7ded1cdd5e91ead9e717b4426b031dbda1dc62246f992`)
- ✅ **Stufe 3: self-code-extend.md** – Bot generiert Code-Patches (HMAC signiert: `790c3090ce0e460e1735b3b8e4f1681d4790b1fa2933cd15c5661283fa0716e2`)

### Go-Code Änderungen:
- `pkg/dashboard/api.go` – `handleSourceCode()` Handler hinzugefügt (~70 Zeilen)
- `pkg/dashboard/server.go` – Route `/api/source` registriert

### Security Design:
- Whitelist: `pkg/`, `cmd/`, `go.mod`, `go.sum`, `Dockerfile`, `docker-compose.yml`
- Blockiert: `.git/`, `vault`, `secrets`, `.env`, `config.json`, `.sig`
- Directory-Traversal Schutz via `filepath.Abs()`
- Kein Auto-Deploy: User muss Code manuell einfügen + testen

### Dokumentation:
- `SESSION-40-SUMMARY.md` erstellt (vollständige Session-Dokumentation)
- `memory-md/01-features.md` – P9 + Self-Extend als DEPLOYED markiert
- `CLAUDE.md` – Aktueller Stand auf Session 40 aktualisiert

---

## Session 41 – AutoStart Fix + Dokumentation (2026-03-06)

**Fokus:** Kritischen AutoStart-Bug beheben (FluxBot stoppt bei PowerShell-Schließen)

### Problem:
JJ berichtet: "Wenn ich die Echse anklicke, läuft FluxBot. Wenn ich aber dann die PowerShell wieder zu mache, dann läuft FluxBot nicht mehr."

**Root Cause:** `INSTALL-Service.ps1` verwendet `sc.exe create` ohne WorkingDirectory. FluxBot crasht sofort beim Start als Service, weil er `config.json` nicht findet.

### Fix:
- ✅ **AUTOSTART-EINRICHTEN.ps1** (NEU) – Verwendet Windows Task Scheduler statt sc.exe
  - Task Scheduler unterstützt WorkingDirectory nativ
  - `Start-Process -WindowStyle Hidden` – kein sichtbares Fenster
  - Auto-Restart bei Absturz (3x, 1 Minute Interval)
  - Desktop-Verknüpfung "FluxBot Dashboard" → öffnet Browser direkt
  - `-Deinstallieren` Switch für vollständige Entfernung
- ✅ **QUICK-START.txt** – Auf v1.2.0 aktualisiert (2 Zeilen statt ganzer Anleitung)
- ✅ **05-bugreports.md** – Bug #6 dokumentiert mit Root Cause + Fix

### Memory-Dateien aktualisiert:
- 📄 **03-session-log.md** – Session 40 + Session 41 dokumentiert
- 📄 **05-bugreports.md** – AutoStart-Bug #6 hinzugefügt
- 📄 **CLAUDE.md** – Aktueller Stand auf Session 41 aktualisiert

### Nächste Session (42):
- Option D: Chrome/Browser-Skill (Playwright/CDP)
- Option E: System-Testing (Cal.com, VT, OAuth2)
- Option B: Lucide Icons (deferred)

---

## Session 1
- AES-256-GCM Vault vollständig implementiert + migriert
- Dashboard lädt/speichert Secrets getrennt von Config
- Bug gefixt: cfg.Validate() lief vor applySecrets() → zweiter Start schlug fehl
- Tailscale VPN-Sidecar integriert, Port auf 127.0.0.1 gebunden
- .env Datei erstellt (Tailscale Auth-Key eingetragen)
- Cal.com Skill auf flexible Platzhalter umgestellt (cal.com + cal.eu)
- Info-Button ⓘ im Dashboard für Platzhalter-Erklärung
- CLAUDE.md erstellt

---

## Session 2 – Priorität 1: VirusTotal auf alle Kanäle
- `pkg/security/vt.go`: ScanURL, ScanURLsInText, ExtractURLs, VTFileBlockedMsg, VTURLBlockedMsg, IsEnabled()
- `pkg/channels/telegram.go`: scannt Voice, Audio, Document, Photo, Video, VideoNote + URLs in Text
- `pkg/channels/discord.go`: scannt alle Anhänge (alle MIME-Typen) + URLs in Text; Download-Logik auf memory-first umgestellt
- `pkg/channels/slack.go`: scannt file_share Events (Bot-Token Auth Download) + URLs in Text; slackFile-Struct hinzugefügt
- `pkg/channels/matrix.go`: verarbeitet m.image/m.video/m.audio/m.file Events, lädt mxc:// URLs herunter + scannt; URL-Scan für Text
- `pkg/channels/whatsapp.go`: Media-Download über Meta Graph API (2-Schritt), scannt Audio/Bild/Dokument/Video/Sticker + URLs in Text

---

## Session 3 – Priorität 2: HMAC Dashboard-API
- `pkg/dashboard/server.go`: HMAC-SHA256 Middleware `hmacVerify()` für POST/PUT/DELETE-Endpoints
- `pkg/dashboard/server.go`: Replay-Schutz via `X-Timestamp` (Unix-Sekunden, ±5 Minuten Toleranz)
- `pkg/dashboard/server.go`: `GET /api/hmac-token` Endpoint (liefert Secret nach Basic Auth)
- `pkg/dashboard/server.go`: `UpdateHMACSecret()` für Hot-Reload
- `pkg/dashboard/dashboard.html`: `initHMAC()` – SubtleCrypto HMAC-Key Import beim Start
- `pkg/dashboard/dashboard.html`: `signPayload()` – Browser-seitiges HMAC-SHA256 Signing
- `pkg/dashboard/dashboard.html`: `api()` – automatisches Signing bei mutierenden Requests
- `cmd/fluxbot/main.go`: `FLUXBOT_HMAC_SECRET` Env-Variable an `dashboard.New()` übergeben + Hot-Reload

**Besprochen (noch nicht implementiert):**
- Ollama-Integration als lokaler AI-Provider (Priorität 7) – spart OpenRouter-Kosten
- Secret-Strategie: HMAC_SECRET soll in den Vault (nicht .env), Keyring für lokale Installation geplant

---

## Session 4 – README + Assets
- `assets/` Ordner erstellt – alle Logos mit sauberen Namen (`fluxion-logo.png`, `fluxion-character.png`, `virustotal-logo.png`, `bitwarden-logo.png`, `kiwerke-logo.png`)
- `README.md` vollständig neu gestaltet: Header mit VirusTotal (links) + FluxBot-Logo (Mitte) + Bitwarden (rechts), alle Abschnitte als echte Markdown-Tabellen, Roadmap + Sicherheits-Tabelle aktualisiert
- Originale Logos im Root noch vorhanden → per `git rm` entfernen sobald `assets/` committed ist

---

## Session 5 – Priorität 3: VT Dashboard Tab
- `pkg/security/vt.go`: `ScanEntry` + `VTStats` Structs; `recordScan()`, `GetStats()`, `GetHistory()`, `ClearHistory()`
- `pkg/security/vt.go`: History-Tracking in `ScanFileHash()` (incl. Cache-Hits) und `ScanURL()`; Stats-Zähler für Dateien/URLs/Geblockte/Cache
- `pkg/dashboard/api.go`: `handleVTStatus()` (Stats+Status), `handleVTHistory()` (letzte 100 Scans), `handleVTClear()` (Reset, HMAC-geschützt)
- `pkg/dashboard/server.go`: Routen `GET /api/vt/status`, `GET /api/vt/history`, `POST /api/vt/clear` registriert
- `pkg/dashboard/dashboard.html`: Sidebar-Eintrag 🛡️ VirusTotal; komplette Section mit Status-Badge, 5 Statistik-Karten, History-Tabelle (Zeit/Typ/Ziel/Ergebnis), Inaktiv-Banner, Info-Box; JS: `loadVTData()`, `renderVTStats()`, `renderVTHistory()`, `clearVTHistory()`

---

## Session 6 – Priorität 6: VT API-Key im Integrationen-Tab
- `pkg/dashboard/dashboard.html`: Eigener VT-Panel im Integrationen-Tab (zwischen SMTP und generischen Keys)
- `pkg/dashboard/dashboard.html`: Rotes Wichtigkeits-Banner ("Erforderlich für sicheren Bot-Betrieb")
- `pkg/dashboard/dashboard.html`: Passwort-Feld für VIRUSTOTAL_API_KEY mit Eye-Toggle
- `pkg/dashboard/dashboard.html`: Gelber ⚠️-Badge → Grüner ✅-Badge (live per `updateVTIntegBadge()`)
- `pkg/dashboard/dashboard.html`: Info-Card rechts ergänzt (VT, 500 API-Calls/Tag, kostenlos)
- `pkg/dashboard/dashboard.html`: `loadConfig()` lädt VT-Key + Badge; `saveConfig()` speichert VT-Key in Vault

---

## Session 7 – Priorität 5: Hilfe-System im Dashboard
- `pkg/dashboard/dashboard.html`: Sidebar-Eintrag ❓ Hilfe; alle 6 Sidebar-Items mit ⓘ Info-Button
- `pkg/dashboard/dashboard.html`: CSS für `.info-btn`, `#info-tooltip`, `.help-*` (Accordion, Search, Table, Code, Tags)
- `pkg/dashboard/dashboard.html`: Komplette `#section-help` mit Suchfeld + 6 Accordion-Panels:
  1. Dashboard-Überblick (Tabelle aller Bereiche)
  2. Vault & Sicherheit (AES-256-GCM, Hot-Reload, Docker vs. Keyring)
  3. Platzhalter-System ({{NAME}} → INTEG_NAME Erklärung)
  4. Skill-Signatur-Workflow (Python-Snippet, wann neu signieren)
  5. Kanäle & Vault-Schlüssel (Token-Namen je Kanal als Tabelle)
  6. Häufige Fehler & Lösungen (HMAC, CRLF Hook, VT, Cache, Git Push)
- `pkg/dashboard/dashboard.html`: `tipShow(btn, text)` + `tipHide()` – viewport-bewusstes Tooltip-Positioning
- `pkg/dashboard/dashboard.html`: `helpToggle(item)` + `helpSearch(query)` – Accordion + Echtzeit-Suche mit data-keywords

---

## Session 8 – Priorität 7: Ollama Integration
- `pkg/provider/ollama.go`: `OllamaProvider` Struct mit `Complete()`, `Name()`, `PingOllama()`; `OllamaDefaultBaseURL` Konstante
- `pkg/config/config.go`: Ollama-Modell-Defaults in `Load()` (llama3.2 / llama3.2-vision)
- `cmd/fluxbot/main.go`: Expliziter `"ollama"` Case im Provider-Switch; OLLAMA_BASE_URL direkt aus Vault; PROVIDER_OLLAMA in extractSecrets/applySecrets; "ollama" in getProviderModels()
- `pkg/dashboard/dashboard.html`: `#dash-ollama-row` mit Endpoint-URL + Modell-Name; `onDashProviderChange()` blendet Ollama-Row + API-Key-Feld ein/aus; `loadConfig()` liest OLLAMA_BASE_URL aus Vault; `saveConfig()` schreibt OLLAMA_BASE_URL + Modell

---

## Session 9 – Bugfixes aus INBOX
- `README.md`: `ki-werke.de` → `kiwerkepro.com` (alle 2 Vorkommen); Install-URLs auf `fluxbot.kiwerke.com` gesetzt
- `pkg/agent/agent.go`: `isForgetCommand()` – erweitert um `entferne`, `entfernen`, `delete`, `remove`
- `pkg/agent/agent.go`: `extractForgetKeyword()` – Präfixe korrekt nach Länge sortiert (längste zuerst), damit „lösche 1" nicht als „e 1" geparst wird; neue Kurzformen `lösche`, `entferne`, `delete`, `remove` ergänzt
- `pkg/skills/loader.go`: `parseSkillFile()` – strippt äußeren ` ```markdown ``` `-Wrapper vor dem Frontmatter-Parsing; dadurch werden Tags (inkl. „kalender") und Name aus dem Frontmatter korrekt gelesen
- **Root Cause Kalender:** `calcom-termine.md` war in ` ```markdown ``` ` gewickelt → Frontmatter wurde nie geparst → Tags-Fallback war nur `[calcom, termine]` → Skill matchte nicht auf „Kalender/kalendereinträge" → KI antwortete aus Training heraus negativ. Fix behebt das ohne Skill-Datei anzufassen (Signatur bleibt gültig).

---

## Session 10 – Kalender Hot-Reload-Bug
**Root Cause:** `CALCOM_BASE_URL` fehlte in `workspace/config.json` → `applySecrets()` holte nur Einträge die bereits in `cfg.Integrations` standen → Platzhalter blieb unersetzt → AI las `{{CALCOM_BASE_URL}}` als Literal und meldete "nicht konfiguriert"

**Root Cause 2:** `onReload()` in `main.go` rief nie `skillsLoader.SetIntegrations()` + `Reload()` auf → nach Dashboard-Speichern blieben alte Skills (mit unresolvierten Platzhaltern) aktiv

- `workspace/config.json`: `CALCOM_BASE_URL` als Integration hinzugefügt (neben `CALCOM_API_KEY`)
- `pkg/skills/loader.go`: `Reload()` Methode hinzugefügt (setzt `l.skills` zurück + ruft `loadAll()` neu auf)
- `cmd/fluxbot/main.go`: `onReload()` erweitert – nach Config-Reload werden Integrationen neu gebaut, `skillsLoader.SetIntegrations()` + `skillsLoader.Reload()` aufgerufen
- `INBOX.md` geleert

---

## Session 11 – Cal.com Skill: Defaults + natürliche Sprache
- `workspace/skills/calcom-termine.md`: Skill komplett überarbeitet – `eventTypeId`, `email`, `name`, `timeZone` werden NIEMALS beim Nutzer erfragt, kommen als Platzhalter-Defaults
- Neue Platzhalter: `{{CALCOM_EVENT_TYPE_ID}}`, `{{CALCOM_OWNER_EMAIL}}`
- Tags erweitert: `kalendereintrag`, `appointment` ergänzt
- `.sig` entfernt (Skill wurde geändert → läuft ohne Signatur mit Log-Warnung bis zur Neusignierung)
- `workspace/config.json`: `CALCOM_BASE_URL`, `CALCOM_EVENT_TYPE_ID`, `CALCOM_OWNER_EMAIL` als Integrationen ergänzt

**Was JJ noch im Dashboard → Integrationen eintragen musste:**
- `CALCOM_EVENT_TYPE_ID` = Event Type ID aus cal.com → Event Types (Zahl, z.B. 123456)
- `CALCOM_OWNER_EMAIL` = eigene E-Mail (z.B. kiwerkepro@gmail.com)

---

## Session 12 – Dashboard-Fixes aus Fehlerbildern
- `pkg/dashboard/dashboard.html`: Placeholder `sk-…` in generischen Integrationsfeldern → `dein Wert` (weniger verwirrend)
- `pkg/dashboard/dashboard.html`: Label `Key / Token` → `Wert` (neutraler)
- `pkg/dashboard/dashboard.html`: `showIntegrationHelp()` – springt jetzt direkt ins Hilfe-Panel + öffnet Platzhalter-Accordion (statt Modal)
- `pkg/dashboard/dashboard.html`: Hilfe-Panel „Platzhalter-System" komplett neu geschrieben – 5-Schritte-Anleitung + Beispiel-Tabelle mit realen Werten (CALCOM_EVENT_TYPE_ID, CALCOM_OWNER_EMAIL etc.), kein technischer Vault-Schlüssel mehr sichtbar

---

## Session 13 – Dashboard UX-Überarbeitung aus Fehlerbildern
- `pkg/dashboard/dashboard.html`: Accordion-Bug gefixt – `showIntegrationHelp()` nutzt jetzt `classList.add('open')` statt `style.display='block'` → kein permanentes Offenbleiben mehr
- `pkg/dashboard/dashboard.html`: Dedizierter **Cal.com-Panel** im Integrationen-Tab (wie VirusTotal-Panel) mit freundlichen deutschen Labels
- `pkg/dashboard/dashboard.html`: `loadConfig()` + `saveConfig()` + `updateCalcomBadge()` für Cal.com-Felder (direkt in Vault: `CALCOM_BASE_URL`, `CALCOM_API_KEY`, `CALCOM_OWNER_EMAIL`, `CALCOM_EVENT_TYPE_ID`)
- `pkg/dashboard/dashboard.html`: Generische Integrationen-Panel-Beschriftung vereinfacht (kein `{{PLATZHALTER_NAME}}` mehr sichtbar)
- `pkg/dashboard/dashboard.html`: Hilfe-Panel „Platzhalter-System" → „Weitere Integrationen" umbenannt und komplett neu geschrieben
- `workspace/skills/calcom-termine.md`: Event Type ID wird automatisch via `GET /event-types` ermittelt wenn nicht konfiguriert
- `workspace/config.json`: CALCOM_*-Einträge aus generischen Integrationen entfernt (werden jetzt direkt im Cal.com-Panel gespeichert)

---

## Session 14 – Dashboard-Fixes aus Fehlerbild 20260222_150607
- `pkg/dashboard/dashboard.html`: Globale CSS-Regel `a { color: var(--accent); }` + `a:hover { color: #8bb4f8; }` – alle Links im Dashboard jetzt einheitlich hellblau
- `pkg/dashboard/dashboard.html`: `input[type="email"]` bekommt explizit `background: var(--input-bg) !important` – kein weißer Browser-Default-Hintergrund mehr beim E-Mail-Feld
- `pkg/dashboard/dashboard.html`: Cal.eu-Link ergänzt neben Cal.com-Link (API-Key erstellen)
- `pkg/dashboard/dashboard.html`: Event Type ID ist kein `<input>` mehr, sondern ein nicht-klickbares Info-Display – „✓ Wird automatisch von FluxBot ermittelt"
- `pkg/dashboard/dashboard.html`: `loadConfig()` + `saveConfig()` – `CALCOM_EVENT_TYPE_ID` vollständig entfernt (kein Vault-Key mehr, keine UI)

---

## Session 15 – Dashboard-Fixes aus Fehlerbildern 154412 + 154655
- `pkg/dashboard/dashboard.html`: Sidebar-Footer `v1.0` → `v1.1.1`
- `pkg/dashboard/api.go`: `Version: "1.0.0"` → `"1.1.1"`
- `pkg/dashboard/dashboard.html`: API-Adresse (Cal.com) ist jetzt ein `<select>`-Dropdown – „Cal.com" oder „Cal.eu" wählbar; kein Freitext mehr
- `pkg/dashboard/dashboard.html`: Platzhalter-Name in „Weitere Integrationen" – Beispiel `CAL_API_KEY` → generisches `MEIN_SERVICE`; Beschreibungs-Label → „Bezeichnung (optional)"; Placeholder → `z.B. Mein Dienst – API Key`

---

## Session 16 – Cal.com Integration Bugfix (3 Root Causes)

**Root Cause 1 – applySecrets() ignorierte CALCOM_*:**
Das Cal.com-Dashboard-Panel (Session 13) speichert Werte im Vault als `CALCOM_BASE_URL`, `CALCOM_API_KEY`, `CALCOM_OWNER_EMAIL` (kein `INTEG_`-Prefix). `applySecrets()` kannte diese Keys nicht → sie landeten NIE in `cfg.Integrations` → Skills Loader substituierte `{{CALCOM_BASE_URL}}` nie.

**Root Cause 2 – Startup: skillsLoader.Reload() fehlte:**
`NewLoader()` lädt alle Skills mit leeren Integrations (weil `SetIntegrations()` erst danach aufgerufen wird). Ohne `Reload()` nach `SetIntegrations()` bleiben alle `{{PLATZHALTER}}` unsubstituiert bis zum ersten Dashboard-Save. Betrifft ALLE Integrationen, nicht nur Cal.com.

**Root Cause 3 – veraltete .sig blockierte Skill:**
Skill wurde in Sessions 11+13 geändert, `.sig` war noch die alte. → `verifySkill()` hat Skill als "manipuliert" geblockt → Skill wurde gar nicht geladen.

**Fixes:**
- `cmd/fluxbot/main.go`: `applySecrets()` – nach `INTEG_*`-Loop: CALCOM_* aus Vault in `cfg.Integrations` injizieren (add/update)
- `cmd/fluxbot/main.go`: Startup-Pfad – `skillsLoader.Reload()` nach `skillsLoader.SetIntegrations()` ergänzt
- `workspace/skills/calcom-termine.md.sig` – neu generiert mit aktuellem SKILL_SECRET

---

## Session 17 – Priorität 8: Google Workspace Integration
Vollständige Implementierung von Google Calendar, Docs, Sheets, Drive, Gmail.
Details siehe `memory-md/01-features.md` → PRIORITÄT 8.

---

## Session 18 – Skill-Neusignierung calcom-termine.md
- `workspace/skills/calcom-termine.md.sig` – neu generiert via Python-Script
- Vault automatisch entschlüsselt, SKILL_SECRET extrahiert, Signatur verifiziert ✅
- Neue Signatur: `0cd0cebc2dd2c5977b0b2094e48b9335f50308d6a0b29a2efb1174a9370f320a`

---

## Session 19 – Keyring-Abstraktionsschicht
- `pkg/security/keyring.go`: `SecretProvider`-Interface erweitert (`MigrateFromConfig`), `IsDockerEnvironment()`, `KeyringProvider`, `ChainedProvider`, `NewSecretProvider()` Factory, `allKnownKeys()`
- `pkg/security/keyring_windows.go`: Windows Credential Manager via `syscall` – `CredReadW`, `CredWriteW`, `CredDeleteW`, `CredEnumerateW` (kein CGo, keine externen Abhängigkeiten)
- `pkg/security/keyring_other.go`: Stub für Linux/macOS (`//go:build !windows`), `errKeyringUnsupported`
- `pkg/security/secrets.go`: `SecretProvider` Interface um `MigrateFromConfig()` ergänzt
- `cmd/fluxbot/main.go`: `NewSecretProvider()` statt `NewVaultProvider()`; HMAC-Secret aus Provider (Vault-Key `HMAC_SECRET`) mit Env-Fallback; `applySecrets()` + `buildGoogleClient()` auf `SecretProvider` Interface
- `pkg/dashboard/server.go`: `vault security.SecretProvider` statt `*security.VaultProvider`; neue Route `/api/secrets/backend`
- `pkg/dashboard/api.go`: `handleSecretBackend()` – liefert Backend-Name + `isDocker`-Flag
- `pkg/dashboard/dashboard.html`: Secret-Backend-Badge im Status-Tab (🗝️ WinCred grün / 🏦 Vault blau / ⚠️ nicht verfügbar gelb)
- Build-Verifikation: `GOOS=linux` ✅ + `GOOS=windows` ✅ (beide sauber ohne Fehler)

**Nächste Schritte (nach Session 19):**
1. Docker-Rebuild: `docker compose down; docker compose up -d --build`
2. Dashboard → Status: Secret-Backend-Badge prüfen (Docker = 🏦 AES-256-GCM Vault)
3. Lokal auf Windows: Badge sollte 🗝️ Windows Credential Manager zeigen
4. Optional: HMAC_SECRET via Dashboard → Secrets als Vault-Key `HMAC_SECRET` eintragen → Env-Variable `FLUXBOT_HMAC_SECRET` in `.env` kann danach entfernt werden

---

## Session 20 – Planung Dashboard Redesign
- Vollständige Planung + Spezifikation des Dashboard-Redesigns erarbeitet
- Komplette Analyse der dashboard.html (2635 Zeilen) durchgeführt
- Alle 5 Fehlerbilder aus Z-FEHLERBILDER/ gelesen und ausgewertet
- Implementationsplan in CLAUDE.md dokumentiert
- dashboard.html noch NICHT geschrieben (nächste Session)
- Redesign-Spezifikation → siehe `memory-md/04-redesign-spec.md`

---

## Session 21 – Ordnerstruktur + CLAUDE.md Auslagerung
- `memory-md/` Ordner erstellt ✅
- `.gitignore` um `memory-md/` ergänzt ✅
- CLAUDE.md in thematische Dateien aufgeteilt:
  - `memory-md/01-features.md` – Implementierte Features + Offene Punkte
  - `memory-md/02-architektur.md` – Architektur-Entscheidungen + Secret-Strategie
  - `memory-md/03-session-log.md` – Dieses Dokument
  - `memory-md/04-redesign-spec.md` – Dashboard Redesign Spezifikation
- CLAUDE.md bleibt als schlanke Index-Datei im Root

---

## Session 22 – Dashboard Redesign (P1)
- INBOX.md geleert (Notizen verarbeitet)
- Fehlerbilder aus Z-FEHLERBILDER/ analysiert + Auto-Cleanup-Regel in CLAUDE.md definiert
- Lucide Icons als P8.5 in `memory-md/01-features.md` dokumentiert
- `.sig` Dateien aus Skills-Liste gefiltert
- Skill-Namen aussagekräftig gemacht
- „Alle Skills neu laden" mit Feedback-Message versehen

---

## Session 23 – Testing P2 (Block 4+5) + Dashboard Login-System

**Block 5 – SOUL.md Verifikation:** 4/4 Tests bestanden ✅
- Go 1.22 korrekt beantwortet, robfig/cron genannt, Node.js abgelehnt, Politik-Absage ruhig + ohne Moralisieren

**Dashboard Login-System komplett neu gebaut:**
- `pkg/dashboard/server.go`: `username` Feld + `usernameMu` hinzugefügt
- `pkg/dashboard/server.go`: `New()` Signatur um `username string` erweitert (Default: "admin")
- `pkg/dashboard/server.go`: `UpdateUsername()` für Hot-Reload
- `pkg/dashboard/server.go`: `/` Route ist jetzt öffentlich (kein Auth auf HTML)
- `pkg/dashboard/server.go`: `GET /api/auth/check` – Credentials prüfen (200/401)
- `pkg/dashboard/server.go`: `GET /api/auth/recover` – Passwort-Wiederherstellung, NUR von 127.0.0.1
- `pkg/dashboard/server.go`: `auth()` prüft jetzt Benutzername + Passwort (vorher nur Passwort)
- `pkg/config/config.go`: `Username string` Feld in `DashboardConfig`
- `cmd/fluxbot/main.go`: `DASHBOARD_USERNAME` in `applySecrets()`, `extractSecrets()`, Hot-Reload
- `pkg/dashboard/dashboard.html`: Custom Login-Overlay (ersetzt Browser-nativen Basic-Auth-Dialog)
- `pkg/dashboard/dashboard.html`: `doLogin()`, `doLogout()`, `showRecovery()`, `applyRecovery()`
- `pkg/dashboard/dashboard.html`: `api()` schickt Authorization-Header mit, 401 → Login-Overlay
- `pkg/dashboard/dashboard.html`: „Passwort vergessen?" → ruft `/api/auth/recover` auf (nur localhost)
- `pkg/dashboard/dashboard.html`: „⎋ Abmelden" Button unten in der Sidebar
- `pkg/dashboard/dashboard.html`: Danger Zone → neues Feld „Dashboard-Benutzername" (DASHBOARD_USERNAME)
- Vault-Key: `DASHBOARD_USERNAME` (Default: admin, konfigurierbar im Dashboard)

---

## Sessions 24–27 – Google Vertex AI TTS (Chirp 3 HD) Implementation

**Gesamtproblem:** Google Cloud TTS und Vertex AI TTS unterstützen KEINE API Keys – nur OAuth2 Bearer Token. Mehrfache Endpoint-Versuche führten zu 401/403/404 Fehlern.

**Session 24:** Erste OAuth2-Implementierung, 401 "API keys not supported"
**Session 25:** Vertex AI `:predict` Endpoint versuchte, 404/401 Fehler
**Session 26:** Erkannt, dass TTS immer OAuth2 braucht, kein API Key möglich
**Session 27:** Vollständige OAuth2 mit Token-Cache in `tts_google.go` + `tts_vertex.go`, `cloud-platform` Scope zu `AllScopes` hinzugefügt, Dashboard-Button "Google-Konto verbinden" ins Google-Tab verschoben

**Gelöstes Problem (Session 28):**
- `pkg/dashboard/api.go`: `handleGoogleAuthURL()` hatte hardcodierte Scope-Liste ohne `cloud-platform` → Fixed: nutzt jetzt `google.AllScopes`
- `pkg/voice/tts_vertex.go`: Endpoint von `:predict` zu `texttospeech.googleapis.com/v1beta1/text:synthesize` geändert (Standard Cloud TTS, kein Vertex AI Predict-Format)
- `pkg/voice/tts_vertex.go`: Default-Stimme `de-AT-Chirp3-HD-Aoede` → `de-DE-Chirp3-HD-Aoede` (de-AT Stimmen existieren auf Cloud TTS nicht)
- OAuth2 `cloud-platform` Scope erfolgreich aktiviert
- Sprachnachrichten funktionieren, werden als Voice-Messages gesendet (Auto-Play auf Android via Proximity-Sensor)

---

## Session 28 – Google TTS Finalisierung

**Abgeschlossen:**
- Google Cloud TTS (Chirp 3 HD) vollständig funktionsfähig ✅
- OAuth2 mit `cloud-platform` Scope funktioniert
- Sprachnachrichten senden funktioniert (Telegram Voice-Message)

---

## Session 29 – TTS: Bare URLs + Links als Text-Followup (2026-02-27)

**Problem:** TTS las Sternchen/Asterisken und komplette URLs vor.

**Root Causes:**
- `stripMarkdownForTTS()`: behandelte `[Link](url)` korrekt, aber **bare URLs** (`https://...`) wurden nie angefasst → TTS las sie vollständig vor
- `strings.Map` entfernte zwar verbleibende `*`, aber URLs blieben unangetastet

**Fixes in `pkg/agent/agent.go`:**
- `stripMarkdownForTTS()`: Bare URL-Regex `https?://\S+` → werden komplett entfernt (nach dem Markdown-Link-Handler)
- `sendTTSReply()`: URLs werden **vor** dem Strippen aus dem Original-Text extrahiert (dedupliziert, trailing Punctuation abgeschnitten) → nach erfolgreichem Voice-Send werden sie als separate Text-Nachricht mit `🔗`-Prefix nachgeschickt

**Verhalten danach:**
- Fluxi liest keine Links vor
- Falls Links in der Antwort waren → kommen sie automatisch als Text-Nachricht direkt nach der Sprachnachricht

---

## Session 30 – Release v1.1.4 + INBOX-Verarbeitung

**Highlights:**
1. ✅ **v1.1.4 Release durchgeführt:**
   - Version in `cmd/fluxbot/main.go` erhöht (`v1.1.3` → `v1.1.4`)
   - Docker Rebuild erfolgreich
   - `git push origin main` + Release-Tag `v1.1.4` gepusht
   - GitHub Release ist live

2. ✅ **CLAUDE.md optimiert:**
   - "Aktueller Stand" stark gekürzt (war ~25 Zeilen, ist jetzt 5 Zeilen)
   - Redundante "Nächster Release" Zeile aus Versioning-Konvention entfernt
   - Details verweisen jetzt auf `memory-md/` Dateien

3. ✅ **INBOX.md komplett verarbeitet:**
   - Alle Items aus Session 30 auf memory-md Dateien verteilt:
     - `04-redesign-spec.md`: 5 neue UI/UX-Punkte (15-19)
     - `05-bugreports.md` ✨ NEU: 3 Bugs dokumentiert
     - `06-feature-roadmap.md` ✨ NEU: 5 Features geplant
     - `02-architektur.md`: E-Mail-Server Multi-Server-Logik
     - `01-features.md`: Dashboard Release-Versioning (P8.7)
   - INBOX.md geleert + Verarbeitungs-Kommentar

4. ✅ **memory-md Structure erweitert:**
   - 03-session-log.md: Stand auf Session 30 aktualisiert

5. ✅ **Workflow-Dokumentation hinzugefügt:**
   - CLAUDE.md: Neuer Abschnitt "Claude-Workflow – Dokumentation nach Änderungen"
   - Mapping-Tabelle: Änderungstyp → memory-md Datei + Aktion
   - Automatische Erkennung neuer memory-md Dateien dokumentiert

---

## Session 31 – Bug-Fix Agenda (2026-03-04)

**Bug #1 – Kalender Wochentag-Widerspruch: ✅ GEFIXT & DEPLOYED**
- `pkg/agent/agent.go` buildSystemPrompt(): Neue "⚠️ DATUMS-VALIDIERUNG" Sektion hinzugefügt
- Bot prüft jetzt **proaktiv**, ob Wochentag + Datum übereinstimmen
- Fordert Bestätigung BEVOR Kalender-Anfrage verarbeitet wird
- v1.1.4 mit Fix ist deployed und läuft (2026-03-04 12:46:59)
- `memory-md/05-bugreports.md`: Status auf "✅ SESSION 30 GEFIXT & DEPLOYED" aktualisiert

**Bug #2 – Bot-Antworten teilweise falsch: ✅ GEFIXT, GETESTET & FUNKTIONIERT**
- Root Cause identifiziert: Google Calendar Events ohne `summary` (Title) führten zu leeren/malformed Einträgen
- Fix implementiert in `pkg/agent/agent.go`:
  - `handleGoogleCalList()`: Neue Validierung – filtert Events mit leerem Title
  - Events ohne Title werden komplett ignoriert (verhindert leere Einträge)
- **Test durchgeführt (Session 31):**
  - JJ hat Event ohne Titel im Google Calendar erstellt
  - Bot-Test: `"Welche Termine habe ich?"` → leerer Event wird gefiltert ✅
  - Andere Events werden normal angezeigt
- **Neue Idee erkannt:** Calendar-Cleanup Feature (Fluxi warnt vor leeren Events & löscht sie) → dokumentiert in `06-feature-roadmap.md`

**Bug #3 – Audit-Logs Format: ✅ VOLLSTÄNDIG GEFIXT & GETESTET**
- ✅ **Phase 1 – Struktur (Session 31):**
  - `AuditEntry` erweitert um: UserIntent, ErrorCode, ErrorMessage, Duration (ms)
  - Log-Format: `ms=XXX intent=XXX [ERROR: CODE] (Message)` Pattern
- ✅ **Phase 2 – Agent-Integration (Session 31):**
  - `pkg/security/guard.go`: `GetAuditLogger()` Getter-Methode
  - `pkg/agent/agent.go`: `auditLogger *security.AuditLogger` Feld + Initialisierung in New()
  - `handleGoogleCalList()`: Vollständig mit Audit-Logging implementiert
- ✅ **Phase 3 – Session-Integration (Session 31):**
  - `logGoogleAudit(session, intent, duration, errCode, errMsg)` Hilfsmethode
  - Alle 9 Google Handler: `session *Session` als erster Parameter
  - Dispatch-Stellen aktualisiert
- **Test-Ergebnis:**
  ```
  VORHER: [14:01:15] channel= user= type= len=0 ms=559 intent=Kalender-Anfrage
  NACHHER: [14:18:36] channel=telegram user=8597470652 type=google-api len=0 ms=523 intent=Kalender-Anfrage
  ```
- **Noch TODO (Session 32+):**
  - Andere Google Handler (CalCreate, DocsCreate etc.) mit `logGoogleAudit` ausstatten

**Neuer Bug #4 gefunden (Session 31):**
- TTS Fehler: `vertex tts: fehler 400: Sentence too long` bei langen Kalender-Antworten
- Workaround aktiv: Fallback auf Text
- Fix-Ideen dokumentiert in `05-bugreports.md`

**Letzter Release:** `v1.1.5` (Session 31 – Bugs #2 & #3 gefixt)

**Next Steps:**
1. 🎯 Bug #4: TTS Sentence-Split implementieren
2. 📋 Andere Google Handler mit `logGoogleAudit` ausstatten
3. 🎨 UI/UX Improvements: Dashboard Points 15-19 aus 04-redesign-spec.md

---

## Session 32 – 2026-03-04

**Thema:** Bug #4 TTS Fix (v2) + Calendar Date Filter + Response Format + Git Tag Cleanup

---

### Bug #4 – TTS "Sentence too long": ✅ GEFIXT (v2 – Newline-First Strategy)

**Fix v1 schlug fehl:**
Erster Ansatz: Split an `.!?` mit max 400 Zeichen. Chirp 3 HD definiert "Satz" als Text zwischen Satzenden-Satzzeichen – Kalender-Zeilen ohne Punkt wurden als ein langer Satz erkannt.
```
vertex tts: chunk 1/2 fehlgeschlagen: fehler 400: Sentence starting with: "Kalen" is too long.
```

**Root Cause (v2 erkannt):**
Kalender-Auflistungen haben Newlines aber KEINE Satzenden-Satzzeichen → Google Chirp sieht alles als einen Satz.

**Fix v2 implementiert in `pkg/voice/tts_vertex.go`:**
1. `const maxTTSChunkLen = 300` – konservativer Wert
2. `splitIntoTTSChunks(text, maxLen)` – neue Strategie:
   - **Phase 1:** Split an `\n` (Zeilenumbrüche)
   - **Phase 2:** Jeder Zeile ohne Satzzeichen `.!?` ein `.` anhängen
   - **Phase 3:** Zeilen zu Chunks ≤300 Zeichen zusammenfassen
   - Overflow: `splitLongSentence()` teilt extra-lange Einzelzeilen an `.!?,` dann Leerzeichen dann Hard-Cut
3. `speakChunk()` – ausgelagerter Single-API-Call-Helper
4. `Speak()` überarbeitet: splitIntoTTSChunks() → 1 oder N API-Calls → chained OGG/Opus (Telegram-kompatibel)

**Ergebnis:** ✅ Keine TTS-Fehler mehr in Logs. Docker rebuild erfolgreich.

---

### Neuer Bug – Calendar Date Filter: ✅ GEFIXT

**Problem:**
- Sprachnachricht → CC Termine Skill → korrekt gefilterter Tag ✅
- Textnachricht → GCal Skill → immer kompletter Kalender (nächste 10 Events) ❌

**Root Cause:**
`__GOOGLE_CAL_LIST__`-Marker hatte keinen Datums-Parameter → immer `CalendarList("primary", 10)`.

**Fix in 3 Dateien:**

`pkg/google/google.go`:
- `CalendarList()` delegiert jetzt an `CalendarListWithRange(calendarID, maxResults, timeMin, timeMax)`
- Neues `CalendarListWithRange()`: setzt `timeMin` + optional `timeMax` als RFC3339 URL-Parameter

`pkg/agent/agent.go` – `handleGoogleCalList(session, response)`:
- Parst optionalen JSON-Block `{"date":"YYYY-MM-DD"}` oder `{"dateFrom":"...","dateTo":"..."}`
- Ruft `CalendarListWithRange()` bei gesetztem Filter (maxResults=25), sonst `CalendarList()` (maxResults=10)
- Neues Marker-Format:
  ```
  __GOOGLE_CAL_LIST__
  {"date":"2026-06-01"}
  __GOOGLE_CAL_LIST_END__
  ```

`workspace/skills/g-cal.md` – komplett überarbeitet:
- Instruiert KI: bei Datums-Anfrage → JSON-Block mit `__GOOGLE_CAL_LIST_END__`
- Dokumentiert beide Formate (`date` und `dateFrom`/`dateTo`)
- Beispiele: "morgen", "heute", "nächste Woche" → KI berechnet Datum aus System-Prompt
- **Muss nach Rebuild neu signiert werden** (Dashboard → Skills → GCal → Neu signieren)

---

### Response Format – Natürliche Sprache: ✅ GEFIXT

**Problem:**
TTS las vor: *"Kalender Google Kalender deine Termine"* – unnatürlich und redundant.

**Fix in `pkg/agent/agent.go` – `handleGoogleCalList()`:**
- Entfernt: `"📅 *Deine nächsten Google Calendar Termine:*"` Header
- Neu (Single Event): `"Du hast einen Termin: *Titel* am 02.01.2026 um 15:04 Uhr."`
- Neu (Multiple): `"Deine nächsten Termine:\n\n• *Titel* – 02.01.2026 um 15:04 Uhr"`
- Neu (Mit Datum): `"Deine Termine am 01.06.2026:\n\n• ..."`
- Neu (Leer): `"Du hast keine bevorstehenden Termine."`
- `formatEventTime()` inline: RFC3339 → `"02.01.2006 um 15:04 Uhr"` (Europe/Vienna)

---

### Git & Release Cleanup: ✅ ERLEDIGT

**Problem:** `v1.1.4` und `v1.1.5` waren beide auf demselben Commit `ccf6789` getaggt. Bug #3 Fix-Commit (`4a6fd2c`) hatte kein Tag.

**Fix:**
- `v1.1.5` von `ccf6789` gelöscht und auf `4a6fd2c` (Bug #3 – Audit-Logs) verschoben
- `v1.1.4` bleibt auf `ccf6789` (TTS Bare URLs Fix) – korrekt ✅
- Session 32 Änderungen committed + `v1.1.6` getaggt und gepusht

**go.sum Fix:**
- `github.com/robfig/cron/v3` fehlte in `go.sum` → alle Release-Workflows schlugen fehl
- Fix: `go mod tidy` + `go.sum` committed → Release-Workflow läuft sauber

**Release-Stand:**
| Tag | Commit | Inhalt |
|-----|--------|--------|
| `v1.1.3` | `d25b1b8` | Version bump |
| `v1.1.4` | `ccf6789` | TTS – Bare URLs Fix |

---

## Session 35 – Docker-Abstieg: Native Windows Installation (2026-03-05)

**Thema:** FluxBot von Docker-Container auf natives Windows-Binary migrieren + Windows-Service Bugfix

### 🎯 Hauptziel: Native Windows Installation
- **Problem:** FluxBot lief nur im Docker-Container; keine native Windows-Unterstützung
- **Lösung:** Go-Binary direkt auf Windows kompilieren + als Windows-Service laufen lassen

### 📦 Implementation:
1. **Binary bauen:**
   ```powershell
   go build -ldflags="-X main.version=v1.1.8" -o fluxbot.exe ./cmd/fluxbot
   ```
   → Erfolgreicher Build ✅

2. **Service registrieren:**
   ```powershell
   .\fluxbot.exe --service install --config "C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT\workspace\config.json"
   ```
   → Erfolgreich installiert ✅

3. **Direkter Start (ohne Service) funktionierte perfekt** ✅
   - Dashboard lädt unter http://localhost:9090
   - Discord + Telegram Kanäle verbinden sich automatisch
   - Windows Credential Manager nutzt (statt Docker Vault)
   - Logs: `[Keyring] ✅ Initialisiert – Backend: Windows Credential Manager`

### 🐛 Bug gefunden & gefixt: Windows-Service Crash (EXIT CODE 1067)

**Root Cause:** Windows-Services starten mit Working Directory `C:\Windows\System32`, nicht mit dem EXE-Verzeichnis.
- Config hatte: `"workspace": "./workspace"` (relativ)
- Service suchte nach `C:\Windows\System32\workspace` → existiert nicht → CRASH

**Fix in `cmd/fluxbot/service_windows.go`:**
```go
// ── Working Directory auf das Verzeichnis der EXE setzen ─────────────────
// Windows-Dienste starten mit CWD = C:\Windows\System32.
// Relative Pfade funktionieren dann nicht.
if exe, err := os.Executable(); err == nil {
    exeDir := filepath.Dir(exe)
    if err := os.Chdir(exeDir); err != nil {
        log.Printf("[Service] Warnung: Working Directory konnte nicht gesetzt werden: %v", err)
    }
}
```

**Workflow für Service-Start:**
```powershell
# 1. Service deinstallieren
.\fluxbot.exe --service uninstall

# 2. Neu kompilieren mit Fix
go build -ldflags="-X main.version=v1.1.8" -o fluxbot.exe ./cmd/fluxbot

# 3. Service neu installieren
.\fluxbot.exe --service install --config "C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT\workspace\config.json"

# 4. Starten
sc start FluxBot

# 5. Status prüfen
sc query FluxBot
```

**Ergebnis nach Fix:**
```
STATE              : 4  RUNNING
WIN32_EXIT_CODE    : 0  (0x0)
```
✅ **Service läuft jetzt stabil!**

### 📝 Dokumentation aktualisiert:
- `memory-md/02-architektur.md`: Notiz hinzugefügt (Service-Mode Dokumentation)
- `CLAUDE.md`: Session 36 als nächste dokumentiert
- Fehlerbilder: `Z-FEHLERBILDER/` hat Auto-Cleanup (wird nach Verarbeitung gelöscht)

### 🎁 Nebenergebnis: go.mod & go.sum Klärung
- ✅ **Gehören ins GitHub** – Public Open-Source Dependencies + Sicherheits-Checksummen
- **go.mod:** Dependency-Definitionen (keine Secrets)
- **go.sum:** Kryptographische Hashes (Manipulations-Schutz)

### 🔧 Nächste Schritte (Session 36+):
1. Docker komplett abschalten oder als Optional-Service konfigurieren
2. `memory-md/01-features.md` aktualisieren (Native Windows als Feature)
3. Feature-Roadmap prüfen (nächste Features aus `06-feature-roadmap.md`)

---
| `v1.1.5` | `4a6fd2c` | Bug #3 – Google Audit-Logs |
| `v1.1.6` | Session 32 | Calendar date filter, TTS Chirp3 Fix, natürliche Antwortformatierung |
| `v1.1.7` | Session 33 | Audit-Logging für alle Google Handler vollständig |

**Letzter Release:** `v1.1.7` ✅

**Next Steps:**
1. 🎨 UI/UX Dashboard Points 15-19 aus 04-redesign-spec.md

---

## Session 33 – 2026-03-04

**Thema:** Audit-Logging für alle Google Handler vervollständigt (v1.1.7)

### Google Handler – Audit-Logging: ✅ VOLLSTÄNDIG (11/11 Handler)

**Geändert:** `pkg/agent/agent.go`

Folgende Handler mit `startTime + logGoogleAudit(Fehler + Erfolg)` ausgestattet:

| Handler | Intent |
|---------|--------|
| `handleGoogleCalCreate` | "Termin erstellen" |
| `handleGoogleCalList` | "Kalender-Anfrage" *(bereits Session 31)* |
| `handleGoogleDocsCreate` | "Dokument erstellen" |
| `handleGoogleDocsAppend` | "Dokument bearbeiten" |
| `handleGoogleDocsRead` | "Dokument lesen" |
| `handleGoogleSheetsCreate` | "Tabelle erstellen" |
| `handleGoogleSheetsRead` | "Tabelle lesen" |
| `handleGoogleSheetsWrite` | "Tabelle schreiben" |
| `handleGoogleDriveList` | "Drive-Suche" |
| `handleGmailSend` | "E-Mail senden" |
| `handleGmailList` | "E-Mails auflisten" |

**Zusätzliche Änderung:**
- `handleGmailSend` + `handleGmailList`: `session *Session` als ersten Parameter hinzugefügt (vorher fehlend)
- Dispatch-Calls für Gmail auf `(session, response)` aktualisiert

**Release:** `v1.1.7` ✅ – committed + gepusht + getaggt

**Next Steps:**
1. ~~🎨 UI/UX Dashboard Points 15-19 aus 04-redesign-spec.md~~ (verschoben)

---

## Session 34 – 2026-03-05

**Thema:** Dashboard Release-Versioning (P8.7) ✅

### Automatische Versionsnummer im Dashboard: ✅ VOLLSTÄNDIG

**Ziel:** Dashboard-Footer zeigt die echte Build-Version, ohne manuell im Code zu ändern.

**Strategie:** `var version = "dev"` in `main.go` + `-ldflags "-X main.version=vX.Y.Z"` beim Build.
Der Release-Workflow (`release.yml`) injizierte die Version bereits bei den Binaries, aber nicht beim Docker-Image, und das Dashboard verwendete sie gar nicht.

**Geänderte Dateien:**

| Datei | Änderung |
|-------|----------|
| `Dockerfile` | `ARG VERSION=dev` + ldflags beim `go build` |
| `cmd/fluxbot/main.go` | `var version = "dev"` (statt `"v1.1.5"` hardcoded) + `version` an `dashboard.New()` übergeben |
| `pkg/dashboard/server.go` | `version string` Feld in `Server`-Struct + Parameter in `New()` |
| `pkg/dashboard/api.go` | `Version: s.version` statt hardcodiertem `"1.1.1"` |
| `pkg/dashboard/dashboard.html` | Footer `<span id="footer-version">` + JS setzt Wert aus `/api/status` |

**Funktionsweise:**
- GitHub `release.yml` baut Binaries schon mit `-X main.version=${VERSION}` ✅
- GitHub `release.yml` übergibt `VERSION` bereits als Docker build-arg ✅
- Dockerfile überträgt das jetzt per ldflags in die Binary ✅
- `version` wird über `dashboard.New()` → `Server.version` → `handleStatus()` → `/api/status` ins Frontend geliefert
- `loadStatus()` (automatisch beim Login) setzt `#footer-version`
- Lokal (`docker compose up --build` ohne build-arg): Fallback auf `"dev"`

**Nächster Release:** `v1.1.8`

---

## Session 35 – 2026-03-05

**Thema:** GitHub Release Changelog (Auto-Generation) ✅

### Problem erkannt:
User: _"Außerdem habe ich gerade festgestellt, dass in den ganzen Releases immer nur die Erklärung ist, was wie zu installieren ist. Aber es wird nie irgendwo beschrieben, was geändert wurde."_

**Lösung:** Automatischer Changelog in `release.yml` (Git-Log Parsing)

**Geänderte Datei:**

| Datei | Änderung |
|-------|----------|
| `.github/workflows/release.yml` | Neue Step: "Changelog generieren" + Git-Log-Parsing |

**Neue Funktionalität:**

1. **fetch-depth: 0** – Vollständige Git-History abrufen (statt nur letzter 1-2 Commits)
2. **Letzten Tag auslesen** – `git describe --tags --abbrev=0` (oder "" wenn erstes Release)
3. **Changelog generieren** – gruppiert nach:
   - `feat:` → **✨ Features**
   - `fix:` → **🐛 Bugfixes**
   - Rest → **📋 Weitere Änderungen**
4. **Release-Body oben** – Changelog vor Installation (statt Installation allein)

**Beispiel-Output:**
```
### 📝 Changelog

Commits seit **v1.1.7**:

✨ Features:
- Implement automatic version numbering in dashboard (JJ)

🐛 Bugfixes:
- Fix version injection in Docker build (JJ)

📋 Weitere Änderungen:
- Update documentation (JJ)

---

### 🚀 Installation
[Installation-Anleitung hier...]
```

**Wichtig:** Conventional Commits (`feat:`, `fix:`) müssen eingehalten werden für korrektes Grouping.

**Nächster Release:** `v1.1.8` – wird automatisch Changelog anzeigen

---

## Session 36 – 2026-03-05

**Thema:** v1.1.8 Release durchgeführt – Native Windows Installation Support ✅

### 🎯 Gesamtpakete für v1.1.8:

**Session 35 Zusammenfassung:**
- **Native Windows Binary:** FluxBot läuft jetzt direkt auf Windows (ohne Docker)
- **Windows Service Integration:** `sc start/stop/query FluxBot`
- **Critical Bug Fix:** Working Directory in Service (`os.Chdir(exeDir)`)
- **Secrets-Strategie:** Windows Credential Manager (statt Docker Vault)
- **Dashboard:** Lädt via http://localhost:9090 oder http://fluxbot.TAILNET.ts.net:9090

### 📦 Release v1.1.8 durchgeführt:

```powershell
# Version aktualisiert
cmd/fluxbot/main.go: var version = "v1.1.8"

# Commit erstellt
git commit -m "Release v1.1.8: Native Windows Installation Support"

# Tag erstellt und gepusht
git tag -a v1.1.8 -m "Release v1.1.8: Native Windows Installation Support"
git push origin main
git push origin v1.1.8
```

**Commit:** 5490546
**Gepusht:** ✅ main + v1.1.8 Tag

### 🐛 Bugs behoben in v1.1.8:
1. **Windows Service Crash (EXIT CODE 1067)** – Working Directory Fix
2. **Relative Pfade in Service** – jetzt absolut via `os.Executable()`
3. **Docker-abhängiges Vault** – Native Windows Credential Manager

### 📝 Dokumentation aktualisiert:
- `memory-md/02-architektur.md`: Service-Dokumentation + Keyring-Abstraktionsschicht
- `CLAUDE.md`: "Aktueller Stand" auf v1.1.8 + Session 36 dokumentiert
- `memory-md/03-session-log.md`: Komplette Session 35 + 36 Einträge

### 🚀 Nächster Release: v1.1.9

**Next Steps für Session 37+:**
1. 🎨 Dashboard UI/UX Improvements (Points 15-19 aus 04-redesign-spec.md)
2. 📊 Feature-Roadmap Punkte abarbeiten (aus 06-feature-roadmap.md)
3. 🧪 Testing + Docker Abstieg optional machen (nicht mehr Pflicht)

---

## Session 37 – 2026-03-05

**Thema:** OpenClaw Referenz-Study für Security-Pattern-Analyse 🔒

### 🎯 Aktivitäten:

1. **OpenClaw Repository analysiert**
   - GitHub: github.com/openclaw/openclaw
   - Lokales Research-Verzeichnis: `O1000-OpenClaw-new/` in DEVELOPING
   - TypeScript/Node.js 22+ Projekt mit pnpm Monorepo

2. **Architektur-Study durchgeführt**
   - **src/** Struktur: 50+ Module (channels, security, pairing, secrets, gateway, providers, skills)
   - **Channels:** Telegram, Discord, Slack, WhatsApp, Signal, Matrix, iMessage, etc. (ähnlich FluxBot)
   - **Gateway:** WebSocket Control Plane (ws://127.0.0.1:18789) für lokale Koordination
   - **Skills:** ClawHub Registry + Bundled/Managed/Workspace Skills

3. **Security-Patterns dokumentiert**
   - ✅ **Trust-First Pairing Mode** (8-char Codes, 1h Timeout, max 3 offene Requests)
   - ✅ **Granulare DM-Policy** (Separate Group/DM Access Control mit Audit-Gründen)
   - ✅ **Comprehensive Auditing** (audit.ts, audit-extra.ts, audit-channel.ts)
   - ✅ **Skill Scanner** (Validierung vor Ausführung, Injection-Detection)
   - ✅ **Dangerous Tools Whitelist** (system.run, eval, File Deletion blockierbar)

4. **Docker-Setup vorbereitet**
   - .env für Research erstellt (lokale Bindung nur)
   - Multi-Image Strategy dokumentiert (gateway, sandbox, sandbox-browser)
   - Docker in VM nicht verfügbar → Code-Analyse stattdessen durchgeführt

### 📝 Dokumentation erstellt:
- **`memory-md/07-openclaw-research.md`** – Umfassende Research-Dokumentation
  - Architektur-Überblick
  - Trust-First Design Pattern
  - DM-Policy System Details
  - Security Audit Framework
  - Key Learnings für FluxBot
  - Implementierungs-Roadmap (Phase 1-3)

- **CLAUDE.md** aktualisiert – Neue Dokumentation registriert

### 🚀 Key Findings für FluxBot:

**High Priority (P1-P3):**
1. **DM-Pairing Mode** – Unbekannte Telegram/Discord User brauchen Short-Code Approval
2. **Granulare DM-Policy** – Separate Access Control pro Channel/User/Scope
3. **Skill Security Audit** – Vor Ausführung scannen auf Injection/Dangerous Calls
4. **Dangerous Tools Whitelist** – Nur Admin darf `system.run`, File Delete, etc.

**Medium Priority (P4-P6):**
5. **Skill Scanner Framework** – Umfassende Validierung von Skill-Code
6. **Audit Logging System** – Alle Entscheidungen (allow/block/pairing) dokumentieren
7. **Sandbox Docker Images** – Skills in isolierten Containern ausführen
8. **Skill Registry** – Zentrale Verwaltung ähnlich ClawHub

### 📚 Analysierte Dateien:
- `src/pairing/pairing-store.ts` – Trust-Store mit Path Traversal Protection
- `src/security/dm-policy-shared.ts` – Granulare Access Control Logic
- `src/security/audit.ts` – Comprehensive Audit Framework
- `src/security/skill-scanner.ts` – Code-Validierung vor Ausführung
- `docker-compose.yml` – Multi-Service Orchestration
- `.env.example` – Secrets-Management Patterns

### 🔗 Verknüpfungen:
- `memory-md/07-openclaw-research.md` – Ausführliche Dokumentation
- `memory-md/06-feature-roadmap.md` – Roadmap aktualisieren (nächste Session)
- `memory-md/02-architektur.md` – Security-Patterns mit FluxBot architektur vergleichen (nächste Session)

### 🎯 Nächste Schritte (Session 38+):

**Phase 1: Security Foundation**
- [ ] DM-Pairing Mode für Telegram/Discord implementieren
- [ ] Granulare DM-Policy in `config.json` definieren
- [ ] `dangerous-tools.go` ähnlich OpenClaw's Pattern
- [ ] Update `memory-md/02-architektur.md`

**Phase 2: Skill Hardening**
- [ ] Skill-Scanner vor Ausführung integrieren
- [ ] Per-User Skill Restrictions
- [ ] Skill-Sandbox Docker Images

**Phase 3: Monitoring**
- [ ] Audit Dashboard im FluxBot UI
- [ ] Alert System für Security Violations
- [ ] Log Retention Policy

---

## Session 38 (2026-03-04)

**Focus:** P9 DM-Pairing Mode Implementierung

**Delivered:**
- ✅ `pkg/pairing/store.go` – Thread-safe JSON-Store (pending/approved/blocked)
- ✅ `pkg/channels/telegram.go` – 3-Tier Access Control (AllowFrom → PairingStore → fallback)
- ✅ `pkg/dashboard/api.go` – handlePairing (GET/POST), handlePairingStats
- ✅ `pkg/dashboard/server.go` – HMAC-geschützte Routen `/api/pairing`
- ✅ `pkg/dashboard/dashboard.html` – Pairing-Tab mit Stats, Filter, Tabelle
- ✅ `config.json` – Pairing-Section hinzugefügt

**Key Decisions:**
- Telegram User-ID statt 8-Zeichen Pairing-Code (einfacher, kein TTL)
- Design: Trust-First (unbekannte User müssen freigegeben werden)
- pairing.json im Workspace (Runtime-State, nicht im Vault)

**Status:** ✅ DEPLOYED – P9 live

---

## Session 39 (2026-03-04 bis 2026-03-05)

**Focus:** Windows Service Deployment + v1.1.9 Release

**Delivered:**
- ✅ INSTALL-Service.ps1 – Windows Service Installer (mit UTF-8 Emoji-Fixes)
- ✅ QUICK-START.txt – User-freundliche Anleitung
- ✅ START-FluxBot.ps1 – Simple exe-Starter
- ✅ Git Commit 736af0e – Alle Dateien gepusht
- ✅ Tag v1.1.9 – Release gesetzt
- ✅ Dashboard live auf http://localhost:9090
- ✅ Service status: Running, Auto-Start enabled

**Bugs Fixed:**
- UTF-8 Emoji in PowerShell → Rewrite ohne Emoji
- Git Lock File → Auto-Recovery, kein Fehler

**Documentation:**
- memory-md/03-session-log.md aktualisiert
- memory-md/01-features.md – P9 als DEPLOYED markiert
- CLAUDE.md – Release v1.1.9, Service Status

**Status:** ✅ PRODUCTION READY

---

## Session 40 (2026-03-05)

**Focus:** Option C – Self-Extend Feature (3 Stufen)

**Delivered:**

**Stufe 1 – Skill-Writer:**
- ✅ `workspace/skills/self-skill-writer.md` (HMAC signiert)
  - Bot verfasst neue Skill-Markdown-Dateien
  - Format erklärt: Frontmatter (YAML), Marker, Platzhalter
  - `__SKILL_WRITE__` Marker für Output
  - User kopiert in workspace/skills/ + signiert im Dashboard

**Stufe 2 – Code-Reader:**
- ✅ `pkg/dashboard/api.go` – `handleSourceCode()` Handler (60 Zeilen)
  - Query-Parameter: `file=pkg/agent/agent.go`
  - Whitelist: pkg/*, cmd/*, go.mod, go.sum, Dockerfile, docker-compose.yml
  - Blocked: .git/, vault/, secrets, .env, config.json, .sig
  - Security: Directory-Traversal schutz via `filepath.Abs()`
  - Response: JSON mit file, content, lines

- ✅ `pkg/dashboard/server.go` – Route registriert
  - `mux.HandleFunc("/api/source", s.auth(s.handleSourceCode))`
  - Auth erforderlich, kein HMAC (lesend, nicht kritisch)

- ✅ `workspace/skills/self-code-reader.md` (HMAC signiert)
  - `__SOURCE_READ__` Marker zum Code lesen
  - Erklärt Whitelist + Sicherheit
  - Use-Cases: Bug-Analyse, Feature-Fragen, Architektur-Verständnis

**Stufe 3 – Code-Extender:**
- ✅ `workspace/skills/self-code-extend.md` (HMAC signiert)
  - Bot generiert Code-Patches als `__CODE_PATCH__` Blöcke
  - Drei Typen: `new_file`, `modify`, `new_function`
  - Fields: type, file, description, code, instructions
  - Security: KEIN Auto-Deploy, manueller Review erforderlich
  - Best Practices dokumentiert (kleine Patches, Testing-Anleitung, etc.)

**Documentation Updates:**
- memory-md/01-features.md – P9 als DEPLOYED, Self-Extend als ✅ DEPLOYED
- Detailplan: memory-md/06-self-extend-spec.md

**Key Decisions:**
- API über /api/source (lesend, sicher)
- Whitelist-basiert (kein directory traversal möglich)
- Skill-basiert (flexible Anleitung für Bot + User)
- Sicherheit: Manueller Review vor Code-Deployment

**Status:** ✅ LIVE – Self-Extend Feature funktional

**Options Summary:**
- ✅ Option A (P9 Live-Testing): PASSED
- ✅ Option C (Self-Extend): COMPLETED
- ⏳ Option B (Lucide Icons): DEFERRED (nach C, D, E)
- ⏳ Option D (Chrome/Browser-Skill): TODO
- ⏳ Option E (System-Testing): TODO

---

## Session 41 (2026-03-06)

**Focus:** AutoStart Bug Fix + v1.2.0 Release

**Problem:**
- FluxBot startete nur, wenn PowerShell-Fenster offen blieb
- Sobald Fenster geschlossen → FluxBot crashte
- Root Cause: Windows Service (sc.exe) hatte keine WorkingDirectory-Option

**Delivered:**
- ✅ Neues Skript `AUTOSTART-EINRICHTEN.ps1` (Windows Task Scheduler statt Service)
  - Auto-Restart bei Absturz (3x, nach 1 Minute)
  - Trigger: AtLogon (automatisch beim Login)
  - Hidden Mode: `Start-Process -WindowStyle Hidden`
  - Mit WorkingDirectory: `C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT`

- ✅ Desktop-Verknüpfung "FluxBot Dashboard" für Benutzer
  - Öffnet direkt http://localhost:9090 im Browser

- ✅ `QUICK-START.txt` aktualisiert (v1.2.0)
  - 2 einfache Schritte: 1) PowerShell Admin, 2) AUTOSTART-EINRICHTEN.ps1 ausführen

**Status:** ✅ DEPLOYMENT – v1.2.0 released

---

## Session 42 (2026-03-06)

**Focus:** Browser Skills (Option D) – Web-Suche + Screenshot + Form-Fill

**Delivered:**

**Phase 1 – Web-Suche (Tavily API):**
- ✅ `pkg/search/search.go` – TavilySearcher Client
  - `Search(query, maxResults)` → JSON Results
  - Rate-Limiting: max 10 Anfragen/Minute eingebaut
  - Error-Handling: API-Fehler + Timeout (30s)

- ✅ `workspace/skills/web-search.md` (HMAC signiert)
  - `__WEB_SEARCH__` Marker zum Webs durchsuchen
  - Output: Titel, URL, Snippet
  - Vault-Key: `SEARCH_API_KEY` (Tavily API Key)

**Phase 2 – Browser-Steuerung (chromedp):**
- ✅ `pkg/browser/browser.go` – Browser Client
  - `New()`, `IsConfigured()`, `IsAllowed(url)`
  - `ReadPage(url)` → sichtbarer Text (max 4000 Zeichen)
  - `Screenshot(url)` → PNG Bytes (Full-Page)
  - `FillForm(url, fields, submitSelector)` → Text-Result
  - Browser-Typen: Chromium (default), Firefox, WebKit
  - Timeout: 60 Sekunden (konfigurierbar)

- ✅ Vier Browser-Skills:
  1. `workspace/skills/web-search.md` – Web durchsuchen (Tavily)
  2. `workspace/skills/browser-read.md` – Seite lesen (Text extrahieren)
  3. `workspace/skills/browser-screenshot.md` – Screenshot machen (PNG)
  4. `workspace/skills/browser-fill.md` – Formular ausfüllen + absenden
  - Alle HMAC signiert, Vault-Keys dokumentiert

**Vault-Keys (neu):**
```
SEARCH_API_KEY           ← Tavily Web-Suche
BROWSER_ENDPOINT         ← Chrome CDP Endpoint (z.B. ws://localhost:9222)
BROWSER_ALLOWED_DOMAINS  ← Whitelist (kommagetrennt, leer = alle)
```

**Deployment-Vorbereitung:**
- chromedp Abhängigkeit: `go mod tidy` + `go build -o fluxbot.exe ./cmd/fluxbot` nötig
- Browser-Binaries: chromedp downloaded beim ersten Start automatisch

**Status:** ✅ CODE READY – Skills implementiert und signiert

---

## Session 43 (2026-03-07)

**Focus:** Browser Screenshots Bug Debugging + Path-Fix

**Problem (Session 42 Fallout):**
- User: `"Mache einen Screenshot von bild.de"`
- Bot: `"Bildgenerierung ist aktuell nicht aktiviert"` ← **FALSCH!**
- Expected: Bot soll browser-screenshot Skill verwenden

**Root Cause Analysis:**
1. **`isImageRequest()` false positive:** Text "bild.de" enthält "bild" → Funktion gibt true zurück
2. **Missing Implementation:** `isBrowserContext()` wurde in Zeile 648 aufgerufen aber nicht implementiert
   - Code: `if a.isImageRequest(text) && !a.isBrowserContext(text) { ... }`
   - Result: Build schlägt fehl → alte Binary läuft ohne Fix

3. **Zusätzlicher Bug:** `/tmp/` Hardcodierung nicht Windows-kompatibel
   - Betroffen: `pkg/channels/utils.go`, `pkg/channels/discord.go`
   - Windows hat kein `/tmp/`, nur `%TEMP%` (AppData/Local/Temp)

**Fixes implementiert:**
1. ✅ Funktion `isBrowserContext()` implementiert (pkg/agent/agent.go Zeilen 2148–2167)
   - Keywords: screenshot, webseite, http://, https://, www., bild.de, browser, url, etc.
   - Logic: Gibt true zurück wenn Text Browser-Request enthält

2. ✅ Zeile 648 Guard: `if a.isImageRequest(text) && !a.isBrowserContext(text)`
   - Nur Bild-API aufrufen wenn NICHT browser-context

3. ✅ Windows Path-Fix:
   - `pkg/channels/utils.go` saveTempFile(): `/tmp/` → `filepath.Join(os.TempDir(), ...)`
   - `pkg/channels/discord.go` SaveTempFileFromData(): `/tmp/` → `filepath.Join(os.TempDir(), ...)`

**Documentation Updated:**
- `memory-md/05-bugreports.md` – Bug #7 Status aktualisiert

**Status:** 🔴 ZURÜCKGESTELLT BIS DEPLOYMENT-TEST
- Builds immer noch mit altem go.mod (chromedp) → neue Binary war nie erzeugt
- Nächste Aktion: Playwright-Migration durchführen (Session 44)

---

## Session 44 (2026-03-07)

**Focus:** Playwright-Migration + Browser-API-Fixes

**Problem (Session 43 Fallout):**
- go.mod hatte `playwright-go v1.45.0` (ungültig, nicht verfügbar)
- User versuchte `v1.44.0` (auch ungültig)
- `go mod tidy` schlägt fehl → Build nie erfolgreich → alte Binary läuft immer noch
- **Evidenz:** `go.sum` hatte 0 Playwright-Einträge

**Root Cause (Session 44 gelöst):**
1. Playwright wurde installiert mit: `go install github.com/playwright-community/playwright-go/cmd/playwright@latest`
   - Downloads v0.5700.1 (korrekte Version)
   - Browser binaries: `playwright install --with-deps` → Chromium, Firefox, WebKit, FFMPEG

2. Aber go.mod hatte immer noch alte/falsche Version
   - **Fix:** go.mod auf v0.5700.1 aktualisieren (die tatsächlich installierte Version)
   - `go mod tidy` durchgeführt → erfolgreich

**API-Inkompatibilitäten in browser.go gefunden & gefixt:**

**Build-Fehler vor Fix (8 Fehler):**
```
pkg\browser\browser.go:120: cannot use c.timeout.Milliseconds() (int64) as float64
pkg\browser\browser.go:144: undefined: playwright.WaitUntilLoadState
pkg\browser\browser.go:150: cannot use "networkidle" as PageWaitForLoadStateOptions
pkg\browser\browser.go:200: cannot use playwright.String("png") as *ScreenshotType
pkg\browser\browser.go:251: cannot use c.timeout.Milliseconds() (int64) as float64
pkg\browser\browser.go:265: WaitForSelector returns 2 values
... (weitere)
```

**Root Cause:**
- playwright-go v0.5700.1 API ist anders als alte Version
- Enums wie `WaitUntilStateNetworkidle` sind **bereits Pointer-Typen** (kein `&` nötig)
- Constructor-Funktionen wie `WaitUntilLoadState()` existieren nicht mehr
- Timeouts: int64 → float64 Konvertierung erforderlich
- WaitForSelector gibt 2 Werte zurück: (ElementHandle, error)

**8 Fixes in pkg/browser/browser.go:**

| Zeile | Problem | Alt | Neu |
|-------|---------|-----|-----|
| 120-121 | SetDefaultTimeout int64 | `c.timeout.Milliseconds()` | `float64(c.timeout.Milliseconds())` |
| 144, 186, 237 | WaitUntil Constructor | `WaitUntilLoadState("networkidle")` | `WaitUntilStateNetworkidle` (direkt) |
| 150, 193, 243, 270 | WaitForLoadState String | `page.WaitForLoadState("networkidle")` | `page.WaitForLoadState(PageWaitForLoadStateOptions{State: LoadStateNetworkidle})` |
| 210 | ScreenshotType String | `playwright.String("png")` | `playwright.ScreenshotTypePng` (direkt) |
| 251 | WaitForSelector Timeout | `playwright.Float(int64)` | `playwright.Float(float64(...))` |
| 265 | WaitForSelector Return | `if err := ...` | `if _, err := ...` (2 values) |

**Key Learning:**
- Enum-Konstanten in playwright-go v0.5700.1 sind bereits `*Type` (keine `&` nötig)
- Version-Locking in go.mod ist essentiell: `go get @latest` danach `go mod tidy`
- Playwright braucht Browser-Binaries: `playwright install --with-deps`

**Befixte Dateien:**
- `pkg/browser/browser.go` – Alle 8 API-Fehler behoben

**Status:** 🔄 BEREIT FÜR `go build`
- Nächste Aktion: `go build -o fluxbot.exe ./cmd/fluxbot` (sollte erfolgreich sein)
- Danach: Prozess neustarten + Screenshot-Test durchführen
- Wenn erfolgreich: Release v1.2.1+ markieren

---

## Session 45 – Browser Screenshot Bug: Weiter-Debugging (2026-03-07)

**Fokus:** Bug #7 weiter debuggen – "Mache einen Screenshot von bild.de" ergibt "Bildgenerierung nicht aktiviert"

**Build:** go build erfolgreich (Playwright-Migration aus Session 44 kompiliert)

**Analyse-Ergebnisse:**

1. **isImageRequest() False Positive bestätigt:**
   - Text "Mache einen Screenshot von bild.de": hasBild=true (wegen "bild" in "bild.de") + "von " = isImageRequest() TRUE
   - isBrowserContext() gibt TRUE zurueck (wegen "screenshot")
   - Debug-Log bestaetigt: `isBrowser=true isImage=false | Bedingung: false` = Code-Fix funktioniert

2. **Echtes Problem: Skill wird nicht gematcht**
   - Log zeigt `[Agent] Nachricht: Mache einen Screenshot von bild.de...` OHNE vorherige `[Agent] Skill: browser-screenshot` Zeile
   - Skill-Matcher findet browser-screenshot Skill NICHT
   - Vermutete Ursache: Skill-Datei editiert, Signatur ungueltig, Skill in invalidSkills statt skills

3. **Skill-Signatur-Problem:**
   - browser-screenshot.md wurde editiert (Tags und Inhalt geaendert)
   - .sig-Datei existiert (manuell von JJ signiert um 22:27)
   - Logs zeigen "21 aktiv, 0 benoetigen Neu-Signierung" - Signatur scheint gueltig
   - ABER: Skill wird trotzdem nicht gematcht (unklar warum)

4. **Prozess-Management-Problem (Windows):**
   - Mehrfach alte FluxBot-Prozesse gleichzeitig aktiv (bis zu 6 Instanzen)
   - Bash kann Windows-Prozesse nicht zuverlaessig killen
   - Port 9090 permanent blockiert durch alte Prozesse
   - Code-Aenderungen in neuer Binary kommen nie zum Einsatz weil alter Prozess die Nachrichten verarbeitet

**Code-Aenderungen (Session 45):**

1. `pkg/agent/agent.go` - `isBrowserContext()`: Erweitert um Domain-TLDs (.de, .com, .org, .net), "seite", "lies", "oeffne", "zeig mir"
2. `pkg/agent/agent.go` - `isImageRequest()`: Browser-Ausschluesse erweitert; generisches `hasBild && (von || zeig)` Pattern entfernt, nur noch explizite Trigger
3. `workspace/skills/browser-screenshot.md`: Tags bereinigt (entfernt: "foto", "bild", "zeig", "snip"), Aktivierung praezisiert

**Status:** NICHT GELOEST

**Naechste Aktion (Session 46):**
1. ALLE FluxBot-Prozesse in PowerShell stoppen: `Get-Process fluxbot | Stop-Process -Force`
2. Pruefen dass keine Prozesse mehr laufen: `Get-Process fluxbot` (sollte Fehler geben)
3. Neu starten: `Start-Process .\fluxbot.exe -WorkingDirectory "C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT" -WindowStyle Hidden`
4. Screenshot-Test durchfuehren
5. Falls Skill nicht matcht: browser-screenshot.md nochmals signieren (Inhalt wurde geaendert)

---

## Session 50 – P12 Dark/Light Mode Theme Toggle (2026-03-08)

**Fokus:** PRIORITÄT 12 implementieren – Dark/Light Mode Theme Toggle für bessere Dashboard-UX

**Implementierte Features:**

1. **Theme-Toggle Button:**
   - Im Sidebar-Footer, neben Abmelden-Button
   - ☀️/🌙 Icons (dynamisch je nach aktuellem Theme)
   - Label wechselt: "Dark Mode" → "Light Mode"

2. **CSS-Variablen System:**
   - Light Mode (default): white backgrounds (#ffffff), dark text (#1a1a1a), light borders
   - Dark Mode: dark backgrounds (#0f1117), light text (#e2e8f0), dark borders
   - `data-theme` Attribute auf `<html>` für Theme-Switching
   - CSS-Overrides via `html[data-theme="dark"]`

3. **JavaScript Theme-Management:**
   - `initTheme()`: Lädt gespeicherte Preference oder System-Preference
   - `setTheme(theme)`: Setzt Theme + localStorage + aktualisiert Icons
   - `toggleTheme()`: Wechselt zwischen Light/Dark
   - localStorage Key: `fluxbot-theme`

4. **Styling-Anpassungen:**
   - Smooth 0.3s Transitions für alle Farben
   - Modals, Tooltips, Command-Boxes, Blur-Overlays angepasst
   - Light Mode: Modal-Overlays weniger opak (0.3 statt 0.72)
   - Dark Mode: Modal-Overlays voller opak (0.72)
   - Command-Box: Angepasste Farben per Theme

5. **Persistierung & Fallback:**
   - localStorage speichert Preference
   - Auto-detect System-Preference via `window.matchMedia('(prefers-color-scheme: dark)')`
   - Preference bleibt über mehrere Sessions

**Build & Deployment:**
- ✅ `go build -o fluxbot.exe ./cmd/fluxbot` – Clean build
- ✅ Process restart mit neuer Binary
- ✅ Git commit `595f322`

**Dokumentation aktualisiert:**
- CLAUDE.md: Session 50 Summary + Status
- memory-md/06-feature-roadmap.md: P12 als ✅ ERLEDIGT markiert
- memory-md/03-session-log.md: Diesen Eintrag hinzugefügt

**Status:** ✅ ABGESCHLOSSEN
- Theme-Toggle funktioniert
- Persistierung funktioniert
- Alle Styles angepasst
- Smooth Transitions implementiert

---

## Session 49 – Chrome Button Removal (2026-03-08)

**Fokus:** Session 48 Cleanup – Chrome-Button Feature komplett entfernen (unvollstaendige Implementierung)

**Entfernte Code-Aenderungen:**

1. **pkg/system/browser.go** – DELETED
   - File wurde komplett geloescht
   - Enthalte: `OpenBrowser(url string)` mit Windows/macOS/Linux Support

2. **pkg/dashboard/server.go:**
   - Removed: `"github.com/ki-werke/fluxbot/pkg/system"` import
   - Removed: `mux.HandleFunc("/api/system/open-browser", ...)` Route-Registration
   - Removed: `handleOpenBrowser()` API-Handler Funktion

3. **pkg/dashboard/dashboard.html:**
   - Removed: Quick Actions `<div>` mit Chrome-Open Button (Zeile ~1233-1239)
   - Removed: `openBrowser()` JavaScript Funktion (Zeile ~2791-2823)

**Build & Deployment:**
- ✅ `go build -o fluxbot.exe ./cmd/fluxbot` – Clean build, no errors
- ✅ Process restart – FluxBot neu gestartet mit new binary
- ✅ Git commit – `c085a2f` "feat: Remove Chrome button feature"

**Lernpunkt:**
Chrome-Button war premature optimization – Feature wurde incomplete gelassen (Prozess-Management nicht robust).
Focus auf Core-Features ist besser als halb-fertige Extras.

**Status:** ✅ ABGESCHLOSSEN
- Alle Artefakte entfernt
- Commit gepusht
- Codebase clean
- Bereit für naechste Features
