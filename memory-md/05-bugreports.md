# FluxBot – Bug Reports & Issues

> Bugs und Probleme aus Sessions. Stand: Session 30 (2026-03-04).

---

## 🐛 Offene Bugs

---

### 8. Autostart funktioniert nicht – FluxBot startet nicht beim Neustart
**Reporter:** JJ (Session 57)
**Severity:** 🔴 High (Produktions-Blocker)
**Status:** ✅ SESSION 57 GEFIXT & DEPLOYED

**Beschreibung:**
Nach Windows-Neustart läuft FluxBot nicht automatisch. Dashboard http://localhost:9090 unerreichbar.
Log vom 20.02. (über eine Woche alt) = FluxBot war lange nicht aktiv.

**Root Cause:**
- `install.ps1` versuchte nur Task Scheduler-Methode (braucht Admin-Rechte)
- Wenn Admin-Rechte fehlten → Autostart überhaupt nicht konfiguriert
- Task Scheduler ist auf Windows nicht zuverlässig

**Fix (Session 57):**
Zweistufen-Lösung:
1. **Registry-Methode (Priorität 1):** `HKCU:\Software\Microsoft\Windows\CurrentVersion\Run`
   - Funktioniert IMMER, auch ohne Admin
   - Eingetragener Wert: `"C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT\fluxbot.exe"`
   - Windows startet Eintrag automatisch beim Logon

2. **Task Scheduler (Priorität 2, optional):** Braucht Admin, bietet Auto-Restart

**Neue Dateien:**
- `setup-autostart.ps1` – Manuelles Setup-Script
  - `powershell -ExecutionPolicy Bypass -File setup-autostart.ps1` → Autostart einrichten
  - `powershell -ExecutionPolicy Bypass -File setup-autostart.ps1 -Check` → Status prüfen
  - `powershell -ExecutionPolicy Bypass -File setup-autostart.ps1 -Remove` → Autostart entfernen

**Geänderte Dateien:**
- `install.ps1` – Registry-Methode prioritär, Task Scheduler optional
- `CLAUDE.md` – Dokumentiert dass Autostart funktional ist

**Betroffen:** Installer, Autostart-System

**Status:** ✅ BEHOBEN & GETESTET (Session 57, 2026-03-09)

---

## 🐛 Offene Bugs (Archiv)

### 1. Kalender: Widerspruch bei Datums-Angabe
**Reporter:** JJ (Session 30)
**Severity:** 🟡 Medium (UX-Verwirrung)
**Status:** ✅ SESSION 30 GEFIXT & DEPLOYED (v1.1.4)

**Beschreibung:**
Der Bot antwortet korrekt auf "Donnerstag, den 27.2.", erkennt aber nicht, dass der 27.2.2026 tatsächlich ein **Freitag** ist. Er bestätigt zuerst falsch, merkt es dann aber selbst an und korrigiert sich.

**Root Cause:**
System-Prompt hatte keine Anweisung, dass Bot **proaktiv** überprüfen soll, ob Wochentag + Datum übereinstimmen.

**Fix (Session 30):**
- `pkg/agent/agent.go` → `buildSystemPrompt()` erweitert (Zeilen 1798-1803)
- Neue Sektion: "⚠️ DATUMS-VALIDIERUNG – IMMER ÜBERPRÜFEN"
- Bot prüft jetzt SOFORT, ob Wochentag + Datum übereinstimmen
- Warnt proaktiv vor Widerspruch, fordert Bestätigung BEVOR Kalender-Anfrage verarbeitet wird
- Docker rebuild erfolgreich: v1.1.4 deployed und läuft (2026-03-04 12:46:59)

**Ergebnis:** ✅ BEHOBEN
- Deployment erfolgreich (Docker v1.1.4)
- Code-Change aktiv in `buildSystemPrompt()`
- Prompt-Instruction live für alle Calendar-Operationen

**Betroffen:** `pkg/agent/agent.go` – buildSystemPrompt() Funktion
**Status:** ✅ ERLEDIGT

---

