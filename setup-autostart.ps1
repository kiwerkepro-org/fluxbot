# setup-autostart.ps1 - Konfiguriert automatisches Starten von FluxBot
# Erfordert: Administrator-Rechte
# Verwendung: powershell -ExecutionPolicy Bypass -File setup-autostart.ps1

param(
    [switch]$Remove,
    [switch]$Check
)

$BotPath = "C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT\fluxbot.exe"
$WorkingDir = "C:\Users\jjs-w\DEVELOPING\F1000-FLUXBOT"
$RegistryPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run"
$RegistryKey = "FluxBot"
$TaskName = "FluxBot-AutoStart"
$TaskDescription = "Starte FluxBot automatisch beim Logon"

function Check-Admin {
    $CurrentUser = [Security.Principal.WindowsIdentity]::GetCurrent()
    $Principal = New-Object Security.Principal.WindowsPrincipal($CurrentUser)
    return $Principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Setup-Registry {
    Write-Host "[Registry] Registriere FluxBot im Run-Eintrag..." -ForegroundColor Cyan

    $StartCommand = "`"$BotPath`""

    try {
        Set-ItemProperty -Path $RegistryPath -Name $RegistryKey -Value $StartCommand -Force
        Write-Host "[OK] Registry-Eintrag erfolgreich erstellt/aktualisiert" -ForegroundColor Green
        Write-Host "    Pfad: $RegistryPath"
        Write-Host "    Key:  $RegistryKey"
        Write-Host "    Wert: $StartCommand"
        return $true
    }
    catch {
        Write-Host "[ERROR] Fehler beim Erstellen des Registry-Eintrags: $_" -ForegroundColor Red
        return $false
    }
}

function Setup-TaskScheduler {
    Write-Host "[TaskScheduler] Erstelle Scheduled Task..." -ForegroundColor Cyan

    $ExistingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue

    if ($ExistingTask) {
        Write-Host "[!] Task '$TaskName' existiert bereits - wird gelöscht..." -ForegroundColor Yellow
        Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue
    }

    try {
        $Trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
        $Action = New-ScheduledTaskAction -Execute $BotPath -WorkingDirectory $WorkingDir
        $Settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -Compatibility Win8 -StartWhenAvailable -ExecutionTimeLimit 0

        Register-ScheduledTask -TaskName $TaskName -Trigger $Trigger -Action $Action -Settings $Settings -Description $TaskDescription -Force | Out-Null

        Write-Host "[OK] Task Scheduler-Aufgabe erfolgreich erstellt" -ForegroundColor Green
        Write-Host "    Task-Name: $TaskName"
        Write-Host "    Trigger:   AtLogOn ($env:USERNAME)"
        Write-Host "    Aktion:    $BotPath"
        return $true
    }
    catch {
        Write-Host "[ERROR] Fehler beim Erstellen der Task: $_" -ForegroundColor Red
        return $false
    }
}

function Remove-Autostart {
    Write-Host "[Entfernen] Entferne Autostart-Einträge..." -ForegroundColor Yellow

    try {
        Remove-ItemProperty -Path $RegistryPath -Name $RegistryKey -ErrorAction SilentlyContinue
        Write-Host "[OK] Registry-Eintrag gelöscht" -ForegroundColor Green
    }
    catch {
        Write-Host "[!] Registry-Eintrag nicht gefunden oder konnte nicht gelöscht werden" -ForegroundColor Yellow
    }

    try {
        $ExistingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
        if ($ExistingTask) {
            Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
            Write-Host "[OK] Task Scheduler-Aufgabe gelöscht" -ForegroundColor Green
        }
        else {
            Write-Host "[!] Task Scheduler-Aufgabe nicht gefunden" -ForegroundColor Yellow
        }
    }
    catch {
        Write-Host "[ERROR] Fehler beim Löschen der Task: $_" -ForegroundColor Red
    }
}

function Check-Autostart {
    Write-Host "[Status] Prüfe Autostart-Einträge..." -ForegroundColor Cyan
    Write-Host ""

    $RegistryValue = Get-ItemProperty -Path $RegistryPath -Name $RegistryKey -ErrorAction SilentlyContinue
    if ($RegistryValue) {
        Write-Host "[OK] Registry-Eintrag AKTIV" -ForegroundColor Green
        Write-Host "    Wert: $($RegistryValue.$RegistryKey)"
    }
    else {
        Write-Host "[FEHLT] Registry-Eintrag NICHT gesetzt" -ForegroundColor Red
    }

    Write-Host ""

    $ScheduledTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    if ($ScheduledTask) {
        $State = $ScheduledTask.State
        $IsEnabled = $ScheduledTask.Settings.Enabled
        Write-Host "[OK] Task Scheduler AKTIV" -ForegroundColor Green
        Write-Host "    Task-Name: $TaskName"
        Write-Host "    Enabled:   $IsEnabled"
        Write-Host "    State:     $State"
    }
    else {
        Write-Host "[FEHLT] Task Scheduler NICHT konfiguriert" -ForegroundColor Red
    }

    Write-Host ""

    if (Test-Path $BotPath) {
        Write-Host "[OK] FluxBot-Executable GEFUNDEN" -ForegroundColor Green
        Write-Host "    Pfad: $BotPath"
    }
    else {
        Write-Host "[FEHLT] FluxBot-Executable NICHT GEFUNDEN" -ForegroundColor Red
        Write-Host "    Erwartet: $BotPath"
    }
}

# ── Main ──────────────────────────────────────────────────────────────────────

Write-Host "=====================================================================" -ForegroundColor Cyan
Write-Host "  FluxBot AutoStart Konfiguration" -ForegroundColor Cyan
Write-Host "=====================================================================" -ForegroundColor Cyan
Write-Host ""

if ($Check) {
    Check-Autostart
    exit 0
}

if ($Remove) {
    Remove-Autostart
    exit 0
}

Write-Host "[Setup] Konfiguriere FluxBot Autostart..." -ForegroundColor Cyan
Write-Host ""

$RegistryOk = Setup-Registry
Write-Host ""

if (Check-Admin) {
    $TaskOk = Setup-TaskScheduler
    Write-Host ""
}
else {
    Write-Host "[!] Task Scheduler-Setup übersprungen (Admin-Rechte erforderlich)" -ForegroundColor Yellow
    Write-Host "    Tipp: Führe aus als Administrator" -ForegroundColor Gray
    Write-Host ""
}

Write-Host "===== AutoStart-Setup abgeschlossen =====" -ForegroundColor Green
Write-Host ""
Write-Host "Beim nächsten Neustart startet FluxBot automatisch"
Write-Host ""
Write-Host "Zum Überprüfen:    powershell -ExecutionPolicy Bypass -File setup-autostart.ps1 -Check"
Write-Host "Zum Entfernen:     powershell -ExecutionPolicy Bypass -File setup-autostart.ps1 -Remove"
