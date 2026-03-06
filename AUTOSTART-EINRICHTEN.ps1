# =============================================================================
# FluxBot AutoStart Einrichten
# Einmalig als Administrator ausfuehren!
# =============================================================================
# Was dieses Skript macht:
#   1. Alten Windows Service entfernen (war fehlerhaft)
#   2. Task Scheduler Eintrag erstellen (startet beim Login, kein Fenster)
#   3. FluxBot sofort starten (unsichtbar im Hintergrund)
#   4. Desktop-Verknuepfung erstellen (oeffnet nur den Browser)
# =============================================================================

param(
    [switch]$Deinstallieren
)

$WorkDir   = Split-Path -Parent $MyInvocation.MyCommand.Path
$ExePath   = Join-Path $WorkDir "fluxbot.exe"
$TaskName  = "FluxBot"
$Dashboard = "http://localhost:9090"

# Admin-Check
$IsAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]"Administrator")
if (-not $IsAdmin) {
    Write-Host ""
    Write-Host "FEHLER: Bitte als Administrator ausfuehren!" -ForegroundColor Red
    Write-Host ""
    Write-Host "Rechtsklick auf PowerShell -> 'Als Administrator ausfuehren'" -ForegroundColor Yellow
    Write-Host "Dann nochmal: powershell -ExecutionPolicy Bypass -File .\AUTOSTART-EINRICHTEN.ps1" -ForegroundColor Yellow
    pause
    exit 1
}

if (-not (Test-Path $ExePath)) {
    Write-Host ""
    Write-Host "FEHLER: fluxbot.exe nicht gefunden!" -ForegroundColor Red
    Write-Host "Erwartet in: $ExePath" -ForegroundColor Yellow
    pause
    exit 1
}

# =============================================================================
# DEINSTALLIEREN
# =============================================================================
if ($Deinstallieren) {
    Write-Host ""
    Write-Host "FluxBot AutoStart wird entfernt..." -ForegroundColor Yellow

    # Task entfernen
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
    Write-Host "  Task entfernt." -ForegroundColor Green

    # Alten Service entfernen (falls noch vorhanden)
    $svc = Get-Service -Name $TaskName -ErrorAction SilentlyContinue
    if ($svc) {
        Stop-Service -Name $TaskName -Force -ErrorAction SilentlyContinue
        sc.exe delete $TaskName | Out-Null
        Write-Host "  Service entfernt." -ForegroundColor Green
    }

    # FluxBot-Prozess stoppen
    Get-Process -Name "fluxbot" -ErrorAction SilentlyContinue | Stop-Process -Force
    Write-Host "  Prozess gestoppt." -ForegroundColor Green

    Write-Host ""
    Write-Host "FluxBot AutoStart erfolgreich entfernt." -ForegroundColor Green
    pause
    exit 0
}

# =============================================================================
# SCHRITT 1: Alten Windows Service entfernen (war fehlerhaft – kein WorkDir)
# =============================================================================
Write-Host ""
Write-Host "=============================================" -ForegroundColor Cyan
Write-Host "  FluxBot AutoStart einrichten" -ForegroundColor Cyan
Write-Host "=============================================" -ForegroundColor Cyan

$svc = Get-Service -Name $TaskName -ErrorAction SilentlyContinue
if ($svc) {
    Write-Host ""
    Write-Host "[1/4] Alter Service wird entfernt..." -ForegroundColor Yellow
    Stop-Service -Name $TaskName -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
    sc.exe delete $TaskName | Out-Null
    Start-Sleep -Seconds 1
    Write-Host "      Alter Service entfernt." -ForegroundColor Green
} else {
    Write-Host "[1/4] Kein alter Service gefunden." -ForegroundColor Gray
}

# =============================================================================
# SCHRITT 2: Laufenden FluxBot stoppen (sauberer Neustart)
# =============================================================================
$proc = Get-Process -Name "fluxbot" -ErrorAction SilentlyContinue
if ($proc) {
    Write-Host "[2/4] Laufenden FluxBot stoppen..." -ForegroundColor Yellow
    $proc | Stop-Process -Force
    Start-Sleep -Seconds 2
    Write-Host "      FluxBot gestoppt." -ForegroundColor Green
} else {
    Write-Host "[2/4] FluxBot laeuft nicht (OK)." -ForegroundColor Gray
}

# =============================================================================
# SCHRITT 3: Task Scheduler Eintrag erstellen
#   - Startet bei jedem Windows-Login automatisch
#   - Kein Fenster sichtbar (WindowStyle Hidden)
#   - WorkDir korrekt gesetzt (wichtig fuer config.json)
#   - Neustart automatisch bei Absturz (3x, nach 1 Minute)
# =============================================================================
Write-Host "[3/4] AutoStart im Task Scheduler einrichten..." -ForegroundColor Yellow

# Alten Task entfernen falls vorhanden
Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue

# PowerShell-Wrapper: startet FluxBot ohne sichtbares Fenster
$StartCmd = "Start-Process -FilePath '$ExePath' -WorkingDirectory '$WorkDir' -WindowStyle Hidden"

$Action = New-ScheduledTaskAction `
    -Execute "powershell.exe" `
    -Argument "-WindowStyle Hidden -NonInteractive -ExecutionPolicy Bypass -Command `"$StartCmd`"" `
    -WorkingDirectory $WorkDir

# Trigger: beim Login des aktuellen Users
$Trigger = New-ScheduledTaskTrigger -AtLogon -User $env:USERNAME

# Einstellungen: kein Zeitlimit, bei Absturz automatisch neustarten
$Settings = New-ScheduledTaskSettingsSet `
    -ExecutionTimeLimit (New-TimeSpan -Hours 0) `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -StartWhenAvailable `
    -RunOnlyIfNetworkAvailable:$false

# Mit hoechsten Rechten starten (damit Dashboard auf Port 9090 funktioniert)
$Principal = New-ScheduledTaskPrincipal `
    -UserId $env:USERNAME `
    -LogonType Interactive `
    -RunLevel Highest

Register-ScheduledTask `
    -TaskName $TaskName `
    -Action $Action `
    -Trigger $Trigger `
    -Settings $Settings `
    -Principal $Principal `
    -Force | Out-Null

Write-Host "      AutoStart eingerichtet." -ForegroundColor Green

# =============================================================================
# SCHRITT 4: Desktop-Verknuepfung erstellen
#   Die "Echse" oeffnet ab jetzt nur noch den Browser (Dashboard)
#   FluxBot laeuft sowieso schon im Hintergrund
# =============================================================================
Write-Host "[4/4] Desktop-Verknuepfung wird erstellt..." -ForegroundColor Yellow

$DesktopPath = [Environment]::GetFolderPath("Desktop")
$ShortcutPath = Join-Path $DesktopPath "FluxBot Dashboard.lnk"

$WshShell = New-Object -ComObject WScript.Shell
$Shortcut = $WshShell.CreateShortcut($ShortcutPath)
$Shortcut.TargetPath = "cmd.exe"
$Shortcut.Arguments = "/c start $Dashboard"
$Shortcut.WindowStyle = 7   # 7 = minimiert (cmd blitzt kaum auf)
$Shortcut.Description = "FluxBot Dashboard oeffnen"

# Icon: Browser-Icon oder eigenes Icon falls vorhanden
$IcoPath = Join-Path $WorkDir "fluxbot.ico"
if (Test-Path $IcoPath) {
    $Shortcut.IconLocation = $IcoPath
} else {
    # Standard-Browser-Icon
    $Shortcut.IconLocation = "shell32.dll,14"
}

$Shortcut.Save()
Write-Host "      Desktop-Verknuepfung erstellt: 'FluxBot Dashboard'" -ForegroundColor Green

# =============================================================================
# FERTIG: FluxBot jetzt sofort starten
# =============================================================================
Write-Host ""
Write-Host "FluxBot wird gestartet..." -ForegroundColor Cyan
Start-Process -FilePath $ExePath -WorkingDirectory $WorkDir -WindowStyle Hidden
Start-Sleep -Seconds 3

# Pruefen ob FluxBot laeuft
$proc = Get-Process -Name "fluxbot" -ErrorAction SilentlyContinue
if ($proc) {
    Write-Host ""
    Write-Host "=============================================" -ForegroundColor Green
    Write-Host "  FERTIG! FluxBot laeuft!" -ForegroundColor Green
    Write-Host "=============================================" -ForegroundColor Green
    Write-Host ""
    Write-Host "  Dashboard:   $Dashboard" -ForegroundColor Cyan
    Write-Host "  Prozess-ID:  $($proc.Id)" -ForegroundColor Gray
    Write-Host ""
    Write-Host "  Ab jetzt startet FluxBot automatisch" -ForegroundColor Green
    Write-Host "  bei jedem Windows-Login." -ForegroundColor Green
    Write-Host ""
    Write-Host "  Die Verknuepfung 'FluxBot Dashboard'" -ForegroundColor Green
    Write-Host "  auf dem Desktop oeffnet direkt das" -ForegroundColor Green
    Write-Host "  Dashboard im Browser." -ForegroundColor Green
    Write-Host ""

    # Browser mit Dashboard oeffnen
    Start-Process $Dashboard

} else {
    Write-Host ""
    Write-Host "WARNUNG: FluxBot konnte nicht gestartet werden." -ForegroundColor Red
    Write-Host "Bitte Logs pruefen:" -ForegroundColor Yellow
    Write-Host "  $WorkDir\workspace\logs\fluxbot.log" -ForegroundColor Gray
}

Write-Host "=============================================" -ForegroundColor Cyan
Write-Host ""
pause