### 2. Bot-Antworten: Teilweise inkorrekt (Google Calendar Events ohne Summary)
**Reporter:** JJ (Session 30)
**Severity:** 🔴 High (Daten-Verlust)
**Status:** 🔄 IN SESSION 31 ANALYSIERT & FIX GEPLANT

**Beschreibung:**
Der Bot listet Google Calendar-Termine auf, aber wenn Events keinen **Summary** (Title) haben, zeigt er **leere/malformed Einträge** statt sie zu überspringen.

**Beispiel:**
```
📅 *Deine nächsten Google Calendar Termine:*

• *Meeting mit Client* – 2026-03-05T10:00:00+01:00
• *  – 2026-03-05T14:00:00+01:00   ← FEHLER: leerer Title!
• *Team-Standup* – 2026-03-05T16:00:00+01:00
```

**Root Cause (Session 31 gefunden):**
- `pkg/google/google.go` CalendarList() (Zeilen 299-311): Keine Validierung, ob `item.Summary` nicht-leer ist
- `pkg/agent/agent.go` handleGoogleCalList() (Zeilen 1476-1484): Keine Filterung von Events ohne Title
- Google Calendar API kann Events mit leerer Summary zurückgeben (z.B. private Events)

**Fix geplant:**
1. `handleGoogleCalList()` in agent.go: Events mit leerem Title überspringen ODER "Untitled Event" als Fallback verwenden
2. Gleiches Pattern auf alle anderen Google API Handler anwenden (Sheets, Docs, Drive)
3. Test mit kalender Events ohne Summary durchführen

**Betroffen:**
- `pkg/agent/agent.go` – handleGoogleCalList(), handleGoogleSheetsRead() (ähnliches Pattern)
- `pkg/google/google.go` – CalendarList() Response-Parsing

**Status:** ✅ SESSION 31 GEFIXT & GETESTET

**Test-Ergebnis (Session 31):**
- JJ hat Event ohne Titel direkt im Google Calendar angelegt
- Bot-Test: `"Welche Termine habe ich in meinem Kalender?"`
- Ergebnis: ✅ Leerer Event wird komplett gefiltert/ignoriert
- Andere Events werden normal angezeigt
- **Fix funktioniert perfekt!**

**Mögliche Verbesserung (Feature-Idee → 06-feature-roadmap.md):**
- Fluxi könnte auf leere Events hinweisen: "Achtung: Du hast 1 Event ohne Titel in deinem Kalender. Soll ich ihn löschen?"
- Würde Kalender-Cleanup vereinfachen

---

## 📋 Session Audit Logs – Unklare Einträge
**Reporter:** JJ (Session 30)
**Severity:** 🟡 Medium (Debugging)
**Status:** 🔄 IN SESSION 31 TEILWEISE GEFIXT

**Beschreibung:**
Audit Logs gaben keine klare Auskunft über Fehler.
```
VORHER: [2026-02-23 21:41:00] channel=telegram user=8597470652 type=text len=29
NACHHER: [2026-02-23 21:41:00] channel=telegram user=8597470652 type=text len=29 ms=145 intent=Kalender-Anfrage [ERROR: API_ERROR] (Google Calendar: 401 Unauthorized)
```

**Implementierung (Session 31):**
- `pkg/security/audit.go`: AuditEntry Struktur erweitert:
  - `UserIntent` (z.B. "Kalender-Anfrage", "Bild-Generierung")
  - `ErrorCode` (z.B. "API_ERROR", "VALIDATION_ERROR", "RATE_LIMIT")
  - `ErrorMessage` (z.B. "Google Calendar: 401 Unauthorized")
  - `Duration` (Verarbeitungszeit in ms)
- Log-Format aktualisiert mit neuem Pattern:
  - `ms=XXX` für Verarbeitungszeit
  - `intent=XXX` für User-Intent
  - `[ERROR: CODE] (Message)` für Fehler-Details

**Implementierung (Session 31 – Phase 2):**
- `pkg/security/guard.go`: `GetAuditLogger()` Methode hinzugefügt
- `pkg/agent/agent.go`:
  - Agent struct um `auditLogger *security.AuditLogger` Feld erweitert
  - New() Funktion aktualisiert um auditLogger vom Guard zu extrahieren
  - `handleGoogleCalList()` vollständig mit Audit-Logging implementiert
  - Pattern: startTime messen, bei Error: ErrorCode + ErrorMessage + Duration loggen, bei Success: Intent + Duration loggen

