# CLAUDE.md – FluxBot Projektgedächtnis

> Dieses File ist das persistente Gedächtnis für Claude-Sessions.
> Am Anfang jeder neuen Session: "Lies CLAUDE.md und mach weiter."

---

## Projekt-Überblick

**FluxBot** – Multi-Channel AI Agent von KI-WERKE
**Repo:** `github.com/kiwerkepro-org/fluxbot` (private org)
**Go-Modul:** `github.com/ki-werke/fluxbot`
**Sprache:** Go 1.22
**Owner:** JJ (kiwerkepro@gmail.com), Österreich
**Dashboard:** http://localhost:9090 (nur via Tailscale oder lokal)

---

## Architektur

```
cmd/fluxbot/main.go          ← Einstiegspunkt, Vault-Init, Provider-Setup
pkg/
  agent/        ← FluxAgent, Session-Management, Agent-Loop
  channels/     ← Telegram, Discord, Slack, Matrix, WhatsApp
  config/       ← Config-Struct, Validation, Load/Save
  dashboard/    ← HTTP-Server (port 9090), API-Handler, dashboard.html
  security/     ← HMAC Guard, VirusTotal (vt.go), Vault (secrets.go)
  skills/       ← Skill-Loader, HMAC-Signatur, Platzhalter-Substitution
  provider/     ← AI-Provider (OpenRouter, Anthropic, OpenAI, Groq, etc.)
  voice/        ← Sprach-Input/Output (Groq)
  imagegen/     ← Bildgenerierung (FAL, OpenAI, Stability, etc.)
  email/        ← SMTP E-Mail-Versand
workspace/
  config.json   ← Konfiguration (KEINE Secrets mehr – alles im Vault)
  .secrets.vault← AES-256-GCM verschlüsselte Secrets
  .vaultkey     ← Vault-Schlüssel (hex, chmod 600)
  skills/       ← Skill-Dateien (.md + .sig)
  memory/       ← Agent-Gedächtnis
  logs/         ← fluxbot.log
```

---

## Was ist implementiert ✅

### Security
| Feature | Status | Datei |
|---------|--------|-------|
| AES-256-GCM Vault | ✅ fertig | `pkg/security/secrets.go` |
| HMAC-SHA256 Skill-Signierung | ✅ fertig | `pkg/skills/signer.go` |
| VirusTotal File-Scan | ✅ teilweise | `pkg/security/vt.go` |
| Tailscale VPN-Sidecar | ✅ fertig | `docker-compose.yml` |
| Pre-Commit Hook (Secret-Check) | ✅ fertig | `.git/hooks/pre-commit` |
| .gitignore Hardening | ✅ fertig | `.gitignore` |
| Dashboard Port 127.0.0.1 | ✅ fertig | `docker-compose.yml` |

### Vault (AES-256-GCM)
- Schlüssel-Priorität: `FLUXBOT_VAULT_KEY` (Env) → `.vaultkey` Datei → auto-generiert
- Einmalige Migration: config.json → Vault beim ersten Start mit neuem Code
- Hot-Reload: POST `/api/secrets` → `onReload()` → `applySecrets()` → sofort aktiv
- Dashboard-Passwort Hot-Reload via `dash.UpdatePassword()`
- `cfg.Validate()` läuft NACH `applySecrets()` (wichtig! Bug war hier)

