package security

import (
	"strings"
	"sync"
	"sync/atomic"
)

// DangerousCategory identifiziert eine Klasse potenziell gefährlicher Operationen.
type DangerousCategory = string

const (
	CategorySystemRun           DangerousCategory = "system.run"
	CategoryFileDelete          DangerousCategory = "file.delete"
	CategoryFileModify          DangerousCategory = "file.modify"
	CategoryCodeEval            DangerousCategory = "code.eval"
	CategoryNetworkUnrestricted DangerousCategory = "network.unrestricted"
)

// AllDangerousCategories ist die Standard-Sperrliste (alle Kategorien).
var AllDangerousCategories = []string{
	CategorySystemRun,
	CategoryFileDelete,
	CategoryFileModify,
	CategoryCodeEval,
	CategoryNetworkUnrestricted,
}

// DangerousToolsResult ist das Ergebnis einer Dangerous-Tools-Prüfung.
type DangerousToolsResult struct {
	Blocked  bool
	Category string // erkannte Kategorie
	Reason   string // lesbarer Grund
	Pattern  string // gematchtes Muster
}

// dangerousPattern verknüpft ein Schlüsselwort-Muster mit einer Kategorie.
type dangerousPattern struct {
	pattern  string
	category DangerousCategory
	reason   string
}

