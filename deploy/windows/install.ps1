# FluxBot – Windows Service Installer
# Installiert FluxBot als Windows-Dienst (automatischer Start mit Windows)
#
# Verwendung (als Administrator in PowerShell):
#   Set-ExecutionPolicy Bypass -Scope Process -Force
#   .\deploy\windows\install.ps1 -BinaryPath ".\fluxbot.exe" -ConfigPath ".\workspace\config.json"
#
# Deinstallation:
#   .\deploy\windows\install.ps1 -Uninstall

param(
    [string]$BinaryPath  = ".\fluxbot.exe",
    [string]$ConfigPath  = ".\workspace\config.json",
    [switch]$Uninstall   = $false
)

$ServiceName = "FluxBot"
$DisplayName = "FluxBot – Multi-Channel AI Agent"
$Description = "FluxBot KI-Assistent von KI-WERKE (github.com/ki-werke/fluxbot)"

# ── Farben und Hilfsfunktionen ─────────────────────────────────────────────────
function Write-Info    { param($msg) Write-Host "[FluxBot] $msg" -ForegroundColor Cyan }
function Write-Success { param($msg) Write-Host "[OK] $msg" -ForegroundColor Green }
function Write-Warn    { param($msg) Write-Host "[Warnung] $msg" -ForegroundColor Yellow }
function Write-Err     { param($msg) Write-Host "[Fehler] $msg" -ForegroundColor Red; exit 1 }

Write-Host ""
Write-Host "╔══════════════════════════════════════════════════╗" -ForegroundColor Blue
Write-Host "║  FluxBot – Windows Service Installer             ║" -ForegroundColor Blue
Write-Host "║  KI-WERKE | github.com/ki-werke/fluxbot          ║" -ForegroundColor Blue
Write-Host "╚══════════════════════════════════════════════════╝" -ForegroundColor Blue
Write-Host ""

# ── Administrator-Check ────────────────────────────────────────────────────────
$currentPrincipal = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Err "Dieses Script muss als Administrator ausgeführt werden.`nRechtsklick auf PowerShell → 'Als Administrator ausführen'"
}

# ══════════════════════════════════════════════════════════════════════════════
# DEINSTALLATION
# ══════════════════════════════════════════════════════════════════════════════
if ($Uninstall) {
    Write-Info "Deinstalliere FluxBot Service..."

    $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if (-not $svc) {
        Write-Warn "Dienst '$ServiceName' nicht gefunden – nichts zu tun."
        exit 0
    }

    # Stoppen falls aktiv
    if ($svc.Status -eq "Running") {
        Write-Info "Stoppe Dienst..."
        Stop-Service -Name $ServiceName -Force
        Start-Sleep -Seconds 2
    }

    # Entfernen (fluxbot.exe --service uninstall macht das sauber)
    $exePath = (Get-WmiObject Win32_Service -Filter "Name='$ServiceName'").PathName
    if ($exePath) {
        # Pfad aus dem Service-Eintrag extrahieren (ohne Argumente)
        $exe = ($exePath -split ' --')[0].Trim('"')
        if (Test-Path $exe) {
            Write-Info "Führe Service-Deinstallation aus..."
            Start-Process -FilePath $exe -ArgumentList "--service", "uninstall" -Verb RunAs -Wait
        }
    } else {
        # Fallback: sc.exe
        sc.exe delete $ServiceName | Out-Null
    }

    Write-Success "FluxBot Service deinstalliert."
    exit 0
}

# ══════════════════════════════════════════════════════════════════════════════
# INSTALLATION
# ══════════════════════════════════════════════════════════════════════════════

# ── Vorabprüfungen ─────────────────────────────────────────────────────────────
Write-Info "Prüfe Voraussetzungen..."

