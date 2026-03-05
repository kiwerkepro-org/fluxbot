# FluxBot Windows Service Installer
# Run as Administrator!
# Usage: powershell -ExecutionPolicy Bypass -File .\INSTALL-Service.ps1 -Action install

param(
    [string]$Action = "install"
)

$exePath = Split-Path -Parent $MyInvocation.MyCommand.Path
$fluxbotExe = Join-Path $exePath "fluxbot.exe"
$serviceName = "FluxBot"
$serviceDisplayName = "FluxBot AI Agent"

# Admin-Check
$IsAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole] "Administrator")
if (-not $IsAdmin) {
    Write-Host "ERROR: This script must run as Administrator!" -ForegroundColor Red
    exit 1
}

if (-not (Test-Path $fluxbotExe)) {
    Write-Host "ERROR: fluxbot.exe not found!" -ForegroundColor Red
    Write-Host "Expected at: $fluxbotExe" -ForegroundColor Yellow
    exit 1
}

Write-Host "================================================" -ForegroundColor Green
Write-Host "FluxBot Windows Service Manager" -ForegroundColor Green
Write-Host "================================================" -ForegroundColor Green

# Stop Service
function StopService {
    $svc = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if ($svc) {
        Write-Host "Stopping service..." -ForegroundColor Yellow
        Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 2
    }
}

# Uninstall Service
function UninstallService {
    $svc = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if ($svc) {
        Write-Host "Removing old service..." -ForegroundColor Yellow
        StopService
        sc.exe delete $serviceName 2>$null | Out-Null
        Start-Sleep -Seconds 1
        Write-Host "Service removed." -ForegroundColor Green
    }
}

# Install Service
function InstallService {
    Write-Host "Installing FluxBot Service..." -ForegroundColor Cyan
    Write-Host "  Service: $serviceName"
    Write-Host "  Exe: $fluxbotExe"
    Write-Host "  WorkDir: $exePath"

    # Create service
    sc.exe create $serviceName binPath= $fluxbotExe 2>$null | Out-Null

    if ($LASTEXITCODE -eq 0 -or $LASTEXITCODE -eq 1073) {
        sc.exe config $serviceName displayname= $serviceDisplayName 2>$null | Out-Null
        sc.exe config $serviceName start= auto 2>$null | Out-Null
        Write-Host "Service installed successfully." -ForegroundColor Green
        return $true
    } else {
        Write-Host "Service installation attempt completed (may already exist)." -ForegroundColor Yellow
        return $true
    }
}

# Start Service
function StartService {
    Write-Host "Starting service..." -ForegroundColor Cyan
    $svc = Get-Service -Name $serviceName -ErrorAction SilentlyContinue

    if (-not $svc) {
        Write-Host "Service not found. Run: .\INSTALL-Service.ps1 -Action install" -ForegroundColor Red
        return
    }

    if ($svc.Status -eq "Running") {
        Write-Host "Service already running!" -ForegroundColor Green
    } else {
        Start-Service -Name $serviceName -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 2
        $svc = Get-Service -Name $serviceName
        if ($svc.Status -eq "Running") {
            Write-Host "Service started successfully!" -ForegroundColor Green
        } else {
            Write-Host "Service failed to start. Check logs:" -ForegroundColor Yellow
            Write-Host "  $exePath\workspace\logs\fluxbot.log" -ForegroundColor Gray
        }
    }

    Write-Host ""
    Write-Host "Dashboard: http://localhost:9090" -ForegroundColor Cyan
}

# Show Status
function ShowStatus {
    $svc = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    if ($svc) {
        $color = if ($svc.Status -eq "Running") { "Green" } else { "Yellow" }
        Write-Host ""
        Write-Host "Service Status:" -ForegroundColor Cyan
        Write-Host "  Name: $($svc.Name)"
        Write-Host "  Status: $($svc.Status)" -ForegroundColor $color
        Write-Host "  StartType: $($svc.StartType)"

        if ($svc.Status -eq "Running") {
            Write-Host ""
            Write-Host "Dashboard available at: http://localhost:9090" -ForegroundColor Green
        }
    } else {
        Write-Host "Service not installed." -ForegroundColor Yellow
    }
}

# Main
switch ($Action.ToLower()) {
    "install" {
        UninstallService
        if (InstallService) {
            StartService
            ShowStatus
        }
    }
    "uninstall" {
        UninstallService
        Write-Host "FluxBot Service removed." -ForegroundColor Green
    }
    "start" {
        StartService
        ShowStatus
    }
    "stop" {
        StopService
        Write-Host "FluxBot Service stopped." -ForegroundColor Green
    }
    "status" {
        ShowStatus
    }
    default {
        Write-Host "Usage: .\INSTALL-Service.ps1 -Action install|uninstall|start|stop|status" -ForegroundColor Yellow
    }
}

Write-Host ""