// dangerousPatterns enthält alle Erkennungsmuster (lowercase, für strings.Contains).
var dangerousPatterns = []dangerousPattern{
	// ── system.run ─────────────────────────────────────────────────────────
	{"rm -rf", CategorySystemRun, "Gefährlicher Löschbefehl (rm -rf)"},
	{"del /f /s", CategorySystemRun, "Gefährlicher Löschbefehl (del /f /s)"},
	{"format c:", CategorySystemRun, "Festplatten-Formatierung"},
	{"format my", CategorySystemRun, "System-Formatierung (EN)"},
	{"formatiere meine festplatte", CategorySystemRun, "Festplatten-Formatierung"},
	{"formatiere mein system", CategorySystemRun, "System-Formatierung"},
	{"os.system(", CategorySystemRun, "System-Aufruf (os.system)"},
	{"os.popen(", CategorySystemRun, "System-Aufruf (os.popen)"},
	{"subprocess.run(", CategorySystemRun, "Prozess-Start (subprocess.run)"},
	{"subprocess.call(", CategorySystemRun, "Prozess-Start (subprocess.call)"},
	{"subprocess.popen(", CategorySystemRun, "Prozess-Start (subprocess.Popen)"},
	{"shell_exec(", CategorySystemRun, "Shell-Exec (PHP)"},
	{"proc_open(", CategorySystemRun, "Prozess-Öffnen (PHP proc_open)"},
	{"runtime.exec(", CategorySystemRun, "Runtime-Exec (Java)"},
	{"cmd.exe", CategorySystemRun, "Shell-Befehl (cmd.exe)"},
	{"/bin/sh -c", CategorySystemRun, "Shell-Aufruf (/bin/sh -c)"},
	{"/bin/bash -c", CategorySystemRun, "Shell-Aufruf (/bin/bash -c)"},
	{"shutdown /s", CategorySystemRun, "System-Shutdown (Windows)"},
	{"shutdown -h now", CategorySystemRun, "System-Shutdown (Linux)"},
	{"taskkill /f", CategorySystemRun, "Prozess-Kill (taskkill)"},
	{"killall -9", CategorySystemRun, "Prozess-Kill (killall)"},
	{"führe terminal-befehl aus", CategorySystemRun, "Terminal-Befehl ausführen"},
	{"führe folgenden befehl aus", CategorySystemRun, "Befehl ausführen"},
	{"execute this command", CategorySystemRun, "Befehl ausführen (EN)"},
	{"run this command", CategorySystemRun, "Befehl ausführen (EN)"},
	{"run the following command", CategorySystemRun, "Befehl ausführen (EN)"},
	{"powershell -command", CategorySystemRun, "PowerShell-Befehl"},
	{"powershell -enc", CategorySystemRun, "PowerShell encoded command"},
	{"invoke-expression", CategorySystemRun, "PowerShell Invoke-Expression"},
	{"iex(", CategorySystemRun, "PowerShell IEX"},

	// ── file.delete ────────────────────────────────────────────────────────
	{"os.remove(", CategoryFileDelete, "Datei löschen (os.remove)"},
	{"os.unlink(", CategoryFileDelete, "Datei löschen (os.unlink)"},
	{"shutil.rmtree(", CategoryFileDelete, "Verzeichnis löschen (shutil.rmtree)"},
	{"pathlib.path.unlink(", CategoryFileDelete, "Datei löschen (pathlib)"},
	{"file.delete(", CategoryFileDelete, "Datei löschen (file.delete)"},
	{"fs.unlinkSync(", CategoryFileDelete, "Datei löschen (Node.js)"},
	{"fs.rmdirSync(", CategoryFileDelete, "Verzeichnis löschen (Node.js)"},
	{"lösche die datei", CategoryFileDelete, "Datei löschen"},
	{"lösche den ordner", CategoryFileDelete, "Ordner löschen"},
	{"lösche alle dateien", CategoryFileDelete, "Alle Dateien löschen"},
	{"delete the file", CategoryFileDelete, "Datei löschen (EN)"},
	{"delete all files", CategoryFileDelete, "Alle Dateien löschen (EN)"},
	{"remove all files", CategoryFileDelete, "Alle Dateien löschen (EN)"},
	{"remove the directory", CategoryFileDelete, "Verzeichnis löschen (EN)"},

	// ── file.modify ────────────────────────────────────────────────────────
	{"sed -i", CategoryFileModify, "Datei modifizieren (sed -i)"},
	{"awk -i inplace", CategoryFileModify, "Datei modifizieren (awk)"},
	{"open(path, 'w')", CategoryFileModify, "Datei überschreiben (Python open w)"},
	{"open(path, \"w\")", CategoryFileModify, "Datei überschreiben (Python open w)"},
	{"with open(", CategoryFileModify, "Datei-Schreibzugriff (Python with open)"},
	{"os.write(", CategoryFileModify, "Datei schreiben (os.write)"},
	{"fs.writefilesync(", CategoryFileModify, "Datei schreiben (Node.js writeFileSync)"},
	{"überschreibe die datei", CategoryFileModify, "Datei überschreiben"},
	{"überschreibe den inhalt", CategoryFileModify, "Dateiinhalt überschreiben"},
	{"schreibe in die datei", CategoryFileModify, "In Datei schreiben"},
	{"write to the file", CategoryFileModify, "Datei schreiben (EN)"},
	{"overwrite the file", CategoryFileModify, "Datei überschreiben (EN)"},
	{"modifiziere die konfigurationsdatei", CategoryFileModify, "Konfigurationsdatei ändern"},
	{"modify the config file", CategoryFileModify, "Config-Datei ändern (EN)"},
	{"edit the hosts file", CategoryFileModify, "hosts-Datei ändern (EN)"},
	{"bearbeite die hosts-datei", CategoryFileModify, "hosts-Datei ändern"},

	// ── code.eval ──────────────────────────────────────────────────────────
	{"eval(", CategoryCodeEval, "Code-Eval (eval)"},
	{"exec(", CategoryCodeEval, "Code-Ausführung (exec)"},
	{"new function(", CategoryCodeEval, "Function-Konstruktor (JS)"},
	{"new function (", CategoryCodeEval, "Function-Konstruktor (JS)"},
	{"__import__(", CategoryCodeEval, "Dynamischer Import (Python)"},
	{"compile(source", CategoryCodeEval, "Code-Kompilierung (Python compile)"},
	{"execfile(", CategoryCodeEval, "Datei-Ausführung (Python execfile)"},
	{"führe den code aus", CategoryCodeEval, "Code ausführen"},
	{"führe diesen code aus", CategoryCodeEval, "Code ausführen"},
	{"führe dieses skript aus", CategoryCodeEval, "Skript ausführen"},
	{"run this code", CategoryCodeEval, "Code ausführen (EN)"},
	{"execute this script", CategoryCodeEval, "Skript ausführen (EN)"},
	{"execute this code", CategoryCodeEval, "Code ausführen (EN)"},
	{"führe das python-skript aus", CategoryCodeEval, "Python-Skript ausführen"},
	{"run the python script", CategoryCodeEval, "Python-Skript ausführen (EN)"},

	// ── network.unrestricted ───────────────────────────────────────────────
	{"requests.get(", CategoryNetworkUnrestricted, "HTTP-GET (Python requests)"},
	{"requests.post(", CategoryNetworkUnrestricted, "HTTP-POST (Python requests)"},
	{"requests.put(", CategoryNetworkUnrestricted, "HTTP-PUT (Python requests)"},
	{"urllib.request.urlopen(", CategoryNetworkUnrestricted, "HTTP-Request (urllib)"},
	{"httpx.get(", CategoryNetworkUnrestricted, "HTTP-GET (httpx)"},
	{"fetch('http", CategoryNetworkUnrestricted, "Fetch-Request (JS http)"},
	{"fetch(\"http", CategoryNetworkUnrestricted, "Fetch-Request (JS http)"},
	{"xmlhttprequest", CategoryNetworkUnrestricted, "XMLHttpRequest (JS)"},
	{"curl http://", CategoryNetworkUnrestricted, "cURL HTTP-Request"},
	{"curl https://", CategoryNetworkUnrestricted, "cURL HTTPS-Request"},
	{"wget http://", CategoryNetworkUnrestricted, "wget HTTP-Request"},
	{"wget https://", CategoryNetworkUnrestricted, "wget HTTPS-Request"},
	{"sende daten an einen server", CategoryNetworkUnrestricted, "Daten-Exfiltration"},
	{"send data to a server", CategoryNetworkUnrestricted, "Daten-Exfiltration (EN)"},
	{"exfiltriere", CategoryNetworkUnrestricted, "Daten-Exfiltration"},
	{"exfiltrate", CategoryNetworkUnrestricted, "Daten-Exfiltration (EN)"},
}