$BinaryAbsPath = (Resolve-Path $BinaryPath -ErrorAction SilentlyContinue)?.Path
if (-not $BinaryAbsPath -or -not (Test-Path $BinaryAbsPath)) {
    Write-Err "Binary nicht gefunden: $BinaryPath`n  Erst kompilieren: GOOS=windows GOARCH=amd64 go build -o fluxbot.exe .\cmd\fluxbot"
}

$ConfigAbsPath = (Resolve-Path $ConfigPath -ErrorAction SilentlyContinue)?.Path
if (-not $ConfigAbsPath -or -not (Test-Path $ConfigAbsPath)) {
    # config.example.json als Fallback
    $exampleConfig = (Resolve-Path "workspace\config.example.json" -ErrorAction SilentlyContinue)?.Path
    if ($exampleConfig) {
        Write-Warn "config.json nicht gefunden – kopiere config.example.json"
        Copy-Item $exampleConfig -Destination $ConfigPath
        $ConfigAbsPath = (Resolve-Path $ConfigPath).Path
        Write-Warn "Bitte API-Keys in config.json eintragen: $ConfigAbsPath"
    } else {
        Write-Err "Keine config.json und keine config.example.json gefunden."
    }
}

# Bereits installiert?
$existingSvc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existingSvc) {
    Write-Warn "Dienst '$ServiceName' ist bereits installiert (Status: $($existingSvc.Status))"
    Write-Warn "Zum Neuinstallieren: .\deploy\windows\install.ps1 -Uninstall"
    exit 1
}

Write-Success "Voraussetzungen OK"
Write-Info "Binary:  $BinaryAbsPath"
Write-Info "Config:  $ConfigAbsPath"

# ── Service über fluxbot.exe --service install registrieren ───────────────────
Write-Info "Registriere Windows-Dienst '$ServiceName'..."

$result = Start-Process -FilePath $BinaryAbsPath `
    -ArgumentList "--service", "install", "--config", "`"$ConfigAbsPath`"" `
    -Verb RunAs -Wait -PassThru

if ($result.ExitCode -ne 0) {
    Write-Err "Service-Installation fehlgeschlagen (Exit-Code: $($result.ExitCode))"
}

Write-Success "Dienst '$ServiceName' registriert."

# ── Dienst starten ────────────────────────────────────────────────────────────
Write-Info "Starte Dienst..."
try {
    Start-Service -Name $ServiceName
    Start-Sleep -Seconds 2
    $svc = Get-Service -Name $ServiceName
    if ($svc.Status -eq "Running") {
        Write-Success "FluxBot läuft als Windows-Dienst!"
    } else {
        Write-Warn "Dienst gestartet, Status: $($svc.Status)"
        Write-Warn "Logs: Ereignisanzeige → Windows-Protokolle → Anwendung → Quelle: FluxBot"
    }
} catch {
    Write-Warn "Dienst konnte nicht gestartet werden: $_"
    Write-Warn "Starte manuell: Start-Service -Name $ServiceName"
}

# ── Zusammenfassung ────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "╔══════════════════════════════════════════════════╗" -ForegroundColor Green
Write-Host "║  Installation abgeschlossen                      ║" -ForegroundColor Green
Write-Host "╚══════════════════════════════════════════════════╝" -ForegroundColor Green
Write-Host ""
Write-Host "  Binary:   $BinaryAbsPath"
Write-Host "  Config:   $ConfigAbsPath"
Write-Host ""
Write-Host "  Nützliche Befehle:"
Write-Host "    Get-Service FluxBot                     # Status prüfen"
Write-Host "    Start-Service FluxBot                   # Starten"
Write-Host "    Stop-Service FluxBot                    # Stoppen"
Write-Host "    Restart-Service FluxBot                 # Neustart"
Write-Host "    .\deploy\windows\install.ps1 -Uninstall # Deinstallieren"
Write-Host ""
Write-Host "  Logs: Ereignisanzeige → Windows-Protokolle → Anwendung → Quelle: FluxBot"
Write-Host ""
