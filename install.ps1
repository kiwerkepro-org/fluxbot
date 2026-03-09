# ─────────────────────────────────────────────────────────────────────────────
#  FluxBot Installer für Windows
#  Verwendung:  irm https://fluxbot.ki-werke.de/install.ps1 | iex
#
#  Zwei Modi:
#    Nativ  – Lädt Binary direkt von GitHub Releases (kein Docker nötig)
#    Docker – Startet via docker-compose (Docker Desktop erforderlich)
# ─────────────────────────────────────────────────────────────────────────────
#Requires -Version 5.1
$ErrorActionPreference = "Stop"

# ── Konfiguration ────────────────────────────────────────────────────────────
$GH_REPO      = "kiwerkepro-org/fluxbot"
$GH_API       = "https://api.github.com/repos/$GH_REPO/releases/latest"
$COMPOSE_URL  = "https://raw.githubusercontent.com/ki-werke/fluxbot/main/docker-compose.prod.yml"
$INSTALL_DIR  = Join-Path $env:USERPROFILE "FluxBot"
$DATA_DIR     = Join-Path $INSTALL_DIR "fluxbot-data"
$BINARY_NAME  = "fluxbot.exe"

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

function Write-Step($n, $text) { Write-Host "  [$n] " -ForegroundColor Yellow -NoNewline; Write-Host $text }
function Write-OK($text)        { Write-Host "  ✔  " -ForegroundColor Green -NoNewline; Write-Host $text }
function Write-Fail($text)      { Write-Host "  ✘  " -ForegroundColor Red -NoNewline; Write-Host $text }
function Write-Info($text)      { Write-Host "     $text" -ForegroundColor DarkGray }

# ── Hauptskript ───────────────────────────────────────────────────────────────
Write-Banner

# ── Modus wählen ─────────────────────────────────────────────────────────────
Write-Host "  Installationsmodus wählen:" -ForegroundColor White
Write-Host ""
Write-Host "  [1] Nativ  – Binary direkt auf Windows (empfohlen, kein Docker nötig)" -ForegroundColor White
Write-Host "  [2] Docker – via Docker Desktop " -ForegroundColor White
Write-Host ""
$choice = Read-Host "  Auswahl [1/2]"

if ($choice -eq "2") {
    Install-Docker
} else {
    Install-Native
}