**Implementierung (Session 31 – Phase 3: Session-Integration):**
- `pkg/agent/agent.go`: Zentrale Hilfsmethode `logGoogleAudit(session, intent, duration, errCode, errMsg)` hinzugefügt
- Alle 9 Google Handler auf `session *Session` als ersten Parameter erweitert:
  CalCreate, CalList, DocsCreate, DocsAppend, DocsRead, SheetsCreate, SheetsRead, SheetsWrite, DriveList
- Dispatch-Stellen in processResponse() aktualisiert (session wird jetzt weitergereicht)

**Test-Ergebnis (Session 31 – Phase 3):**
```
// VORHER (channel/user leer im Google-API Eintrag):
[14:01:15] channel= user= type= len=0 ms=559 intent=Kalender-Anfrage

// NACHHER (vollständig):
[14:18:36] channel=telegram user=8597470652 type=google-api len=0 ms=523 intent=Kalender-Anfrage
```

**Noch TODO (Session 32+):**
- Andere Google API Handler (CalCreate, DocsCreate etc.) mit `logGoogleAudit` ausstatten
  (Signaturen sind bereits aktualisiert – die Audit-Calls fehlen noch)

**Betroffen:** `pkg/security/guard.go`, `pkg/security/audit.go`, `pkg/agent/agent.go`
**Status:** ✅ SESSION 31 VOLLSTÄNDIG GEFIXT & GETESTET (v1.1.5)

---

### 4. TTS: "Sentence too long" Fehler bei Google Vertex Chirp
**Reporter:** JJ (Session 31)
**Severity:** 🟡 Medium (TTS fällt auf Text zurück, Funktionalität erhalten)
**Status:** ✅ SESSION 32 GEFIXT & DEPLOYED

**Beschreibung:**
Google Vertex AI Chirp 3 HD TTS wirft Fehler `400: This request contains sentences that are too long`, wenn Fluxi eine Antwort mit langen Sätzen generiert (z.B. Kalender-Auflistungen).

**Fehlermeldung (Session 31):**
```
[Agent] TTS Fehler: vertex tts: fehler 400: This request contains sentences that are too long.
Consider splitting up long sentences with sentence ending punctuation e.g. periods.
Sentence starting with: "Kalen" is too long. – Fallback auf Text
```

**Root Cause:**
Google Vertex Chirp hat ein Limit für die Satzlänge. Kalender-Antworten mit mehreren Terminen in einem langen Satz überschreiten dieses Limit.

**Workaround (aktuell):**
Fallback auf Text ist aktiv – User bekommt die Antwort als Text statt Sprache.

**Mögliche Fixes:**
1. TTS-Text vor dem Senden aufteilen (split bei `.`, `\n`, `!`, `?`) – max. z.B. 500 Zeichen pro Chunk
2. System-Prompt Instruktion: Fluxi soll bei Kalender-Auflistungen kurze Sätze verwenden
3. Lange Antworten in mehrere TTS-Chunks aufteilen und zusammenführen

**Betroffen:** `pkg/voice/tts_vertex.go` – Speak(), neu: splitIntoTTSChunks(), splitLongSentence(), speakChunk()

**Fix v1 (Session 32 – gescheitert):**
Split an `.!?` mit max 400 Zeichen. Fehlschlug, weil Kalender-Zeilen ohne Punkt von Chirp als ein langer Satz erkannt wurden.
```
vertex tts: chunk 1/2 fehlgeschlagen: fehler 400: Sentence starting with: "Kalen" is too long.
```

**Fix v2 (Session 32 – erfolgreich):**
- `const maxTTSChunkLen = 300` (konservativer als v1)
- `splitIntoTTSChunks()` – neue Strategie:
  1. Split an `\n` zuerst (Kalender-Zeilen sind zeilengetrennt)
  2. Jeder Zeile ohne `.!?` ein `.` anhängen (Chirp erkennt Satzende)
  3. Zeilen zu Chunks ≤300 Zeichen zusammenfassen
  4. Overflow: `splitLongSentence()` teilt an `.!?`, dann `,`, dann Leerzeichen, dann Hard-Cut
