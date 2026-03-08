# CLAUDE.md – FluxBot Projektgedächtnis

> Dieses File ist das persistente Gedächtnis für Claude-Sessions.
> Am Anfang jeder neuen Session: "Lies CLAUDE.md und mach weiter."

---

## Wichtige Dateipfade

| Datei | Pfad | Hinweis |
|-------|------|---------|
| INBOX.md | `C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT\INBOX.md` | **Auto-Cleanup:** Notizen unterhalb des grünen Kommentars werden nach dem Lesen/Verarbeiten automatisch gelöscht |
| Z-FEHLERBILDER | `C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT\Z-FEHLERBILDER\` | **Auto-Cleanup:** Verarbeitete Bilder werden nach dem Lesen automatisch gelöscht (von Claude oder manuell von JJ) |

---

## Detaillierte Dokumentation (memory-md/)

| Datei | Inhalt |
|-------|--------|
| `memory-md/01-features.md` | Alle implementierten Features (P1–P8) + offene Punkte |
| `memory-md/02-architektur.md` | Architektur-Entscheidungen + Secret-Strategie + Keyring |
| `memory-md/03-session-log.md` | Chronologisches Session-Protokoll (Sessions 1–36) |
| `memory-md/04-redesign-spec.md` | Dashboard Redesign Spezifikation (Session 20+) |
| `memory-md/05-bugreports.md` | Bugs & Issues (Session 30+) |
| `memory-md/06-feature-roadmap.md` | Zukunfts-Features & Ideen (Session 30+) |
| `memory-md/07-openclaw-research.md` | OpenClaw Referenz-Study (Session 37) – Security-Patterns, Trust-Based Design, DM-Pairing |

---

## Claude-Workflow – Dokumentation nach Änderungen

**Nach jeder durchgeführten Änderung müssen die entsprechenden memory-md Dateien aktualisiert werden:**

| Änderungstyp | Datei | Aktion |
|-------------|-------|--------|
| **Neues Feature implementiert** | `01-features.md` | Feature in Offene Punkte als `[x] erledigt` markieren oder neue PRIORITÄT hinzufügen |
| **Bug gefixt** | `05-bugreports.md` | Bug-Eintrag mit Fix-Details dokumentieren oder abhaken |
| **Architektur-Änderung** | `02-architektur.md` | Neue Decision / Secret-Key / Integration dokumentieren |
| **Dashboard UI-Änderung** | `04-redesign-spec.md` | Neuer Punkt (15+) oder bestehender Punkt aktualisieren |
| **Neue Feature-Idee erkannt** | `06-feature-roadmap.md` | Feature mit PRIORITÄT, Beschreibung, Use-Case hinzufügen |
| **Session abgeschlossen** | `03-session-log.md` | `## Session XX` mit Highlights, Bugfixes, Implementierungen dokumentieren |
| **INBOX-Items verarbeitet** | `INBOX.md` | Geleert + Verarbeitungs-Kommentar hinterlassen |

**Automatische Erkennung:** Falls neue memory-md Dateien hinzukommen (z.B. `07-xyz.md`), müssen diese automatisch in der Tabelle oben ("Detaillierte Dokumentation") dokumentiert werden.

---

## Projekt-Überblick

**FluxBot** – Multi-Channel AI Agent von KI-WERKE
**Repo:** `github.com/kiwerkepro-org/fluxbot` (private org)
**Go-Modul:** `github.com/ki-werke/fluxbot`
**Sprache:** Go 1.22
**Owner:** JJ (kiwerkepro@gmail.com), Österreich
**Dashboard:** http://localhost:9090 (nur via Tailscale oder lokal)

### Versioning-Konvention
- **Aktueller Release:** `v1.1.4`
- **Schema:** `vMAJOR.MINOR.PATCH`
- **Regel:** Die letzte Ziffer (PATCH) wird bei jedem Release um 1 erhöht, solange JJ nichts anderes angibt.
- Release-Tag wird nach jedem abgeschlossenen Feature-Block auf GitHub gepusht (`git tag -a vX.Y.Z && git push origin vX.Y.Z`)

---

## Architektur

