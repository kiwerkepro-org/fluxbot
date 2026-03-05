# FluxBot Starter Script für Windows
# Einfach dieses Skript mit: powershell.exe -ExecutionPolicy Bypass -File START-FluxBot.ps1

Write-Host "=" * 60
Write-Host "FluxBot Starter" -ForegroundColor Green
Write-Host "=" * 60

$exePath = Split-Path -Parent $MyInvocation.MyCommand.Path
$fluxbotExe = Join-Path $exePath "fluxbot.exe"

if (-not (Test-Path $fluxbotExe)) {
    Write-Host "❌ FEHLER: fluxbot.exe nicht gefunden!" -ForegroundColor Red
    Write-Host "Pfad: $fluxbotExe"
    exit 1
}

Write-Host "`n📍 Starte FluxBot von: $fluxbotExe`n" -ForegroundColor Cyan

# Starte FluxBot
try {
    & $fluxbotExe
} catch {
    Write-Host "❌ Fehler beim Starten: $_" -ForegroundColor Red
    exit 1
}
