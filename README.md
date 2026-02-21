<div align="center">
<img src="fluxion_logo.png" alt="Fluxion Logo" width="200">
<h1>🤖 FluxBot</h1>
<strong>powered by <a href="https://ki-werke.de">KI-WERKE</a></strong>
</div>


Dein persönlicher KI-Assistent für Telegram, WhatsApp, Slack und mehr.

FluxBot ist ein selbst gehosteter KI-Bot – er läuft auf deinem eigenen Server oder Computer, nicht irgendwo in der Cloud. Du behältst die volle Kontrolle über deine Daten und kannst jeden KI-Anbieter verwenden, den du möchtest.

Entwickelt von KI-WERKE · Gebaut mit Go · Läuft überall

🧠 Was kann FluxBot?

Funktion

Beschreibung

💬 Schreiben & Fragen			

Beantwortet Fragen, schreibt Texte, übersetzt, erklärt – alles per Chat

🎙️ Sprachnachrichten

Sprachnachricht schicken → FluxBot transkribiert und antwortet

🎨 Bilder erstellen

Einfach schreiben „Erstelle ein Bild von..." – fertig

🔍 Websuche

Aktuelle Infos, News, Recherche direkt im Chat

🧠 Langzeit-Gedächtnis

FluxBot merkt sich wichtige Fakten dauerhaft

🎭 Eigene Persönlichkeit

Über SOUL.md kannst du FluxBot eine eigene Identität geben

🛠️ Skills

Füge eigene Fähigkeiten als einfache Textdateien hinzu

🖥️ Web-Dashboard

Einstellungen bequem im Browser verwalten

📱 Unterstützte Messenger

Messenger

Status

Telegram

✅ Fertig & produktiv

WhatsApp

✅ Implementiert

Slack

✅ Implementiert

Matrix

✅ Implementiert

Discord

🔧 In Entwicklung

🤖 Unterstützte KI-Anbieter

FluxBot funktioniert mit allen großen KI-Anbietern. Du wirst beim ersten Start geführt, welchen du verwenden möchtest.

Empfohlen: OpenRouter, Anthropic (Claude), OpenAI (GPT), Google (Gemini), xAI (Grok), Groq, Mistral, Together, DeepSeek, Perplexity, Cohere und viele mehr – oder ein benutzerdefinierter OpenAI-kompatibler Endpunkt.

⚡ Installation

Es gibt zwei Wege – wähle den, der zu dir passt:

🖱️ Weg 1 – Direktinstallation (empfohlen für Desktop-Nutzer)

Kein Docker, kein Terminal, kein JSON. Einfach herunterladen und starten.

Lade die passende Datei von der Releases-Seite herunter:

Windows: fluxbot-windows-amd64.exe

Linux: fluxbot-linux-amd64

macOS (Intel): fluxbot-darwin-amd64

macOS (Apple Silicon): fluxbot-darwin-arm64

Datei starten (Doppelklick unter Windows; unter Linux/macOS ggf. chmod +x ausführen).

Der Einrichtungsassistent öffnet sich automatisch im Browser – dort trägst du deinen KI-Anbieter, deinen API-Key und deinen Messenger ein. FluxBot startet danach sofort.

Das war's. Kein manuelles Bearbeiten von Konfigurationsdateien notwendig.

🐳 Weg 2 – Docker (empfohlen für Server und fortgeschrittene Nutzer)

Voraussetzung: Docker Desktop muss installiert und gestartet sein.

Windows (PowerShell):