```
cmd/fluxbot/main.go          ← Einstiegspunkt, NewSecretProvider(), Provider-Setup
pkg/
  agent/        ← FluxAgent, Session-Management, Agent-Loop
  channels/     ← Telegram, Discord, Slack, Matrix, WhatsApp
  config/       ← Config-Struct, Validation, Load/Save
  dashboard/    ← HTTP-Server (port 9090), API-Handler, dashboard.html
  security/     ← HMAC Guard, VirusTotal (vt.go), Vault (secrets.go),
                   Keyring-Abstraktionsschicht (keyring.go, keyring_windows.go, keyring_other.go)
  skills/       ← Skill-Loader, HMAC-Signatur, Platzhalter-Substitution
  provider/     ← AI-Provider (OpenRouter, Anthropic, OpenAI, Groq, Ollama, etc.)
  voice/        ← Sprach-Input/Output (Groq)
  imagegen/     ← Bildgenerierung (FAL, OpenAI, Stability, etc.)
  email/        ← SMTP E-Mail-Versand
workspace/
  config.json   ← Konfiguration (KEINE Secrets mehr – alles im Vault)
  .secrets.vault← AES-256-GCM verschlüsselte Secrets
  .vaultkey     ← Vault-Schlüssel (hex, chmod 600)
  skills/       ← Skill-Dateien (.md + .sig)
  memory-md/    ← Agent-Gedächtnis
  logs/         ← fluxbot.log
```

---

## Vault Secret-Keys (Naming Convention – Quick Reference)

```
TELEGRAM_TOKEN, DISCORD_TOKEN, SLACK_BOT_TOKEN, SLACK_APP_TOKEN
SLACK_SIGNING_SECRET, MATRIX_TOKEN, WHATSAPP_API_KEY, WHATSAPP_WEBHOOK_SECRET
PROVIDER_OPENROUTER, PROVIDER_ANTHROPIC, PROVIDER_OPENAI, PROVIDER_GOOGLE
PROVIDER_XAI, PROVIDER_GROQ, PROVIDER_MISTRAL, PROVIDER_TOGETHER
PROVIDER_DEEPSEEK, PROVIDER_PERPLEXITY, PROVIDER_COHERE, ...
VOICE_API_KEY
IMG_OPENROUTER, IMG_FAL, IMG_OPENAI, IMG_STABILITY, IMG_TOGETHER, IMG_REPLICATE
VID_RUNWAY, VID_KLING, VID_LUMA, VID_PIKA, VID_HAILUO, VID_SORA, VID_VEO
SKILL_SECRET, VIRUSTOTAL_API_KEY, DASHBOARD_PASSWORD, DASHBOARD_USERNAME
HMAC_SECRET
OLLAMA_BASE_URL  (optional, Default: http://localhost:11434)
INTEG_{NAME}  z.B. INTEG_CALCOM_API_KEY, INTEG_CALCOM_BASE_URL
GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, GOOGLE_REFRESH_TOKEN
CALCOM_BASE_URL, CALCOM_API_KEY, CALCOM_OWNER_EMAIL
SEARCH_API_KEY          ← Tavily Web-Suche (Session 42)
BROWSER_ENDPOINT        ← Chrome CDP Endpoint, z.B. ws://localhost:9222 (Session 42)
BROWSER_ALLOWED_DOMAINS ← kommagetrennte Whitelist (leer = alle, nicht empfohlen) (Session 42)
```

---

## Dashboard API (Quick Reference)

| Endpoint | Methode | Funktion |
|----------|---------|----------|
| `/api/config` | GET | Konfiguration laden (keine Secrets) |
| `/api/config` | PUT | Konfiguration speichern |
| `/api/secrets` | GET | Alle Vault-Secrets laden |
| `/api/secrets` | POST | Secrets batch-speichern + Hot-Reload |
| `/api/secrets/backend` | GET | Secret-Backend-Info (Keyring/Vault) |
| `/api/status` | GET | Bot-Status |
| `/api/channels` | GET | Aktive Kanäle |
| `/api/vt/status` | GET | VirusTotal Status + Stats |
| `/api/vt/history` | GET | Scan-History |
| `/api/vt/clear` | POST | History zurücksetzen (HMAC) |
| `/api/google/auth-url` | GET | OAuth2 Auth-URL |
| `/api/google/oauth-callback` | GET | OAuth2 Callback → Vault |
| `/api/skills` | GET | Skill-Liste |
| `/api/skills/sign` | POST | Skill neu signieren (HMAC) |
| `/api/auth/check` | GET | Credentials prüfen (200/401) |
| `/api/auth/recover` | GET | Passwort-Wiederherstellung (nur 127.0.0.1) |
| `/api/pairing` | GET | Pairing-Liste (optional ?status=pending/approved/blocked) |
| `/api/pairing` | POST | Pairing-Aktion: approve/block/remove/note (HMAC) |
| `/api/pairing/stats` | GET | Pairing-Statistiken (approved/pending/blocked/total) |

