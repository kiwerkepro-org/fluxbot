<div align="center">

<img src="assets/virustotal-logo.png" width="140" alt="VirusTotal"/>
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
<img src="assets/fluxion-logo.png" alt="FluxBot Logo" width="220"/>
&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
<img src="assets/bitwarden-logo.png" width="140" alt="Bitwarden"/>

<br/>

# FluxBot

**powered by KI-WERKE**

![Go](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/Lizenz-MIT-green)
![Self-Hosted](https://img.shields.io/badge/Self--Hosted-100%25-blueviolet)
![Platforms](https://img.shields.io/badge/Windows%20%7C%20Linux%20%7C%20macOS-lightgrey)
![Release](https://img.shields.io/github/v/release/kiwerkepro-org/fluxbot?label=Release&color=blue)

**Dein persönlicher KI-Assistent – selbst gehostet, sicher, grenzenlos erweiterbar.**

</div>

---

FluxBot läuft auf deinem eigenen Rechner oder Server – nicht in der Cloud. Du behältst die volle Kontrolle über deine Daten und kannst jeden KI-Anbieter frei wählen.

---

## 💚 Unterstützen

FluxBot ist Open Source und kostenlos. Wenn dir das Projekt gefällt, kannst du die Entwicklung mit einer Spende unterstützen:

<div align="center">

<a href="https://www.paypal.com/donate/?hosted_button_id=PJAUB53Z734TS" target="_blank"><img src="https://img.shields.io/badge/PayPal-Spenden-blue?logo=paypal&logoColor=white" alt="PayPal Spenden"/></a>

<a href="https://www.paypal.com/donate/?hosted_button_id=PJAUB53Z734TS" target="_blank"><strong>Jetzt via PayPal spenden</strong></a>

</div>

---

## 🧠 Was kann FluxBot?

| Funktion | Beschreibung |
|----------|-------------|
| 💬 **Schreiben & Fragen** | Beantwortet Fragen, schreibt Texte, übersetzt, erklärt – alles per Chat |
| 🎙️ **Sprachnachrichten** | Sprachnachricht schicken → FluxBot transkribiert und antwortet per Sprache oder Text |
| 🎨 **Bilder erstellen** | „Erstelle ein Bild von …" – unterstützt FLUX, Stability, OpenAI, FAL und mehr |
| 🌐 **Browser-Steuerung** | Webseiten öffnen, Screenshots machen, Formulare ausfüllen, Multi-Step-Aktionen |
| 🔍 **Websuche** | Aktuelle Infos, News, Recherche direkt im Chat (via Tavily) |
| 📅 **Google Workspace** | Kalender, Docs, Sheets, Drive, Gmail – direkt aus dem Chat steuern |
| ⏰ **Erinnerungen** | „Erinnere mich morgen um 9 Uhr an …" – persistente Cron-Reminder |
| 🧠 **Langzeit-Gedächtnis** | FluxBot merkt sich wichtige Fakten dauerhaft über Sessions hinweg |
| 🎭 **Eigene Persönlichkeit** | Über `SOUL.md` gibst du FluxBot eine eigene Identität und Verhaltensregeln |
| 🛠️ **Skills** | Füge eigene Fähigkeiten als einfache Textdateien hinzu – HMAC-signiert |
| 🤖 **Self-Extend** | FluxBot kann eigene Skills schreiben und seinen Code erweitern |
| 🖥️ **Web-Dashboard** | Einstellungen, Secrets, Skills, Pairing – alles bequem im Browser |
| 🔒 **Sicherheits-Scan** | Alle Dateien & Links werden automatisch via VirusTotal geprüft |
| 👥 **DM-Pairing** | Neue Nutzer anfragen lassen, per Dashboard freigeben oder blockieren |

---

## 📱 Unterstützte Messenger

| Messenger | Status |
|-----------|--------|
| Telegram  | ✅ Produktiv (Text, Sprache, Bilder, Dateien) |
| Discord   | ✅ Implementiert |
| Slack     | ✅ Implementiert |
| WhatsApp  | ✅ Implementiert |
| Matrix    | ✅ Implementiert |

---

## 🤖 Unterstützte KI-Anbieter

FluxBot funktioniert mit allen großen KI-Anbietern. Du wählst deinen Anbieter bequem im Dashboard.

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

> **Tipp:** Mit **Ollama** kannst du FluxBot komplett lokal betreiben – kein API-Key, keine Cloud-Kosten, volle Datenschutzkontrolle.

---

## ⚡ Installation

### Direktinstallation *(empfohlen)*

Lade die passende Datei von der <a href="https://github.com/kiwerkepro-org/fluxbot/releases" target="_blank">Releases-Seite</a> herunter:

| Betriebssystem | Datei |
|---------------|-------|
| Windows | `fluxbot-windows-amd64.exe` |
| Linux | `fluxbot-linux-amd64` |
| macOS (Intel) | `fluxbot-darwin-amd64` |
| macOS (Apple Silicon) | `fluxbot-darwin-arm64` |

Datei starten → Der Einrichtungsassistent öffnet sich automatisch im Browser unter `http://localhost:9090`. KI-Anbieter, API-Key und Messenger eintragen – fertig.

**Windows AutoStart:** Das mitgelieferte `AUTOSTART-EINRICHTEN.ps1` einmalig als Admin ausführen – FluxBot startet dann automatisch bei jedem Login im Hintergrund.

---

## ⬆️ Updaten

Neue Version von der <a href="https://github.com/kiwerkepro-org/fluxbot/releases" target="_blank">Releases-Seite</a> herunterladen und die alte Datei ersetzen. Konfiguration und Daten bleiben erhalten.

---

## 🌐 Browser-Steuerung

FluxBot kann echte Webseiten steuern – powered by <a href="https://playwright.dev/" target="_blank">Playwright</a>.

| Befehl | Aktion |
|--------|--------|
| „Mach einen Screenshot von bild.de" | Screenshot als Bild im Chat |
| „Rufe example.com auf" | Öffnet ein sichtbares Browserfenster |
| „Lies den Text von example.com" | Extrahiert den Seiteninhalt als Text |
| „Geh auf google.com, such nach X und mach einen Screenshot" | Multi-Step Browser-Aktionen |
| „Füll das Kontaktformular auf example.com aus" | Formular automatisch ausfüllen |

**Features:**
- Anti-Bot-Detection (Stealth-Modus, realistischer User-Agent)
- Cookie-Banner werden automatisch geschlossen
- Domain-Whitelist für kontrollierte Nutzung
- Passwort-Felder werden nie automatisch ausgefüllt

---

## 📅 Google Workspace Integration

FluxBot kann deinen Google Kalender, Docs, Sheets, Drive und Gmail direkt aus dem Chat steuern.

| Dienst | Beispiel |
|--------|---------|
| **Kalender** | „Welche Termine habe ich morgen?" / „Erstelle einen Termin am Freitag um 14 Uhr" |
| **Docs** | „Erstelle ein Google Doc mit dem Titel Projektplan" |
| **Sheets** | „Lies die Tabelle XY aus" / „Schreib diese Daten in Sheets" |
| **Gmail** | „Schick eine Mail an max@example.com" |
| **Drive** | „Was liegt in meinem Drive?" |

> Setup: Google Cloud Console → OAuth 2.0 Client ID erstellen → Im Dashboard unter Integrationen verbinden.

---

## 💬 Chat-Befehle

Schreib einfach normal – FluxBot versteht natürliche Sprache. Diese Funktionen sind besonders:

**Gedächtnis:**
```
Merke dir, ich wohne in Wien        → speichert dauerhaft
Vergiss das mit Wien                → löscht diesen Fakt
Vergiss alles                       → löscht das komplette Gedächtnis
```

**Erinnerungen:**
```
Erinnere mich morgen um 9 Uhr an das Meeting
Erinnere mich jeden Montag an den Wochenbericht
Welche Erinnerungen habe ich?
```

**Gespräch zurücksetzen:**
```
Neues Gespräch    → Löscht den Chatverlauf (Fakten bleiben)
Reset             → Gleiche Funktion
```

**Bilder erstellen:**
```
Erstelle ein Bild von einem Sonnenuntergang am Meer
Male mir ein futuristisches Wien
```

---

## 🖥️ Web-Dashboard

Das Dashboard ist unter `http://localhost:9090` erreichbar:

- **Status** – Bot-Status, aktive Kanäle, Version
- **Konfiguration** – KI-Anbieter, Modelle, Messenger, Integrationen
- **Skills** – Skills verwalten, signieren, neu laden
- **VirusTotal** – Scan-Status, History, Statistiken
- **Pairing** – Neue Nutzer freigeben oder blockieren
- **System** – Secrets (AES-256-GCM Vault), Danger Zone
- **Hilfe** – Integrierte Dokumentation mit Suchfunktion

Geschützt durch Login + HMAC-signierte API-Requests.

---

## 🎭 Persönlichkeit anpassen (SOUL.md)

<div align="right">
<img src="assets/fluxion-character.png" alt="Fluxion Character" width="130">
</div>

Bearbeite `workspace/SOUL.md` um FluxBot eine eigene Persönlichkeit zu geben:

```
Du bist FluxBot, der KI-Assistent von Mein Unternehmen GmbH.
Du antwortest immer freundlich und auf Deutsch.
Du sprichst Kunden mit "Sie" an.
```

Nach dem Speichern FluxBot neu starten – fertig.

---

## 🛠️ Eigene Skills hinzufügen

Ein Skill ist eine `.md`-Textdatei im Ordner `workspace/skills/`. FluxBot erkennt Keywords und lädt den passenden Skill automatisch.

**Beispiel:** `workspace/skills/wordpress.md`
```markdown
---
name: wordpress
tags: [wordpress, plugin, theme, wp]
---

# WordPress-Experte

- Beziehe alle Antworten auf WordPress
- Frage zuerst: Plugin oder Theme?
- Empfehle immer quelloffene Lösungen
```

Skills müssen im Dashboard signiert werden (HMAC-SHA256). Unsignierte Skills werden im Dashboard mit einem Warnzeichen angezeigt.

**Mitgelieferte Skills:** Browser-Screenshot, Browser-Read, Browser-Fill, Browser-Actions, Web-Suche, Google Calendar, Google Docs, Gmail, Cron-Reminder, Self-Skill-Writer, Self-Code-Reader, Self-Code-Extend

---

## 🔒 Sicherheit

| Schutzmaßnahme | Beschreibung |
|---------------|-------------|
| **Zugangskontrolle** | AllowFrom-Whitelist + DM-Pairing für dynamische Freigabe |
| **VirusTotal-Scan** | Alle Dateien und Links auf allen Kanälen automatisch geprüft |
| **AES-256-GCM Vault** | API-Keys und Secrets verschlüsselt – nie im Klartext |
| **HMAC-Signierung** | Dashboard-API und Skills kryptografisch signiert |
| **Injection-Schutz** | 40+ Muster erkennen und blockieren Prompt-Injection (DE & EN) |
| **Rate-Limiting** | Max. 30 Nachrichten pro Minute pro Nutzer |
| **Browser-Sicherheit** | Domain-Whitelist, Passwort-Felder blockiert |
| **Audit-Log** | Alle Aktivitäten protokolliert mit Intent, Duration, Error-Code |
| **Tailscale VPN** | Dashboard nur über verschlüsseltes VPN erreichbar (optional) |

---

## 🗂️ Dateistruktur

```
fluxbot/
├── cmd/fluxbot/              ← Programm-Einstiegspunkt
├── pkg/
│   ├── agent/                ← FluxAgent, Session-Management, Agent-Loop
│   ├── browser/              ← Playwright Browser-Steuerung
│   ├── channels/             ← Telegram, Discord, Slack, Matrix, WhatsApp
│   ├── config/               ← Konfiguration, Validation
│   ├── dashboard/            ← HTTP-Server, API, dashboard.html
│   ├── security/             ← Vault, HMAC, VirusTotal, Audit
│   ├── skills/               ← Skill-Loader, HMAC-Signatur
│   ├── provider/             ← 15+ KI-Anbieter
│   ├── search/               ← Tavily Web-Suche
│   ├── voice/                ← Sprach-I/O (Groq Whisper, Vertex TTS)
│   ├── imagegen/             ← Bildgenerierung (FAL, OpenAI, Stability, …)
│   ├── email/                ← SMTP E-Mail-Versand
│   └── google/               ← Google Workspace OAuth2 Client
├── workspace/
│   ├── config.json           ← Konfiguration
│   ├── .secrets.vault        ← Verschlüsselter Secret-Speicher
│   ├── SOUL.md               ← Persönlichkeit (optional)
│   ├── skills/               ← Skill-Dateien (.md + .sig)
│   ├── sessions/             ← Gesprächsverläufe
│   └── logs/                 ← Protokolle
└── assets/                   ← Logos und Bilder
```

---

## 🆘 Häufige Probleme

**FluxBot antwortet nicht?**
→ Prüfe, ob deine User-ID korrekt in `allowFrom` steht. Bei Telegram hilft `@userinfobot`.

**Dashboard öffnet sich nicht?**
→ Öffne `http://localhost:9090` manuell im Browser.

**Browser-Screenshots funktionieren nicht?**
→ Playwright muss installiert sein: `go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps`

**Logs anzeigen:**
→ `workspace/logs/fluxbot.log` oder im Dashboard unter Logs.

---

## 👨‍💻 Für Entwickler

```bash
git clone https://github.com/kiwerkepro-org/fluxbot.git
cd fluxbot
go mod tidy
go build -o fluxbot ./cmd/fluxbot/
```

---

## 🗺️ Roadmap

| Status | Feature |
|--------|---------|
| ✅ | 5 Messenger (Telegram, Discord, Slack, WhatsApp, Matrix) |
| ✅ | 15+ KI-Anbieter inkl. Ollama (lokal) |
| ✅ | Langzeit-Gedächtnis + Cron-Reminder |
| ✅ | Bild-Generierung (FLUX, Stability, OpenAI, FAL, …) |
| ✅ | Spracherkennung & Sprachausgabe (Groq Whisper, Vertex TTS) |
| ✅ | Browser-Steuerung (Screenshots, Lesen, Formulare, Multi-Step) |
| ✅ | Google Workspace (Kalender, Docs, Sheets, Drive, Gmail) |
| ✅ | Web-Dashboard mit Login, HMAC, Hilfe-System |
| ✅ | AES-256-GCM Secret-Vault + VirusTotal-Scan |
| ✅ | DM-Pairing Mode (dynamische Nutzer-Freigabe) |
| ✅ | Self-Extend (Bot schreibt eigene Skills & Code) |
| ✅ | Skills-System mit HMAC-Signierung |
| 🔧 | Skills-Marketplace |
| 🔧 | Video-Generierung |

---

## 📄 Lizenz

MIT License – © KI-WERKE

Du darfst FluxBot frei verwenden, verändern und weitergeben – auch kommerziell.

---

<div align="center">
<br/>
<strong>Gebaut mit Leidenschaft von KI-WERKE</strong>
<br/>
<sub>Fragen? Issues? Pull Requests? → <a href="https://github.com/kiwerkepro-org/fluxbot/issues" target="_blank">GitHub Issues</a> · kiwerke@gmail.com</sub>
</div>
