# CLAUDE.md – FluxBot Projektgedächtnis

> Dieses File ist das persistente Gedächtnis für Claude-Sessions.
> Am Anfang jeder neuen Session: "Lies CLAUDE.md und mach weiter."

---

## Wichtige Dateipfade

| Datei | Pfad | Hinweis |
|-------|------|---------|
| INBOX.md | `C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT\INBOX.md` | Nur lesen wenn JJ explizit darum bittet |

---

## Detaillierte Dokumentation (memory-md/)

| Datei | Inhalt |
|-------|--------|
| `memory-md/01-features.md` | Alle implementierten Features (P1–P8) + offene Punkte |
| `memory-md/02-architektur.md` | Architektur-Entscheidungen + Secret-Strategie + Keyring |
| `memory-md/03-session-log.md` | Chronologisches Session-Protokoll (Sessions 1–21) |
| `memory-md/04-redesign-spec.md` | Dashboard Redesign Spezifikation (Session 20, noch offen) |

---

## Projekt-Überblick

**FluxBot** – Multi-Channel AI Agent von KI-WERKE
**Repo:** `github.com/kiwerkepro-org/fluxbot` (private org)
**Go-Modul:** `github.com/ki-werke/fluxbot`
**Sprache:** Go 1.22
**Owner:** JJ (kiwerkepro@gmail.com), Österreich
**Dashboard:** http://localhost:9090 (nur via Tailscale oder lokal)

### Versioning-Konvention
- **Aktueller Release:** `v1.1.1`
- **Schema:** `vMAJOR.MINOR.PATCH`
- **Regel:** Die letzte Ziffer (PATCH) wird bei jedem Release um 1 erhöht, solange JJ nichts anderes angibt.
- **Nächster Release:** `v1.1.2` (automatisch, außer JJ gibt ein anderes Schema vor)
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
SKILL_SECRET, VIRUSTOTAL_API_KEY, DASHBOARD_PASSWORD
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

## Aktueller Stand (Session 21 – 2026-02-23)

**Letzte abgeschlossene Session:** 20 (Dashboard Redesign geplant, noch nicht implementiert)
**Aktuelle Session:** 21

**Offen / Nächste Schritte:**
1. `pkg/dashboard/dashboard.html` komplett neu schreiben → siehe `memory-md/04-redesign-spec.md`
2. Git commit + push nach Dashboard-Redesign
3. Docker-Rebuild + Test

**Erledigt in Session 21:**
- `memory-md/` Ordner erstellt ✅
- `.gitignore` um `memory-md/` ergänzt ✅
- CLAUDE.md in 4 thematische Dateien aufgeteilt ✅