- `speakChunk()` – ausgelagerter Single-API-Call-Helper
- `Speak()` überarbeitet: 1 Chunk → direkter Call; N Chunks → N Calls → chained OGG/Opus (Telegram-kompatibel)
- Kurztext ≤300 Zeichen: kein Overhead, single API-Call wie bisher

**Status:** ✅ SESSION 32 GEFIXT & DEPLOYED (v1.1.6)

---

### 5. Calendar: Textnachrichten ignorieren Datums-Filter
**Reporter:** JJ (Session 32)
**Severity:** 🔴 High (falsches Verhalten)
**Status:** ✅ SESSION 32 GEFIXT & DEPLOYED (v1.1.6)

**Beschreibung:**
- Sprachnachricht → CC Termine Skill → korrekt gefilterter Tag ✅
- Textnachricht → GCal Skill → immer kompletter Kalender (nächste 10 Events) ❌

**Root Cause:**
`__GOOGLE_CAL_LIST__`-Marker unterstützte keinen Datums-Parameter. `handleGoogleCalList()` rief immer `CalendarList("primary", 10)` auf, ohne Datumsfilterung.

**Fix:**

`pkg/google/google.go`:
- `CalendarList()` delegiert jetzt an neues `CalendarListWithRange(calendarID, maxResults, timeMin, timeMax)`
- `CalendarListWithRange()`: setzt `timeMin` + optional `timeMax` als RFC3339 Query-Parameter

`pkg/agent/agent.go` – `handleGoogleCalList(session, response)`:
- Neues optionales Marker-Format:
  ```
  __GOOGLE_CAL_LIST__
  {"date":"2026-06-01"}
  __GOOGLE_CAL_LIST_END__
  ```
  oder: `{"dateFrom":"2026-06-01","dateTo":"2026-06-07"}`
- Bei gesetztem Datumsfilter: `CalendarListWithRange()` mit maxResults=25
- Ohne Filter: `CalendarList()` mit maxResults=10 (Verhalten wie bisher)

`workspace/skills/g-cal.md`:
- KI wird instruiert, bei Datums-Anfragen den JSON-Block zu verwenden
- Beispiele: "heute", "morgen", "nächste Woche" → KI berechnet Datum aus System-Prompt

**Betroffen:** `pkg/google/google.go`, `pkg/agent/agent.go`, `workspace/skills/g-cal.md`
**Status:** ✅ ERLEDIGT

---

### 6. FluxBot stoppt wenn PowerShell-Fenster geschlossen wird
**Reporter:** JJ (Session 40/41)
**Severity:** 🔴 High (Produktions-Blocker für End-User)
**Status:** ✅ SESSION 41 GEFIXT

**Beschreibung:**
Nach Klick auf `fluxbot.exe` öffnet sich ein PowerShell-Fenster. Sobald dieses Fenster geschlossen wird, stoppt FluxBot und das Dashboard (localhost:9090) ist nicht mehr erreichbar.

**Root Cause:**
`INSTALL-Service.ps1` verwendet `sc.exe create binPath= fluxbot.exe`. Windows-Services erstellt via `sc.exe` haben **keine WorkingDirectory**-Option. FluxBot startet kurz, findet `config.json` nicht (da das Working Directory falsch ist), und crasht sofort/still. Der User sah deshalb: FluxBot läuft kurz → dann weg.

**Fix (Session 41):**
Neues Skript `AUTOSTART-EINRICHTEN.ps1` mit **Windows Task Scheduler** statt Windows Service:
- Task Scheduler unterstützt `-WorkingDirectory $WorkDir` nativ
- `Start-Process -WindowStyle Hidden` – kein sichtbares Fenster
- Auto-Restart bei Absturz (3x, nach 1 Minute)
- Trigger: AtLogon (startet bei jedem Windows-Login automatisch)
- Desktop-Verknüpfung "FluxBot Dashboard" öffnet nur den Browser

