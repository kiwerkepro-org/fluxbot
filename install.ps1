# ─────────────────────────────────────────────────────────────────────────────
#  FluxBot Installer für Windows (Docker-Variante)
#  Verwendung:  irm https://fluxbot.ki-werke.de/install.ps1 | iex
# ─────────────────────────────────────────────────────────────────────────────
#Requires -Version 5.1
$ErrorActionPreference = "Stop"

# ── Farben & Banner ──────────────────────────────────────────────────────────
function Write-Banner {
    Write-Host ""
    Write-Host "  ███████╗██╗     ██╗   ██╗██╗  ██╗██████╗  ██████╗ ████████╗" -ForegroundColor Cyan
    Write-Host "  ██╔════╝██║     ██║   ██║╚██╗██╔╝██╔══██╗██╔═══██╗╚══██╔══╝" -ForegroundColor Cyan
    Write-Host "  █████╗  ██║     ██║   ██║ ╚███╔╝ ██████╔╝██║   ██║   ██║   " -ForegroundColor Cyan
    Write-Host "  ██╔══╝  ██║     ██║   ██║ ██╔██╗ ██╔══██╗██║   ██║   ██║   " -ForegroundColor Cyan
    Write-Host "  ██║     ███████╗╚██████╔╝██╔╝ ██╗██████╔╝╚██████╔╝   ██║   " -ForegroundColor Cyan
    Write-Host "  ╚═╝     ╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚═════╝  ╚═════╝   ╚═╝   " -ForegroundColor Cyan
    Write-Host ""
    Write-Host "  Multi-Channel AI Agent  ·  ki-werke.de" -ForegroundColor DarkCyan
    Write-Host ""
}

function Write-Step($n, $text) {
    Write-Host "  [$n] " -ForegroundColor Yellow -NoNewline
    Write-Host $text
}

function Write-OK($text) {
    Write-Host "  ✔  " -ForegroundColor Green -NoNewline
    Write-Host $text
}

function Write-Fail($text) {
    Write-Host "  ✘  " -ForegroundColor Red -NoNewline
    Write-Host $text
}

function Write-Info($text) {
    Write-Host "     " -NoNewline
    Write-Host $text -ForegroundColor DarkGray
}

# ── Hauptskript ───────────────────────────────────────────────────────────────
Write-Banner

# ── 1. Docker prüfen ─────────────────────────────────────────────────────────
Write-Step "1/4" "Prüfe Docker..."

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Fail "Docker ist nicht installiert."
    Write-Info ""
    Write-Info "Bitte Docker Desktop für Windows installieren:"
    Write-Info "https://docs.docker.com/desktop/install/windows-install/"
    Write-Info ""
    Write-Info "Nach der Installation diesen Befehl erneut ausführen."
    exit 1
}

try {
    $null = docker info 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Docker antwortet nicht" }
    Write-OK "Docker läuft"
} catch {
    Write-Fail "Docker Desktop ist nicht gestartet."
    Write-Info ""
    Write-Info "Bitte Docker Desktop starten und dann diesen Befehl erneut ausführen."
    exit 1
}

# ── 2. Installationsverzeichnis ───────────────────────────────────────────────
Write-Step "2/4" "Richte Installationsverzeichnis ein..."

$InstallDir = Join-Path $env:USERPROFILE "FluxBot"

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir | Out-Null
    Write-OK "Verzeichnis erstellt: $InstallDir"
} else {
    Write-OK "Verzeichnis vorhanden: $InstallDir"
}

# Datenverzeichnis für persistente Daten
$DataDir = Join-Path $InstallDir "fluxbot-data"
if (-not (Test-Path $DataDir)) {
    New-Item -ItemType Directory -Path $DataDir | Out-Null
}

# ── 3. docker-compose.yml herunterladen ───────────────────────────────────────
Write-Step "3/4" "Lade Konfiguration herunter..."

$ComposeUrl  = "https://raw.githubusercontent.com/ki-werke/fluxbot/main/docker-compose.prod.yml"
$ComposePath = Join-Path $InstallDir "docker-compose.yml"

try {
    Invoke-WebRequest -Uri $ComposeUrl -OutFile $ComposePath -UseBasicParsing
    Write-OK "docker-compose.yml heruntergeladen"
} catch {
    Write-Fail "Download fehlgeschlagen: $_"
    Write-Info "Überprüfe deine Internetverbindung und versuche es erneut."
    exit 1
}

# ── 4. FluxBot starten ────────────────────────────────────────────────────────
Write-Step "4/4" "Starte FluxBot..."

Set-Location $InstallDir

try {
    docker compose pull
    docker compose up -d
} catch {
    Write-Fail "Fehler beim Starten: $_"
    exit 1
}

Write-OK "FluxBot läuft!"

# ── Fertig ────────────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  ╔══════════════════════════════════════════════════════╗" -ForegroundColor Green
Write-Host "  ║                                                      ║" -ForegroundColor Green
Write-Host "  ║   FluxBot wurde erfolgreich installiert!             ║" -ForegroundColor Green
Write-Host "  ║                                                      ║" -ForegroundColor Green
Write-Host "  ║   👉  http://localhost:8090                          ║" -ForegroundColor Green
Write-Host "  ║                                                      ║" -ForegroundColor Green
Write-Host "  ║   Der Einrichtungsassistent öffnet sich gleich.      ║" -ForegroundColor Green
Write-Host "  ║   Folge den Schritten um FluxBot zu konfigurieren.   ║" -ForegroundColor Green
Write-Host "  ║                                                      ║" -ForegroundColor Green
Write-Host "  ╚══════════════════════════════════════════════════════╝" -ForegroundColor Green
Write-Host ""
Write-Info "Installationsverzeichnis: $InstallDir"
Write-Info "Daten (config, Skills):   $DataDir"
Write-Info ""
Write-Info "FluxBot stoppen:   docker compose -f `"$ComposePath`" down"
Write-Info "FluxBot updaten:   docker compose -f `"$ComposePath`" pull && docker compose -f `"$ComposePath`" up -d"
Write-Host ""

# Browser öffnen (kurz warten damit der Container hochfährt)
Start-Sleep -Seconds 3
try {
    Start-Process "http://localhost:8090"
} catch {
    Write-Info "Browser konnte nicht automatisch geöffnet werden."
    Write-Info "Bitte manuell öffnen: http://localhost:8090"
}