---

## Bekannte Eigenheiten / Bugs

- **HMAC_SECRET Quelle:** Priorität: Vault (`HMAC_SECRET`) → Env-Variable (`FLUXBOT_HMAC_SECRET`) → nicht gesetzt (Startup-Warnung, kein Crash). Ziel: .env so weit wie möglich vermeiden.
- **HMAC_SECRET nicht gesetzt:** Startup-Warnung ist normal wenn nicht konfiguriert – kein Crash
- **Pre-Commit Hook CRLF:** Windows-Zeilenenden in `.git/hooks/pre-commit` → `sed -i 's/\r//' .git/hooks/pre-commit && chmod +x` falls Hook-Fehler
- **Git Push aus VM:** Token in Remote-URL hinterlegt – direkt `git push origin main` funktioniert
- **Tailscale Auth-Key:** In `.env` als `TAILSCALE_AUTH_KEY=tskey-auth-...` – `.env` ist gitignored ✅
- **Cal.com Integration:** Braucht zwei Einträge im Vault: `CALCOM_BASE_URL` und `CALCOM_API_KEY`
- **applySecrets():** CALCOM_* Keys werden explizit in `cfg.Integrations` injiziert (kein INTEG_*-Prefix)
- **skillsLoader.Reload():** Muss nach `SetIntegrations()` aufgerufen werden – sowohl beim Startup als auch in `onReload()`

---

## Lokale Entwicklung / Befehle

> ⚠️ **WICHTIG: FluxBot läuft NATIV auf Windows als fluxbot.exe – KEIN Docker!**
> Kein `docker compose`, kein Container. Deployment = go build + Prozess neustarten.

```powershell
# Neue .exe bauen (nach Code-Änderungen)
cd C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT
go mod tidy
go build -o fluxbot.exe ./cmd/fluxbot

# Laufenden Prozess stoppen + neu starten
Stop-Process -Name fluxbot -Force -ErrorAction SilentlyContinue
Start-Process -FilePath .\fluxbot.exe -WorkingDirectory "C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT" -WindowStyle Hidden

# Logs anzeigen
type workspace\logs\fluxbot.log

# Prozess-Status prüfen
Get-Process -Name fluxbot

# Git Push (Token ist in Remote-URL hinterlegt)
git push origin main

# Skill-Signatur → immer über Dashboard (Skills-Tab → signieren)
# NICHT manuell per Python-Script nötig!
```

---

## Aktueller Stand

- **Letzte Session:** 46 (2026-03-08) – Browser Screenshot Bug ENDGÜLTIG GEFIXT
- **Nächste Aktion:** Debug-Log entfernen, Release v1.2.1 taggen
- **Aktueller Release:** `v1.2.0` → `v1.2.1` vorbereitet
- **Browser Integration:** playwright-go v0.5700.1 ✅ FUNKTIONAL (Viewport 1280x800)

### Session 46 Summary (2026-03-08) – BROWSER SCREENSHOT GEFIXT
- **4 Root Causes** gefunden und behoben:
  1. `splitAndTrim()` strippte YAML `[]` Brackets nicht → Tags matchten nie
  2. Generischer Matcher routete "google.com" zu GDocs → direktes Browser-Skill-Routing implementiert
  3. Docker-Container (`fluxbot_ai`) lief parallel mit gleichem Telegram-Token → entfernt
  4. Full-Page Screenshot 7.6 MB → Viewport 1280x800
- **Status:** ✅ GEFIXT & GETESTET – Screenshot von bild.de erfolgreich

### Session 44 Summary (2026-03-07) – PLAYWRIGHT-MIGRATION
- 🔄 **Playwright-Version Problem gelöst:**
  - Problem: go.mod hatte `v1.45.0` (ungültig) → `v1.44.0` (auch ungültig)
  - Solution: go.mod → `v0.5700.1` (korrekte installierte Version)
  - **Root Cause:** `go.sum` hatte 0 Playwright-Einträge = Build nie erfolgreich = **alte Binary lief immer noch!**