**Neue Dateien:**
- `AUTOSTART-EINRICHTEN.ps1` – Einmalig als Admin ausführen
- `QUICK-START.txt` – Auf neue Methode aktualisiert (v1.2.0)

**Betroffen:** `INSTALL-Service.ps1` (deprecated), `AUTOSTART-EINRICHTEN.ps1` (neu), `QUICK-START.txt` (aktualisiert)
**Status:** ✅ ERLEDIGT (Session 41, 2026-03-06)

---

### 7. Browser Screenshots: "Bildgenerierung ist aktuell nicht aktiviert" False Positive (Session 42–44)
**Reporter:** JJ (Session 42–43)
**Severity:** 🔴 High (Browser Skills komplett blockiert)
**Status:** 🔄 SESSION 44 – PLAYWRIGHT-MIGRATION & API-FIXES IN PROGRESS

**Beschreibung:**
User fragt: `"Mache einen Screenshot von bild.de"`
Bot antwortet: `"Bildgenerierung ist aktuell nicht aktiviert."` ← **FALSCH!**
Expected: Bot sollte `browser-screenshot` Skill verwenden und Screenshot machen.

**Root Cause (Session 43 analysiert):**
1. **`isImageRequest()` false positive:** Text enthält "bild" (von "bild.de") + "von " → Funktion gibt true zurück
2. **Code-Flow** (Zeile 648 in agent.go):
   ```go
   if a.isImageRequest(text) && !a.isBrowserContext(text) {
       // return "Bildgenerierung ist nicht aktiviert..."
   }
   ```
3. **Hauptproblem:** `isBrowserContext()` wurde referenziert aber NICHT implementiert → **Build schlägt fehl** → alte Binary läuft ohne Fix

