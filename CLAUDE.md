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
| `memory-md/01-features.md` | Alle implementierten Features (P1–P10) + offene Punkte |
| `memory-md/02-architektur.md` | Architektur-Entscheidungen + Secret-Strategie + Keyring |
| `memory-md/03-session-log.md` | Chronologisches Session-Protokoll (Sessions 1–48) |
| `memory-md/04-redesign-spec.md` | Dashboard Redesign Spezifikation (Session 20+) |
| `memory-md/05-bugreports.md` | Bugs & Issues (Session 30+) |
| `memory-md/06-feature-roadmap.md` | Zukunfts-Features & Ideen aus INBOX.md → priorisiert |
| `memory-md/07-openclaw-research.md` | OpenClaw Security-Learnings (Session 37) – Best-Practices für FluxBot |

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
- **Aktueller Release:** `v1.2.1` (2026-03-08)
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

- **Letzte Session:** 51 (2026-03-08) – P0 Installation & Update System ✅ IMPLEMENTIERT
- **Nächste Aktion:** Features aus Roadmap (P10 Granulare DM-Policy, P11 Dangerous-Tools, P13 OCR)
- **Aktueller Release:** `v1.2.1` ✅ (nächster Release nach P0: v1.2.2)
- **Browser Integration:** playwright-go v0.5700.1 ✅ VOLL FUNKTIONAL
- **Dark Mode:** ✅ LIVE – Theme-Toggle im Sidebar-Footer