- ✅ **8 API-Inkompatibilitäten in pkg/browser/browser.go behoben:**
  - Timeouts: int64 → float64 Konvertierung
  - WaitUntilLoadState Constructor existiert nicht → Enum-Konstanten verwenden
  - LoadState Enums sind bereits Pointer-Typen (keine `&` nötig)
  - ScreenshotType Enum statt String
  - WaitForSelector gibt 2 Werte zurück (Element, error)
  - **Detailfixes:** siehe `memory-md/05-bugreports.md` Bug #7 & `memory-md/03-session-log.md` Session 44

- ✅ **Dokumentation aktualisiert:**
  - `05-bugreports.md` – Bug #7 mit Session 44 Details + 8 API-Fixes
  - `03-session-log.md` – Sessions 41, 42, 43, 44 vollständig dokumentiert
  - `CLAUDE.md` (diese Datei) – Aktualisiert

- ⏳ **TODO vor Release v1.2.1:**
  1. ✅ browser.go API-Fixes abgeschlossen
  2. ⏳ `go build -o fluxbot.exe ./cmd/fluxbot` (sollte jetzt erfolgreich sein)
  3. ⏳ Prozess neustarten + Screenshot-Test
  4. ⏳ Git Commit + Tag v1.2.1

### Session 43 Summary (2026-03-07)
- 🔴 **Browser Screenshots Bug Debugging:**
  - Problem: `"Mache einen Screenshot von bild.de"` → "Bildgenerierung ist aktuell nicht aktiviert"
  - Root Cause (Session 43): `isBrowserContext()` nicht implementiert → Build schlägt fehl
  - **Wahre Root Cause (Session 44):** Falsche Playwright-Version in go.mod (chromedp wurde zu playwright gemigrt)
  - Status: Code-Fixes in Session 43, aber Build war nie erfolgreich

### Session 42 Summary (2026-03-06)
- ✅ **Browser Skills (Option D) implementiert:**
  - Phase 1: Web-Suche via Tavily API (`pkg/search/search.go`)
  - Phase 2: Browser-Steuerung via **chromedp** (`pkg/browser/browser.go`)
    - **Anmerkung:** chromedp wurde später in Session 44 durch playwright-go ersetzt
  - 4 neue Skills: web-search, browser-read, browser-screenshot, browser-fill
  - Vault-Keys: `SEARCH_API_KEY`, `BROWSER_ENDPOINT`, `BROWSER_ALLOWED_DOMAINS`

### Session 41 Summary (2026-03-06)
- ✅ **AutoStart-Bug behoben:** AUTOSTART-EINRICHTEN.ps1 mit Task Scheduler (statt sc.exe Service)
  - FluxBot läuft unsichtbar im Hintergrund, Auto-Restart bei Crash
  - Startet automatisch bei Windows-Login
  - Desktop-Verknüpfung "FluxBot Dashboard" öffnet Browser
- ✅ **QUICK-START.txt** auf v1.2.0 aktualisiert

### Session 40 Summary (2026-03-05)
- ✅ **Option A (P9 Live-Testing):** PASSED – Pairing-Tab funktional
- ✅ **Option C (Self-Extend Feature):** COMPLETED
  - Stufe 1: Skill-Writer, Stufe 2: Code-Reader API, Stufe 3: Code-Extender
  - Security: Whitelist-basiert, Directory-Traversal Schutz

### Status nach Session 44
- **AutoStart:** ✅ Task Scheduler (AtLogon, Hidden, Auto-Restart)
- **Dashboard:** http://localhost:9090 erreichbar
- **P9 DM-Pairing Mode:** ✅ LIVE & FUNKTIONAL
- **Self-Extend Feature:** ✅ LIVE – 3 Stufen implementiert
- **Browser Skills:** ✅ FUNKTIONAL – Screenshot, Read, Fill (Playwright/chromium, Viewport 1280x800)
- **Docker:** ❌ ENTFERNT – FluxBot läuft nur noch nativ auf Windows
- **Details:** `memory-md/03-session-log.md` (Sessions 1–46)