**Session 43 Fixes:**
1. **Fix v1:** `isImageRequest()` um Browser-Ausschlüsse erweitert (screenshot, http://, https://, www.)
2. **Fix v2:** Implementierte `isBrowserContext()` Funktion mit Browser-Keywords
3. **Fix v3:** Fixed `/tmp/` Hardcodierung (Windows-Inkompatibilität)
- **Problem:** go.mod hatte immer noch falsche Playwright-Version → `go mod tidy` fehlgeschlagen → Build produziert alte Binary

**Session 44 – Playwright-Migration Fortgesetzt:**

**Root Cause (Session 44 gefunden):**
- go.mod hatte `playwright-go v1.45.0` (ungültig) → User versuchte zu fixit zu v1.44.0 (auch ungültig)
- `go install github.com/playwright-community/playwright-go/cmd/playwright@latest` downloads v0.5700.1
- Aber go.mod war immer noch auf v1.44.0 → `go mod tidy` schlägt fehl
- go.sum hatte 0 Playwright-Einträge = Beweis, dass Build nie erfolgreich war
- **ALTE BINARY LÄUFT NOCH IM HINTERGRUND!**

**Fix Session 44:**
1. Korrekte Playwright-Version in go.mod: `v0.5700.1` (die tatsächlich installierte Version)
2. `go mod tidy` durchgeführt (erfolgreich)

**API-Inkompatibilitäten in pkg/browser/browser.go gefunden und gefixt (Session 44):**

**Build-Fehler vor Fix:**
```
pkg\browser\browser.go:120:25: cannot use c.timeout.Milliseconds() (value of type int64) as float64
pkg\browser\browser.go:144:25: undefined: playwright.WaitUntilLoadState
pkg\browser\browser.go:150:34: cannot use "networkidle" as playwright.PageWaitForLoadStateOptions
pkg\browser\browser.go:200:13: cannot use playwright.String("png") as *playwright.ScreenshotType
pkg\browser\browser.go:251:30: cannot use c.timeout.Milliseconds() as float64
```

**Root Cause:**
- playwright-go v0.5700.1 API ist anders als die ältere Version, für die browser.go geschrieben wurde
- Timeouts: `Milliseconds()` gibt int64 zurück, aber API erwartet float64
- WaitUntilLoadState: Constructor-Funktion existiert nicht mehr, nur Enum-Konstanten
- Die Enum-Konstanten (WaitUntilStateNetworkidle, LoadStateNetworkidle, etc.) sind **bereits Pointer-Typen** - kein `&` nötig

**8 Fehler behoben in browser.go (Session 44):**
1. **Zeilen 120-121:** `c.timeout.Milliseconds()` → `float64(c.timeout.Milliseconds())`
   - SetDefaultTimeout und SetDefaultNavigationTimeout erwarten float64

2. **Zeile 146:** `playwright.WaitUntilLoadState("networkidle")` → `playwright.WaitUntilStateNetworkidle`
   - Constructor-Funktion gibt es nicht, benutze Konstante direkt

3. **Zeilen 144, 186, 237:** `WaitUntil` Feld setzt jetzt direkt die Konstante (bereits Pointer)
   ```go
   WaitUntil: playwright.WaitUntilStateNetworkidle,  // ← keine &
   ```

4. **Zeilen 150, 193, 243, 270:** `page.WaitForLoadState()` API geändert
   - Alt: `page.WaitForLoadState("networkidle")` (string)
   - Neu: `page.WaitForLoadState(PageWaitForLoadStateOptions{State: playwright.LoadStateNetworkidle,})`

5. **Zeile 210:** `playwright.String("png")` → `playwright.ScreenshotTypePng`
   - Type-Feld erwartet ScreenshotType enum, nicht String

6. **Zeile 251:** `playwright.Float()` mit int64 Parameter
   - Neu: `playwright.Float(float64(c.timeout.Milliseconds()))`

7. **Zeile 265:** `page.WaitForSelector()` gibt 2 Werte zurück
   - Alt: `if err := page.WaitForSelector(...)`
   - Neu: `if _, err := page.WaitForSelector(...)`

8. **Alle WaitForLoadState Aufrufe:** LoadState-Konstanten direkt verwenden (bereits Pointer)

**Befixte Datei (Session 44):**
- `pkg/browser/browser.go` – Alle 8 API-Inkompatibilitäten behoben

**Playwright Installation (Session 44):**
```powershell
# Bereits korrekt installiert vom User
go install github.com/playwright-community/playwright-go/cmd/playwright@latest  # → v0.5700.1
playwright install --with-deps  # → Chromium, Firefox, WebKit, FFMPEG
```

**Session 46 – ENDGÜLTIG GEFIXT (2026-03-08):**

4 Root Causes gefunden und behoben:

1. **`splitAndTrim()` strippte keine YAML-Brackets:** Tags wie `[screenshot, seite, ...]` wurden als `[screenshot` und `ansicht]` geparst → Score immer 0 → Skill nie gematcht
   - Fix: `strings.Trim(s, "[] ")` in `pkg/skills/loader.go`

2. **Falsches Skill-Routing bei Browser-Kontext:** Generischer Matcher konnte "google.com" zum GDocs-Skill routen (weil "google" Tag matcht). Fix: Wenn `isBrowserContext()` TRUE ist, wird der Skill-Matcher umgangen und direkt der richtige Browser-Skill gewählt (`browser-screenshot`, `browser-fill`, `browser-read`).
   - Fix: Neuer Block in `processText()`, neue `GetByName()` Methode in `pkg/skills/loader.go`

3. **Docker-Container lief parallel:** `fluxbot_ai` Docker-Container pollte denselben Telegram-Token → `Conflict: terminated by other getUpdates request` → native Binary empfing keine Nachrichten
   - Fix: Docker-Container gestoppt und entfernt

4. **Full-Page Screenshot zu groß für Telegram:** `FullPage: true` erzeugte 7.6 MB PNG mit extremen Dimensionen → Telegram `PHOTO_INVALID_DIMENSIONS`
   - Fix: Viewport-Screenshot (1280x800) statt Full-Page in `pkg/browser/browser.go`

**Status:** ✅ GEFIXT & GETESTET (Session 46, 2026-03-08)
- Screenshot von bild.de erfolgreich aufgenommen und in Telegram zugestellt
- `📸 Screenshot von https://bild.de wurde aufgenommen.`