// ── Stats ──────────────────────────────────────────────────────────────────

// dangerousToolsStats zählt blockierte Anfragen pro Kategorie (thread-safe, in-memory).
type dangerousToolsStats struct {
	mu     sync.RWMutex
	counts map[string]int64
	total  int64
}

var globalDangerousStats = &dangerousToolsStats{
	counts: map[string]int64{
		CategorySystemRun:           0,
		CategoryFileDelete:          0,
		CategoryFileModify:          0,
		CategoryCodeEval:            0,
		CategoryNetworkUnrestricted: 0,
	},
}

func (s *dangerousToolsStats) record(category string) {
	s.mu.Lock()
	s.counts[category]++
	s.mu.Unlock()
	atomic.AddInt64(&s.total, 1)
}

// GetDangerousToolsStats gibt die blockierten Anfragen pro Kategorie zurück.
func GetDangerousToolsStats() map[string]int64 {
	globalDangerousStats.mu.RLock()
	defer globalDangerousStats.mu.RUnlock()
	result := make(map[string]int64, len(globalDangerousStats.counts))
	for k, v := range globalDangerousStats.counts {
		result[k] = v
	}
	return result
}

// GetDangerousToolsTotal gibt die Gesamtzahl blockierter Anfragen zurück.
func GetDangerousToolsTotal() int64 {
	return atomic.LoadInt64(&globalDangerousStats.total)
}

// ── Check ──────────────────────────────────────────────────────────────────

// CheckDangerousTools prüft einen User-Prompt auf gefährliche Operationen.
// blockedCategories: Liste der gesperrten Kategorien (leer = nichts sperren).
func CheckDangerousTools(prompt string, blockedCategories []string) DangerousToolsResult {
	if len(blockedCategories) == 0 {
		return DangerousToolsResult{Blocked: false}
	}

	blocked := make(map[string]bool, len(blockedCategories))
	for _, c := range blockedCategories {
		blocked[strings.ToLower(c)] = true
	}

	lower := strings.ToLower(prompt)
	for _, p := range dangerousPatterns {
		if !blocked[p.category] {
			continue
		}
		if strings.Contains(lower, p.pattern) {
			globalDangerousStats.record(p.category)
			return DangerousToolsResult{
				Blocked:  true,
				Category: p.category,
				Reason:   p.reason,
				Pattern:  p.pattern,
			}
		}
	}

	return DangerousToolsResult{Blocked: false}
}

// IsDangerousToolsAdmin prüft ob ein Sender in der Admin-Liste ist und den Check überspringen darf.
// Format der adminIDs: "123456" (nur SenderID) oder "telegram:123456" (Channel + SenderID).
func IsDangerousToolsAdmin(senderID, channelID string, adminIDs []string) bool {
	qualified := channelID + ":" + senderID
	for _, admin := range adminIDs {
		if admin == senderID || admin == qualified {
			return true
		}
	}
	return false
}

// DangerousCategoryLabel gibt einen lesbaren deutschen Namen für eine Kategorie zurück.
func DangerousCategoryLabel(cat string) string {
	switch cat {
	case CategorySystemRun:
		return "System-Befehle"
	case CategoryFileDelete:
		return "Dateien löschen"
	case CategoryFileModify:
		return "Dateien ändern"
	case CategoryCodeEval:
		return "Code-Ausführung"
	case CategoryNetworkUnrestricted:
		return "Netzwerk-Zugriff"
	default:
		return cat
	}
}