### Vault Secret-Keys (Naming Convention)
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
INTEG_{NAME}  z.B. INTEG_CALCOM_API_KEY, INTEG_CALCOM_BASE_URL
```

### Dashboard API
| Endpoint | Methode | Funktion |
|----------|---------|----------|
| `/api/config` | GET | Konfiguration laden (keine Secrets) |
| `/api/config` | PUT | Konfiguration speichern |
| `/api/secrets` | GET | Alle Vault-Secrets laden |
| `/api/secrets` | POST | Secrets batch-speichern + Hot-Reload |
| `/api/status` | GET | Bot-Status |
| `/api/channels` | GET | Aktive Kanäle |

### Dashboard JS (dashboard.html)
- `secretsData` – globale Variable für Vault-Werte
- `loadConfig()` – lädt `/api/config` + `/api/secrets` parallel (Promise.all)
- `saveConfig()` – trennt: nicht-sensitiv → `/api/config`, Secrets → `/api/secrets`
- `renderIntegrations()` – Werte aus `secretsData['INTEG_*']`
- `collectIntegrations()` – gibt `{configList, secretsMap}` zurück
- Info-Button ⓘ bei Platzhalter-Name erklärt das Platzhalter-Konzept

### Kanäle
- Telegram ✅, Discord ✅, Slack (konfigurierbar), Matrix (konfigurierbar), WhatsApp (konfigurierbar)

### Skills
- Skill-Dateien in `workspace/skills/*.md` + `.sig` (HMAC-SHA256 Signatur)
- Platzhalter `{{NAME}}` werden aus Integrationen substituiert
- **Cal.com Skill:** `{{CALCOM_BASE_URL}}` + `{{CALCOM_API_KEY}}` (cal.com UND cal.eu)
- Im Dashboard unter Integrationen: Name muss exakt dem `{{PLATZHALTER}}` entsprechen

### Infrastruktur
- Docker Compose mit Tailscale-Sidecar
- GitHub Actions CI/CD mit Bitwarden Secrets Manager (BWS)
- BWS liefert `FLUXBOT_HMAC_SECRET` und `VIRUSTOTAL_API_KEY` im Build
- Git Push funktioniert direkt aus der VM (Token in Remote-URL hinterlegt)
- Pre-Commit Hook: CRLF-Problem wurde gefixt (`sed -i 's/\r//'`)

---

## Offene Punkte / Agenda 📋

### PRIORITÄT 1 – VirusTotal auf alle Kanäle erweitern
```
[ ] Gemeinsame Scan-Hilfsfunktion in pkg/security/ (nicht kanal-spezifisch)
[ ] Telegram:   Dateien scannen (PDF, Bild, Dokument, Video, Audio)
[ ] Telegram:   Links in Textnachrichten auf Malware prüfen
[ ] Discord:    Datei-Uploads scannen
[ ] Discord:    Links prüfen
[ ] Slack:      Datei-Uploads scannen
[ ] Matrix:     Datei-Uploads scannen
[ ] WhatsApp:   Medien/Dokumente scannen
[ ] Alle:       Einheitliche Benutzer-Fehlermeldung bei Fund
```
**Aktueller Stand:** VT nur in `pkg/channels/telegram.go` Zeile 77, nur Voice/Audio-Dateien.

### PRIORITÄT 2 – HMAC Compendium (ausstehende Items)
```
[ ] Prüfen welche HMAC-Items aus dem Compendium noch fehlen
[ ] HMAC für Dashboard-API-Requests (nicht nur Skill-Signierung)
```

### PRIORITÄT 3 – VT Dashboard Tab (Steps 7+8)
```
[ ] VirusTotal-Tab im Dashboard (Scan-History, Status, Statistiken)
```

### PRIORITÄT 4 – Tests
```
[ ] Alle Blöcke aus der Hardcore Test Suite durchführen
[ ] Vault-Persistenz nach Docker-Neustart bestätigen
[ ] Hot-Reload verifizieren
[ ] Cal.com Integration mit korrekten Platzhaltern testen
```

---

## Wichtige Entscheidungen (Why)

| Entscheidung | Begründung |
|-------------|------------|
| VaultProvider statt nativer Keyring | Cross-platform: Windows/Mac/Linux/VPS/Docker – nativer Keyring funktioniert nicht headless |
| AES-256-GCM statt bcrypt für Vault | Vault-Daten müssen entschlüsselbar sein (kein One-Way-Hash) |
| Tailscale statt 2FA im Dashboard | Einfacher, sicherer, zero-trust – "zweiter Faktor" ist VPN-Zugang |
| cfg.Validate() nach applySecrets() | Secrets kommen aus Vault, nicht aus config.json – Validate vorher = Fehler |
| Skill-Platzhalter variabel halten | User bestimmt den Namen, Skill-Datei wird nicht geändert |
| workspace/ gitignored | Enthält persönliche Daten, API-Keys (alt), Gedächtnis, Gesprächsverläufe |

---

## Bekannte Eigenheiten / Bugs

- **HMAC_SECRET nicht gesetzt:** Startup-Warnung ist normal wenn nicht konfiguriert – kein Crash
- **Pre-Commit Hook CRLF:** Windows-Zeilenenden in `.git/hooks/pre-commit` → `sed -i 's/\r//' .git/hooks/pre-commit && chmod +x` falls Hook-Fehler
- **Git Push aus VM:** Token in Remote-URL hinterlegt – direkt `git push origin main` funktioniert
- **Tailscale Auth-Key:** In `.env` als `TAILSCALE_AUTH_KEY=tskey-auth-...` – `.env` ist gitignored ✅
- **Cal.com Integration:** Braucht zwei Einträge: `CALCOM_BASE_URL` und `CALCOM_API_KEY`

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

## Letzte Session (Stand: 2026-02-22)

**Erledigt:**
- AES-256-GCM Vault vollständig implementiert + migriert
- Dashboard lädt/speichert Secrets getrennt von Config
- Bug gefixt: cfg.Validate() lief vor applySecrets() → zweiter Start schlug fehl
- Tailscale VPN-Sidecar integriert, Port auf 127.0.0.1 gebunden
- .env Datei erstellt (Tailscale Auth-Key eingetragen)
- Cal.com Skill auf flexible Platzhalter umgestellt (cal.com + cal.eu)
- Info-Button ⓘ im Dashboard für Platzhalter-Erklärung
- CLAUDE.md erstellt (dieses File)

**Nächster Schritt:** VirusTotal auf alle Kanäle und Dateitypen erweitern