### Session 50 Summary (2026-03-08) – P12 DARK/LIGHT MODE THEME TOGGLE
- ✅ **Feature:** Dark/Light Mode Theme Toggle implementiert (P12)
- ✅ **CSS-Variablen:** Light Mode defaults, Dark Mode via `html[data-theme="dark"]`
  - Light: white backgrounds (#ffffff), dark text (#1a1a1a), light borders
  - Dark: dark backgrounds (#0f1117), light text (#e2e8f0), dark borders
- ✅ **Theme-Toggle Button:** Im Sidebar-Footer (neben Abmelden)
  - ☀️/🌙 Icons (dynamisch je nach aktuellem Theme)
  - Label wechselt: "Dark Mode" ↔ "Light Mode"
- ✅ **Persistierung:** localStorage (`fluxbot-theme`)
  - Auto-detect System-Preference wenn keine Preference gespeichert
  - Preference bleibt über Sessions erhalten
- ✅ **Styling:** Alle UI-Elemente angepasst
  - Modals, Tooltips, Blur-Overlays, Command-Boxen
  - Smooth 0.3s Transitions für alle Farben
  - Modal-Overlays opacity per Theme angepasst
- ✅ **Build:** `go build -o fluxbot.exe ./cmd/fluxbot` ✅ SUCCESS
- ✅ **Commit:** `595f322` – "feat: Add Dark/Light Mode Theme Toggle (P12)"

### Session 49 Summary (2026-03-08) – CHROME BUTTON REMOVAL
- ⚠️ **Rollback:** Chrome-Button Feature aus Session 48 komplett entfernt
  - Grund: Incomplete implementation, process management issues, not meeting requirements
  - User feedback: "Programmiere den Google Chrome öffnen Button wieder komplett raus"
- ✅ **Removed Files/Code:**
  - Deleted: `pkg/system/browser.go` (Chrome launcher)
  - Removed: `handleOpenBrowser()` from `pkg/dashboard/server.go`
  - Removed: System package import from server.go
  - Removed: Quick Actions button from dashboard.html UI
  - Removed: `openBrowser()` JavaScript function from dashboard.html
- ✅ **Build:** `go build -o fluxbot.exe ./cmd/fluxbot` ✅ SUCCESS (clean build)
- ✅ **Commit:** `c085a2f` – "feat: Remove Chrome button feature"
- 📊 **Analysis:** Chrome button was premature optimization – focus on core features instead

### Session 47 Summary (2026-03-08) – BROWSER ACTIONS + STEALTH + LUCIDE
- ✅ **Dynamische Browser-Aktionen:** `RunActions()` mit 7 Aktionstypen + `browser-actions.md` Skill
- ✅ **Anti-Bot-Detection:** Stealth-Init, `--disable-blink-features=AutomationControlled`, realistischer UA
- ✅ **Cookie-Banner Auto-Dismiss:** 16 Selektoren, max 2s, nach jedem Goto
- ✅ **OpenVisible():** Sichtbares Chromium-Fenster via "rufe X auf" / "öffne X"
- ✅ **Browser-Routing:** Default → Screenshot, "öffne" → sichtbar, "lies" → Text, Domain = Browser
- ✅ **Lucide CDN:** Script fehlte → eingebunden, Nav-Icons wieder sichtbar
- ✅ **Unsignierte Skills:** `isSignatureInvalid()` erkennt fehlende `.sig` → Warnung im Dashboard

### Session 48 Summary (2026-03-08) – RELEASE V1.2.1 + README REDESIGN
- ✅ **README komplett überarbeitet:** Neue Header-Layout (inline Bilder, keine Tabelle), Partner-Logos 140px
- ✅ **PayPal Spende:** Vor Features-Section, mit direktem Link, mit Badge & CTA
- ✅ **Alle Links:** `target="_blank"` für neue Tabs
- ✅ **KI-WERKE Links:** Entfernt → nur noch "powered by KI-WERKE" als Text
- ✅ **Neue Features dokumentiert:** Browser (Playwright), Google Workspace, DM-Pairing, Self-Extend
- ✅ **Release v1.2.1:** Committed, tagged, gepusht, GitHub Release mit Release-Notes ✅
- ✅ **Git History clean:** O1000-OpenClaw komplett entfernt + purged
- ✅ **Validierung:** OpenVisible funktioniert ✅, Lucide Icons ✅
- 🔄 **P4 System-Testing:** Auf "später" verschoben (Cal.com, VT Live-Test, Google OAuth)

### Session 51 Summary (2026-03-08) – P0 INSTALLATION & UPDATE SYSTEM
- ✅ **`install.ps1`** – Windows Installer (Nativ + Docker Menü)
  - Nativ: GitHub Release Binary, Playwright-Browser, Task Scheduler Autostart
- ✅ **`install.sh`** – Linux/macOS Installer (Nativ + Docker Menü)
  - Nativ: GitHub Release Binary, Systemd (Linux) / LaunchAgent (macOS)
- ✅ **`pkg/system/updater.go`** – Auto-Updater (Go)
  - GitHub Releases API, Background-Check alle 6h, Binary-Download + Install
- ✅ **Dashboard API:** `/api/system/version`, `/api/system/check-update`, `/api/system/install-update`
- ✅ **Update-Panel** im Status-Tab – Versions-Badge + 1-Klick-Update
- ✅ **`--install-playwright` Flag** – Browser-Installation via Binary-Flag
- ✅ **Makefile:** macOS-Assets normalisiert (`fluxbot-darwin-*` statt `fluxbot-macos-*`)
- ✅ **Version:** Hardcoded-Fallback auf `v1.2.1` korrigiert (war `v1.1.9`)
- ✅ **Build:** `go build -o fluxbot.exe ./cmd/fluxbot` ✅ SUCCESS

### Status nach Session 50
- **AutoStart:** ✅ Task Scheduler (AtLogon, Hidden, Auto-Restart)
- **Dashboard:** http://localhost:9090 erreichbar, Lucide Icons ✅, Dark Mode ✅
- **P9 DM-Pairing Mode:** ✅ LIVE & FUNKTIONAL
- **P12 Dark/Light Mode:** ✅ LIVE – Theme-Toggle im Sidebar, localStorage Persistierung
- **Self-Extend Feature:** ✅ LIVE – 3 Stufen implementiert
- **Browser Skills:** ✅ VOLL FUNKTIONAL – Screenshot, Read, Fill, Actions, OpenVisible (Playwright)
- **OpenVisible:** ✅ FUNKTIONAL (via Heimnetzwerk, öffnet auf Desktop wenn auf Handy Telegram-Befehl kam)
- **Docker:** ❌ ENTFERNT – FluxBot läuft nur noch nativ auf Windows
- **Release:** `v1.2.1` ✅ PUBLISHED
- **Details:** `memory-md/03-session-log.md` (Sessions 1–50)
