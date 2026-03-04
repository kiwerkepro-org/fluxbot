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
| `memory-md/03-session-log.md` | Chronologisches Session-Protokoll (Sessions 1–30) |
| `memory-md/04-redesign-spec.md` | Dashboard Redesign Spezifikation (Session 20+) |
| `memory-md/05-bugreports.md` | Bugs & Issues (Session 30+) |
| `memory-md/06-feature-roadmap.md` | Zukunfts-Features & Ideen (Session 30+) |

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

```powershell
# Docker neu bauen und starten
docker compose down; docker compose up -d --build

# Nur neu starten (kein Rebuild)
docker compose restart fluxbot

# Logs anzeigen
docker logs fluxbot_ai --tail 80

# Git Push (Token ist in Remote-URL hinterlegt)
git push origin main

# Skill-Signatur neu generieren (Python, nach Skill-Änderung)
python3 -c "
import hmac, hashlib
secret = 'SKILL_SECRET_AUS_CONFIG_JSON'
path = 'workspace/skills/SKILL_NAME.md'
with open(path, 'rb') as f: data = f.read()
sig = hmac.new(secret.encode(), data, hashlib.sha256).hexdigest()
with open(path + '.sig', 'w') as f: f.write(sig)
"
```

---

## Aktueller Stand

- **Letzte Session:** 33 (2026-03-04)
- **Nächste Session:** 34
- **Letzter Release:** `v1.1.7` ✅ (Session 33)
- **Details:** `memory-md/03-session-log.md`
