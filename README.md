<div align="center">

<table border="0" cellspacing="0" cellpadding="16"><tr>
<td align="center" valign="middle" width="140">
  <img src="assets/virustotal-logo.png" width="72" alt="VirusTotal"/><br/>
  <sub><b>Malware-Schutz</b></sub>
</td>
<td align="center" valign="middle">
  <img src="assets/fluxion-logo.png" alt="FluxBot Logo" width="210"/><br/>
  <h1>FluxBot</h1>
  <strong>powered by KI-WERKE</strong>
</td>
<td align="center" valign="middle" width="140">
  <img src="assets/bitwarden-logo.png" width="72" alt="Bitwarden"/><br/>
  <sub><b>Secrets-Verwaltung</b></sub>
</td>
</tr></table>

![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/Lizenz-MIT-green)
![Self-Hosted](https://img.shields.io/badge/Self--Hosted-100%25-blueviolet)
![Platforms](https://img.shields.io/badge/Plattformen-Windows%20%7C%20Linux%20%7C%20macOS%20%7C%20Docker-lightgrey)

**Dein persönlicher KI-Assistent für Telegram, WhatsApp, Slack und mehr – selbst gehostet, sicher, kostenlos erweiterbar.**

</div>

---

FluxBot läuft auf deinem eigenen Server oder Computer – nicht in der Cloud. Du behältst die volle Kontrolle über deine Daten und kannst jeden KI-Anbieter frei wählen.

## 🧠 Was kann FluxBot?

| Funktion | Beschreibung |
|----------|-------------|
| 💬 **Schreiben & Fragen** | Beantwortet Fragen, schreibt Texte, übersetzt, erklärt – alles per Chat |
| 🎙️ **Sprachnachrichten** | Sprachnachricht schicken → FluxBot transkribiert und antwortet |
| 🎨 **Bilder erstellen** | Einfach schreiben „Erstelle ein Bild von …" – fertig |
| 🔍 **Websuche** | Aktuelle Infos, News, Recherche direkt im Chat |
| 🧠 **Langzeit-Gedächtnis** | FluxBot merkt sich wichtige Fakten dauerhaft |
| 🎭 **Eigene Persönlichkeit** | Über `SOUL.md` kannst du FluxBot eine eigene Identität geben |
| 🛠️ **Skills** | Füge eigene Fähigkeiten als einfache Textdateien hinzu |
| 🖥️ **Web-Dashboard** | Einstellungen bequem im Browser verwalten |
| 🔒 **Sicherheits-Scan** | Alle Dateien & Links werden automatisch via VirusTotal geprüft |

---

## 📱 Unterstützte Messenger

| Messenger | Status |
|-----------|--------|
| Telegram  | ✅ Fertig & produktiv |
| WhatsApp  | ✅ Implementiert |
| Slack     | ✅ Implementiert |
| Matrix    | ✅ Implementiert |
| Discord   | ✅ Implementiert |

---

## 🤖 Unterstützte KI-Anbieter

FluxBot funktioniert mit allen großen KI-Anbietern. Du wählst deinen Anbieter bequem im Dashboard – ohne Konfigurationsdateien zu bearbeiten.

| Anbieter | Typ |
|----------|-----|
| OpenRouter | ☁️ Cloud (aggregiert viele Modelle) |
| Anthropic (Claude) | ☁️ Cloud |
| OpenAI (GPT) | ☁️ Cloud |
| Google (Gemini) | ☁️ Cloud |
| xAI (Grok) | ☁️ Cloud |
| Groq | ☁️ Cloud (sehr schnell) |
| Mistral, Together, DeepSeek, Perplexity, Cohere | ☁️ Cloud |
| **Ollama** | 🏠 **Lokal – kostenlos, privat, ohne API-Key** |
| Benutzerdefinierter OpenAI-kompatibler Endpunkt | 🔧 Flexibel |

> **Tipp:** Mit **Ollama** kannst du FluxBot komplett lokal betreiben – kein API-Key, keine Cloud-Kosten, volle Datenschutzkontrolle. Empfohlen für einfache Use-Cases oder Umgebungen mit hohem Nachrichtenvolumen.

---

## ⚡ Installation

### 🖱️ Weg 1 – Direktinstallation *(empfohlen für Desktop-Nutzer)*

Kein Docker, kein Terminal, kein JSON. Einfach herunterladen und starten.

Lade die passende Datei von der [Releases-Seite](https://github.com/kiwerkepro-org/fluxbot/releases) herunter:

| Betriebssystem | Datei |
|---------------|-------|
| Windows | `fluxbot-windows-amd64.exe` |
| Linux | `fluxbot-linux-amd64` |
| macOS (Intel) | `fluxbot-darwin-amd64` |
| macOS (Apple Silicon) | `fluxbot-darwin-arm64` |

Datei starten → Der Einrichtungsassistent öffnet sich automatisch im Browser. KI-Anbieter, API-Key und Messenger eintragen – fertig. Kein manuelles Bearbeiten von Konfigurationsdateien.

---

### 🐳 Weg 2 – Docker *(empfohlen für Server und fortgeschrittene Nutzer)*

> Voraussetzung: [Docker Desktop](https://www.docker.com/products/docker-desktop/) muss installiert und gestartet sein.

**Windows (PowerShell):**
```powershell
# Installer-Skript von der Release-Seite herunterladen
```

**Linux / macOS (Terminal):**
```bash
# Installer-Skript von der Release-Seite herunterladen
```

Die Installer-Skripte erledigen alles automatisch: Docker-Check → Verzeichnis anlegen → Image ziehen → Container starten → Dashboard öffnen (`http://localhost:9090`).

Lade sie von der [Releases-Seite](https://github.com/kiwerkepro-org/fluxbot/releases) herunter.

Deine Konfiguration und alle Daten werden dauerhaft in `~/FluxBot/fluxbot-data` gespeichert und bleiben auch nach Updates erhalten.

---

## ⬆️ Updaten

**Direktinstallation:** Neue Version von der Releases-Seite herunterladen und die alte ersetzen. Die `config.json` im selben Verzeichnis bleibt erhalten.

**Docker:**
```bash
docker compose -f ~/FluxBot/docker-compose.yml pull
docker compose -f ~/FluxBot/docker-compose.yml up -d
```

---

## 💬 Chat-Befehle

Du brauchst keine speziellen Befehle – schreib einfach normal. Diese Sätze haben aber besondere Funktionen:

**Gedächtnis:**
```
Merke dir, ich wohne in Wien        → FluxBot speichert das dauerhaft
Merk dir: ich bin Grafikdesigner    → FluxBot speichert das dauerhaft
Vergiss das mit Wien                → Löscht diesen einen Fakt
Vergiss alles                       → Löscht das komplette Gedächtnis
```

**Gespräch zurücksetzen:**
```
Neues Gespräch    → Löscht den bisherigen Chatverlauf (Fakten bleiben erhalten)
Reset             → Gleiche Funktion
```

**Bilder erstellen:**
```
Erstelle ein Bild von einem Sonnenuntergang am Meer
Male mir ein futuristisches Wien
Generiere ein Foto von einer Katze im Weltall
```

---

## 🖥️ Web-Dashboard

Das Dashboard ist unter `http://localhost:9090` erreichbar. Dort kannst du:

- KI-Anbieter und API-Keys wechseln
- Messenger ein- und ausschalten
- Nachrichten und Logs einsehen
- Skills verwalten
- Secrets sicher im verschlüsselten Vault speichern

---

## 🎭 Persönlichkeit anpassen (SOUL.md)

<div align="right">
<img src="assets/fluxion-character.png" alt="Fluxion Character" width="130">
</div>

Du kannst FluxBot eine eigene Persönlichkeit geben – einfach die Datei `workspace/SOUL.md` (Direktinstallation) bzw. `~/FluxBot/fluxbot-data/SOUL.md` (Docker) bearbeiten:

```
Du bist FluxBot, der KI-Assistent von Mein Unternehmen GmbH.
Du antwortest immer freundlich und auf Deutsch.
Du sprichst Kunden mit "Sie" an.
Du gibst niemals Auskunft über Konkurrenz-Produkte.
```

Nach dem Speichern FluxBot neu starten – fertig.

---

## 🛠️ Eigene Skills hinzufügen

Ein Skill ist eine `.md`-Textdatei im Ordner `skills/`. Wenn ein Nutzer ein Keyword schreibt, lädt FluxBot automatisch den passenden Skill.

**Beispiel:** `skills/wordpress.md`
```markdown
# WordPress-Experte

## Keywords
wordpress, plugin, theme, wp

## Regeln
- Beziehe alle Antworten auf WordPress
- Frage zuerst: Plugin oder Theme?
- Empfehle immer quelloffene Lösungen
```

FluxBot erkennt das Keyword `wordpress` und verhält sich automatisch als WordPress-Experte.

---

## 🗂️ Dateistruktur

```
fluxbot/
│
├── 📁 assets/                  ← Logos und Bilder
├── 📁 workspace/               ← Deine persönlichen Dateien (gitignored!)
│   ├── config.json             ← Deine Einstellungen (vom Wizard erstellt)
│   ├── .secrets.vault          ← AES-256-GCM verschlüsselter Secret-Speicher
│   ├── SOUL.md                 ← Persönlichkeit von FluxBot (optional)
│   ├── 📁 sessions/            ← Gesprächsverläufe (automatisch)
│   ├── 📁 logs/                ← Protokoll-Dateien (automatisch)
│   └── 📁 skills/              ← Deine eigenen Skills (.md-Dateien)
│
├── 📁 cmd/fluxbot/             ← Programm-Einstiegspunkt
├── 📁 pkg/                     ← Interne Pakete
│
├── Dockerfile                  ← Docker-Image-Definition
├── docker-compose.yml          ← Entwicklungs-Compose
├── docker-compose.prod.yml     ← Produktions-Compose
├── install.ps1                 ← Windows Installer-Skript
├── install.sh                  ← Linux/macOS Installer-Skript
└── Makefile                    ← Hilfsbefehle zum Bauen
```

---

## 🔒 Sicherheit

| Schutzmaßnahme | Beschreibung |
|---------------|-------------|
| **Zugangskontrolle** | Nur eingetragene Nutzer (`allowFrom`) können mit FluxBot schreiben |
| **VirusTotal-Scan** | Alle empfangenen Dateien und Links werden automatisch auf Malware geprüft |
| **AES-256-GCM Vault** | API-Keys und Secrets werden verschlüsselt gespeichert – nie im Klartext |
| **HMAC-Signierung** | Dashboard-API-Requests werden kryptografisch signiert |
| **Injection-Schutz** | Erkennt und blockiert Manipulationsversuche (40+ Muster, DE & EN) |
| **Rate-Limiting** | Maximal 30 Nachrichten pro Minute pro Nutzer |
| **Tailscale VPN** | Dashboard nur über verschlüsseltes VPN erreichbar (optional) |
| **Audit-Log** | Alle Aktivitäten protokolliert – DSGVO-konform, 90 Tage Aufbewahrung |

---

## 🆘 Häufige Probleme

**FluxBot antwortet nicht?**
→ Prüfe, ob deine Telegram-User-ID korrekt in `allowFrom` steht (`@userinfobot` hilft dabei).

**Docker-Fehler beim Start?**
→ Ist Docker Desktop geöffnet? Das Whale-Icon in der Taskleiste muss aktiv sein.

**Dashboard öffnet sich nicht?**
→ Öffne `http://localhost:9090` manuell im Browser.

**Logs anzeigen:**
```bash
docker compose -f ~/FluxBot/docker-compose.yml logs -f
```

---

## 👨‍💻 Für Entwickler

```bash
git clone https://github.com/ki-werke/fluxbot.git
cd fluxbot
go build ./cmd/fluxbot/

# Oder mit Make:
make build-linux
make build-windows
make build-macos
```

**Docker lokal bauen:**
```bash
docker compose up --build -d fluxbot
docker compose logs -f fluxbot
```

---

## 🗺️ Roadmap

| Status | Feature |
|--------|---------|
| ✅ | Telegram, WhatsApp, Slack, Matrix, Discord |
| ✅ | Langzeit-Gedächtnis |
| ✅ | Skills-System |
| ✅ | Bild-Generierung (FLUX, Seedream, Stability, …) |
| ✅ | Spracherkennung (Groq Whisper) |
| ✅ | Persönlichkeit (SOUL.md) |
| ✅ | 20+ KI-Anbieter (OpenRouter, Claude, GPT, Gemini, Groq, DeepSeek, …) |
| ✅ | Web-Dashboard |
| ✅ | Setup-Wizard |
| ✅ | Direktinstallation (.exe ohne Docker) |
| ✅ | Docker One-Liner-Installer |
| ✅ | AES-256-GCM Secret-Vault |
| ✅ | VirusTotal-Scan auf allen Kanälen |
| ✅ | HMAC-gesichertes Dashboard |
| 🔧 | Ollama-Integration (lokaler AI-Betrieb) |
| 🔧 | Skills-Marketplace |
| 🔧 | VirusTotal-Tab im Dashboard |

---

## 📄 Lizenz

MIT License – © KI-WERKE

Du darfst FluxBot frei verwenden, verändern und weitergeben – auch kommerziell.

---

<div align="center">
<br/><br/>
<strong>Gebaut mit ❤️ von KI-WERKE</strong>
<br/>
<sub>Fragen? Issues? Pull Requests? Kontakt: kiwerke@gmail.com</sub>
</div>