irm [https://fluxbot.ki-werke.de/install.ps1](https://fluxbot.ki-werke.de/install.ps1) | iex


Linux / macOS (Terminal):

curl -fsSL [https://fluxbot.ki-werke.de/install.sh](https://fluxbot.ki-werke.de/install.sh) | bash


Das Skript:

prüft, ob Docker läuft

erstellt das Verzeichnis ~/FluxBot

zieht das aktuelle FluxBot-Image

startet den Container

öffnet http://localhost:8090 im Browser

Dort startet beim ersten Aufruf automatisch der Einrichtungsassistent. Deine Konfiguration und alle Daten werden dauerhaft in ~/FluxBot/fluxbot-data gespeichert und bleiben auch nach Updates erhalten.

⬆️ Updaten

Direktinstallation: neue Version von der Releases-Seite herunterladen und die alte ersetzen. Die config.json im selben Verzeichnis bleibt erhalten.

Docker:

docker compose -f ~/FluxBot/docker-compose.yml pull
docker compose -f ~/FluxBot/docker-compose.yml up -d


💬 Chat-Befehle

Schreib einfach auf Deutsch mit FluxBot – du brauchst keine speziellen Befehle. Aber diese Sätze haben besondere Funktionen:

Gedächtnis

Merke dir, ich wohne in Wien        → FluxBot speichert das dauerhaft
Merk dir: ich bin Grafikdesigner    → FluxBot speichert das dauerhaft
Vergiss das mit Wien                → Löscht diesen einen Fakt
Vergiss alles                       → Löscht das komplette Gedächtnis


Gespräch zurücksetzen

Neues Gespräch    → Löscht den bisherigen Chatverlauf (Fakten bleiben erhalten)
Reset             → Gleiche Funktion


Bilder erstellen

Erstelle ein Bild von einem Sonnenuntergang am Meer
Male mir ein futuristisches Wien
Generiere ein Foto von einer Katze im Weltall


Websuche

Was sind die aktuellen News zu KI?
Suche nach dem Wetter in Berlin morgen


🖥️ Web-Dashboard

Das Dashboard ist unter http://localhost:8090 erreichbar (nach dem Start). Dort kannst du:

KI-Anbieter und API-Keys wechseln

Messenger ein- und ausschalten

Nachrichten und Logs einsehen

Skills verwalten

🎭 Persönlichkeit anpassen (SOUL.md)

<div align="right">
<img src="fluxion.png" alt="Fluxion Character" width="150">
</div>

Du kannst FluxBot eine eigene Persönlichkeit geben – einfach die Datei workspace/SOUL.md (Direktinstallation) bzw. ~/FluxBot/fluxbot-data/SOUL.md (Docker) bearbeiten:

Du bist FluxBot, der KI-Assistent von Mein Unternehmen GmbH.
Du antwortest immer freundlich und auf Deutsch.
Du sprichst Kunden mit "Sie" an.
Du gibst niemals Auskunft über Konkurrenz-Produkte.


Nach dem Speichern FluxBot neu starten – fertig.

🛠️ Eigene Skills hinzufügen

Ein Skill ist eine .md-Textdatei im Ordner skills/. Wenn ein Nutzer ein Keyword schreibt, lädt FluxBot automatisch den passenden Skill.

Beispiel: skills/wordpress.md

# WordPress-Experte

## Keywords
wordpress, plugin, theme, wp

## Regeln
- Beziehe alle Antworten auf WordPress
- Frage zuerst: Plugin oder Theme?
- Empfehle immer quelloffene Lösungen


FluxBot erkennt das Keyword wordpress und verhält sich automatisch als WordPress-Experte.

🗂️ Dateistruktur

fluxbot/
│
├── 📁 workspace/               ← Deine persönlichen Dateien (gitignored!)
│   ├── config.json             ← Deine Einstellungen (vom Wizard erstellt)
│   ├── config.example.json     ← Vorlage (nur zur Referenz)
│   ├── SOUL.md                 ← Persönlichkeit von FluxBot (optional)
│   ├── IDENTITY.md             ← Zusätzliche Identitätsdatei (optional)
│   ├── 📁 sessions/            ← Gesprächsverläufe (automatisch)
│   ├── 📁 logs/                ← Protokoll-Dateien (automatisch)
│   └── 📁 skills/              ← Deine eigenen Skills (.md-Dateien)
│
├── 📁 cmd/fluxbot/             ← Programm-Einstiegspunkt
├── 📁 pkg/                     ← Interne Pakete
│
├── Dockerfile                  ← Docker-Image-Definition
├── docker-compose.yml          ← Entwicklungs-Compose (mit lokalem Build)
├── docker-compose.prod.yml     ← Produktions-Compose (zieht fertiges Image)
├── install.ps1                 ← Windows Installer-Skript
├── install.sh                  ← Linux/macOS Installer-Skript
└── Makefile                    ← Hilfsbefehle zum Bauen


🔒 Sicherheit

Nur du hast Zugriff – über allowFrom in den Einstellungen legst du fest, wer mit FluxBot schreiben darf. Nicht-autorisierte Nutzer werden stillschweigend ignoriert.

Weitere Schutzmaßnahmen die automatisch aktiv sind:

Injection-Schutz – erkennt und blockiert Versuche, FluxBot zu manipulieren (40+ Muster auf DE & EN)

Rate-Limiting – maximal 30 Nachrichten pro Minute pro Nutzer

Audit-Log – alle Aktivitäten werden protokolliert (nur Metadaten, keine Inhalte) – DSGVO-konform

Automatische Log-Löschung – Logs werden nach 90 Tagen automatisch gelöscht

API-Keys niemals im Code – alle Zugangsdaten nur in workspace/config.json (gitignored)

🆘 Häufige Probleme

FluxBot antwortet nicht?
→ Prüfe, ob deine Telegram-User-ID korrekt in allowFrom steht (@userinfobot hilft dabei).

Docker-Fehler beim Start?
→ Ist Docker Desktop geöffnet? Das Whale-Icon in der Taskleiste muss aktiv sein.

Der Einrichtungsassistent öffnet sich nicht (Docker)?
→ Öffne http://localhost:8090 manuell im Browser.

Logs anzeigen:

docker compose -f ~/FluxBot/docker-compose.yml logs -f


Für Entwickler – Logs beim direkten Start:

./fluxbot --config workspace/config.json


👨‍💻 Für Entwickler

Wer FluxBot selbst bauen möchte:

git clone [https://github.com/ki-werke/fluxbot.git](https://github.com/ki-werke/fluxbot.git)
cd fluxbot
go build ./cmd/fluxbot/

# Oder mit Make:
make build-linux
make build-windows
make build-macos


Für Docker lokal bauen:

docker compose up --build -d fluxbot
docker compose logs -f fluxbot


🗺️ Roadmap

[x] Telegram, WhatsApp, Slack, Matrix

[x] Langzeit-Gedächtnis

[x] Skills-System

[x] Bild-Generierung (FLUX.2 Pro, Seedream 4.5)

[x] Spracherkennung (Groq Whisper)

[x] Persönlichkeit (SOUL.md / IDENTITY.md)

[x] 20+ KI-Anbieter (OpenRouter, Claude, GPT, Gemini, xAI, Groq, DeepSeek ...)

[x] Web-Dashboard (Einstellungen per Browser)

[x] Setup-Wizard (Einrichtung ohne JSON-Bearbeitung)

[x] Direktinstallation (.exe ohne Docker)

[x] Docker One-Liner-Installer

[x] Discord

[ ] Skills-Marketplace


📄 Lizenz

MIT License – © KI-WERKE

Du darfst FluxBot frei verwenden, verändern und weitergeben – auch kommerziell.

<div align="center">
<img src="kiwerke-logo.png" alt="KI-WERKE Logo" width="50">




<strong>Gebaut mit ❤️ von <a href="https://ki-werke.de">KI-WERKE</a></strong>




<sub>Fragen? Issues? Pull Requests? Immer willkommen.</sub>
</div>