# ── Native Installation ───────────────────────────────────────────────────────
function Install-Native {
    Write-Host ""
    Write-Host "  ── Native Installation (Windows) ────────────────────────────" -ForegroundColor Cyan

    # 1. Neueste Version ermitteln
    Write-Step "1/5" "Suche neueste Version auf GitHub..."
    try {
        $release = Invoke-RestMethod -Uri $GH_API -Headers @{ "User-Agent" = "FluxBot-Installer" }
        $version = $release.tag_name
        $asset   = $release.assets | Where-Object { $_.name -eq "fluxbot-windows-amd64.exe" } | Select-Object -First 1
        if (-not $asset) {
            Write-Fail "Kein Windows-Binary im Release $version gefunden."
            Write-Info "Bitte manuell herunterladen: https://github.com/$GH_REPO/releases"
            exit 1
        }
        Write-OK "Neueste Version: $version"
    } catch {
        Write-Fail "GitHub Releases nicht erreichbar: $_"
        Write-Info "Bitte Internetverbindung prüfen oder manuell installieren."
        exit 1
    }

    # 2. Installationsverzeichnis
    Write-Step "2/5" "Richte Verzeichnisse ein..."
    New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null
    New-Item -ItemType Directory -Path $DATA_DIR    -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $DATA_DIR "skills") -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $DATA_DIR "logs")   -Force | Out-Null
    Write-OK "Verzeichnis: $INSTALL_DIR"

    # 3. Binary herunterladen
    Write-Step "3/5" "Lade FluxBot $version herunter..."
    $binaryPath = Join-Path $INSTALL_DIR $BINARY_NAME
    try {
        Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $binaryPath -UseBasicParsing
        Write-OK "Binary heruntergeladen: $binaryPath"
    } catch {
        Write-Fail "Download fehlgeschlagen: $_"
        exit 1
    }

    # 4. Playwright-Browser installieren
    Write-Step "4/5" "Installiere Browser-Komponenten (Playwright)..."
    try {
        $proc = Start-Process -FilePath $binaryPath -ArgumentList "--install-playwright" -Wait -PassThru -NoNewWindow
        if ($proc.ExitCode -eq 0) {
            Write-OK "Playwright-Browser installiert"
        } else {
            Write-Info "Browser-Installation hatte Rückgabewert $($proc.ExitCode) – FluxBot läuft trotzdem."
        }
    } catch {
        Write-Info "Browser-Installation übersprungen (optional): $_"
    }

    # 5. Autostart einrichten (Registry + Optional Task Scheduler)
    Write-Step "5/5" "Richte Autostart ein..."

    # Registry Run-Eintrag (funktioniert IMMER, auch ohne Admin-Rechte)
    $RegistryPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
    $RegistryKey = "FluxBot"
    try {
        Set-ItemProperty -Path $RegistryPath -Name $RegistryKey -Value $binaryPath -Force -ErrorAction SilentlyContinue
        Write-OK "Windows Registry Autostart eingerichtet"
    } catch {
        Write-Info "Registry-Eintrag konnte nicht gesetzt werden (nicht kritisch)"
    }

    # Versuche Task Scheduler (benötigt Admin-Rechte)
    $taskName = "FluxBot"
    $configArg = "--config `"$DATA_DIR\config.json`""

    try {
        # Bestehende Task entfernen
        $existing = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
        if ($existing) {
            Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction SilentlyContinue
        }

        # Neue Task erstellen
        $action  = New-ScheduledTaskAction  -Execute $binaryPath -Argument $configArg -WorkingDirectory $INSTALL_DIR
        $trigger = New-ScheduledTaskTrigger -AtLogOn
        $settings = New-ScheduledTaskSettingsSet `
            -ExecutionTimeLimit 0 `
            -RestartCount 3 `
            -RestartInterval (New-TimeSpan -Minutes 1) `
            -StartWhenAvailable `
            -AllowStartIfOnBatteries `
            -DontStopIfGoingOnBatteries

        Register-ScheduledTask `
            -TaskName $taskName `
            -Action $action `
            -Trigger $trigger `
            -Settings $settings `
            -RunLevel Highest `
            -Force | Out-Null

        Write-OK "Task Scheduler zusätzlich konfiguriert"
    } catch {
        Write-Info "Task Scheduler nicht verfügbar (kein Problem – Registry-Eintrag reicht aus)"
    }

    # Desktop-Verknüpfung für Dashboard
    $shell   = New-Object -ComObject WScript.Shell
    $desktop = $shell.SpecialFolders("Desktop")
    $link    = $shell.CreateShortcut((Join-Path $desktop "FluxBot Dashboard.lnk"))
    $link.TargetPath  = "http://localhost:9090"
    $link.Description = "FluxBot Dashboard öffnen"
    $link.Save()

    Show-NativeSuccess $version $INSTALL_DIR $DATA_DIR $binaryPath
}

# ── Docker Installation ───────────────────────────────────────────────────────
function Install-Docker {
    Write-Host ""
    Write-Host "  ── Docker Installation ──────────────────────────────────────" -ForegroundColor Cyan

    # 1. Docker prüfen
    Write-Step "1/4" "Prüfe Docker..."
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        Write-Fail "Docker ist nicht installiert."
        Write-Info "Bitte Docker Desktop installieren: https://docs.docker.com/desktop/install/windows-install/"
        exit 1
    }
    try {
        $null = docker info 2>&1
        if ($LASTEXITCODE -ne 0) { throw "Docker antwortet nicht" }
        Write-OK "Docker läuft"
    } catch {
        Write-Fail "Docker Desktop ist nicht gestartet. Bitte starten und erneut versuchen."
        exit 1
    }

    # 2. Verzeichnisse
    Write-Step "2/4" "Richte Installationsverzeichnis ein..."
    New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null
    New-Item -ItemType Directory -Path $DATA_DIR    -Force | Out-Null
    Write-OK "Verzeichnis: $INSTALL_DIR"

    # 3. docker-compose.yml herunterladen
    Write-Step "3/4" "Lade Konfiguration herunter..."
    $ComposePath = Join-Path $INSTALL_DIR "docker-compose.yml"
    try {
        Invoke-WebRequest -Uri $COMPOSE_URL -OutFile $ComposePath -UseBasicParsing
        Write-OK "docker-compose.yml heruntergeladen"
    } catch {
        Write-Fail "Download fehlgeschlagen: $_"
        exit 1
    }

    # 4. Starten
    Write-Step "4/4" "Starte FluxBot..."
    Set-Location $INSTALL_DIR
    docker compose pull
    docker compose up -d
    Write-OK "FluxBot (Docker) läuft!"

    Show-DockerSuccess $INSTALL_DIR $DATA_DIR $ComposePath
}

# ── Erfolgs-Ausgabe: Nativ ────────────────────────────────────────────────────
function Show-NativeSuccess($version, $installDir, $dataDir, $binaryPath) {
    Write-Host ""
    Write-Host "  ╔════════════════════════════════════════════════════════════╗" -ForegroundColor Green
    Write-Host "  ║                                                            ║" -ForegroundColor Green
    Write-Host "  ║   FluxBot $version wurde erfolgreich installiert!          ║" -ForegroundColor Green
    Write-Host "  ║                                                            ║" -ForegroundColor Green
    Write-Host "  ║   👉  http://localhost:9090   (Setup-Assistent)            ║" -ForegroundColor Green
    Write-Host "  ║                                                            ║" -ForegroundColor Green
    Write-Host "  ║   Desktop-Verknüpfung 'FluxBot Dashboard' erstellt.       ║" -ForegroundColor Green
    Write-Host "  ╚════════════════════════════════════════════════════════════╝" -ForegroundColor Green
    Write-Host ""
    Write-Info "Installationsverzeichnis: $installDir"
    Write-Info "Daten (config, Skills):   $dataDir"
    Write-Info "Binary:                   $binaryPath"
    Write-Info ""
    Write-Info "Autostart: Windows Registry + Task Scheduler (beim Logon)"
    Write-Info "Updaten:   Dashboard → Status → Update installieren"
    Write-Info "Stoppen:   Prozess beenden oder Task Scheduler deaktivieren"
    Write-Info ""
    Write-Info "Tipp: Autostart neu konfigurieren mit:"
    Write-Info "      powershell -ExecutionPolicy Bypass -File setup-autostart.ps1"
    Write-Host ""
    Start-Sleep -Seconds 3
    try { Start-Process "http://localhost:9090" } catch {}
}

# ── Erfolgs-Ausgabe: Docker ───────────────────────────────────────────────────
function Show-DockerSuccess($installDir, $dataDir, $composePath) {
    Write-Host ""
    Write-Host "  ╔════════════════════════════════════════════════════════════╗" -ForegroundColor Green
    Write-Host "  ║                                                            ║" -ForegroundColor Green
    Write-Host "  ║   FluxBot (Docker) wurde erfolgreich installiert!         ║" -ForegroundColor Green
    Write-Host "  ║                                                            ║" -ForegroundColor Green
    Write-Host "  ║   👉  http://localhost:8090   (Setup-Assistent)            ║" -ForegroundColor Green
    Write-Host "  ║                                                            ║" -ForegroundColor Green
    Write-Host "  ╚════════════════════════════════════════════════════════════╝" -ForegroundColor Green
    Write-Host ""
    Write-Info "Installationsverzeichnis: $installDir"
    Write-Info "Daten (config, Skills):   $dataDir"
    Write-Info ""
    Write-Info "Stoppen:  docker compose -f `"$composePath`" down"
    Write-Info "Updaten:  docker compose -f `"$composePath`" pull && docker compose -f `"$composePath`" up -d"
    Write-Host ""
    Start-Sleep -Seconds 3
    try { Start-Process "http://localhost:8090" } catch {}
}
