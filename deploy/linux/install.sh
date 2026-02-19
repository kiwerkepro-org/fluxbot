#!/bin/bash
# FluxBot – Linux Native Install Script
# Installiert FluxBot als systemd-Dienst auf Linux (Debian/Ubuntu/RHEL/Arch)
#
# Verwendung:
#   sudo bash deploy/linux/install.sh [BINARY_PFAD] [WORKSPACE_PFAD]
#
# Beispiel:
#   sudo bash deploy/linux/install.sh ./fluxbot /opt/fluxbot/workspace

set -e

# ── Farben ────────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info()    { echo -e "${BLUE}[FluxBot]${NC} $1"; }
success() { echo -e "${GREEN}[✅ OK]${NC} $1"; }
warn()    { echo -e "${YELLOW}[⚠️ Warnung]${NC} $1"; }
error()   { echo -e "${RED}[❌ Fehler]${NC} $1"; exit 1; }

# ── Root-Check ────────────────────────────────────────────────────────────────
if [ "$EUID" -ne 0 ]; then
    error "Dieses Script muss als root ausgeführt werden: sudo bash $0"
fi

# ── Parameter ─────────────────────────────────────────────────────────────────
BINARY="${1:-./fluxbot}"
WORKSPACE="${2:-/opt/fluxbot/workspace}"
INSTALL_BIN="/usr/local/bin/fluxbot"
INSTALL_DIR="/opt/fluxbot"
SERVICE_FILE="/etc/systemd/system/fluxbot.service"
SERVICE_USER="fluxbot"

echo ""
echo "╔══════════════════════════════════════════════════╗"
echo "║  FluxBot – Linux Native Installer                ║"
echo "║  KI-WERKE | github.com/ki-werke/fluxbot          ║"
echo "╚══════════════════════════════════════════════════╝"
echo ""

# ── Vorabprüfungen ────────────────────────────────────────────────────────────
info "Prüfe Voraussetzungen..."

if [ ! -f "$BINARY" ]; then
    error "Binary nicht gefunden: $BINARY\n  Erst kompilieren: GOOS=linux GOARCH=amd64 go build -o fluxbot ./cmd/fluxbot"
fi

if ! command -v systemctl &>/dev/null; then
    error "systemd nicht gefunden. Dieses Script unterstützt nur systemd-basierte Systeme."
fi

# ffmpeg prüfen (für Voice-Funktionen)
if ! command -v ffmpeg &>/dev/null; then
    warn "ffmpeg nicht gefunden. Voice-Funktionen werden ohne ffmpeg eingeschränkt sein."
    warn "Installieren: apt install ffmpeg  /  dnf install ffmpeg  /  pacman -S ffmpeg"
fi

success "Voraussetzungen OK"

# ── Systembenutzer erstellen ──────────────────────────────────────────────────
info "Erstelle Systembenutzer '$SERVICE_USER'..."
if id "$SERVICE_USER" &>/dev/null; then
    warn "Benutzer '$SERVICE_USER' existiert bereits – überspringe"
else
    useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
    success "Benutzer '$SERVICE_USER' erstellt"
fi

# ── Verzeichnisse erstellen ───────────────────────────────────────────────────
info "Erstelle Verzeichnisstruktur..."
mkdir -p "$INSTALL_DIR/workspace/sessions"
mkdir -p "$INSTALL_DIR/workspace/logs"
mkdir -p "$INSTALL_DIR/workspace/skills"
chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"
success "Verzeichnisse erstellt: $INSTALL_DIR"

# ── config.json prüfen / kopieren ─────────────────────────────────────────────
CONFIG_DEST="$INSTALL_DIR/workspace/config.json"
if [ -f "$CONFIG_DEST" ]; then
    warn "config.json existiert bereits – wird nicht überschrieben"
elif [ -f "workspace/config.json" ]; then
    cp workspace/config.json "$CONFIG_DEST"
    chown "$SERVICE_USER:$SERVICE_USER" "$CONFIG_DEST"
    chmod 600 "$CONFIG_DEST"
    success "config.json kopiert"
elif [ -f "workspace/config.example.json" ]; then
    cp workspace/config.example.json "$CONFIG_DEST"
    chown "$SERVICE_USER:$SERVICE_USER" "$CONFIG_DEST"
    chmod 600 "$CONFIG_DEST"
    warn "config.example.json kopiert nach $CONFIG_DEST – bitte API-Keys eintragen!"
else
    warn "Keine config.json gefunden – bitte manuell erstellen: $CONFIG_DEST"
fi

# ── SOUL.md kopieren ──────────────────────────────────────────────────────────
for f in SOUL.md IDENTITY.md; do
    if [ -f "workspace/$f" ]; then
        cp "workspace/$f" "$INSTALL_DIR/workspace/$f"
        chown "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR/workspace/$f"
        success "$f kopiert"
    fi
done

# ── Binary installieren ───────────────────────────────────────────────────────
info "Installiere Binary nach $INSTALL_BIN..."
cp "$BINARY" "$INSTALL_BIN"
chmod 755 "$INSTALL_BIN"
success "Binary installiert: $INSTALL_BIN"

# ── systemd Service installieren ──────────────────────────────────────────────
info "Installiere systemd Service..."

# Service-Template anpassen (Workspace-Pfad eintragen)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_TEMPLATE="$SCRIPT_DIR/fluxbot.service"

if [ ! -f "$SERVICE_TEMPLATE" ]; then
    # Inline-Fallback wenn Template nicht gefunden
    cat > "$SERVICE_FILE" << EOF
[Unit]
Description=FluxBot – Multi-Channel AI Agent by KI-WERKE
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_BIN --config $INSTALL_DIR/workspace/config.json
Restart=on-failure
RestartSec=10s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=fluxbot
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=$INSTALL_DIR/workspace
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF
else
    # Template kopieren und Pfade anpassen
    sed "s|/opt/fluxbot|$INSTALL_DIR|g; s|/usr/local/bin/fluxbot|$INSTALL_BIN|g; s|User=fluxbot|User=$SERVICE_USER|g" \
        "$SERVICE_TEMPLATE" > "$SERVICE_FILE"
fi

chmod 644 "$SERVICE_FILE"
success "Service-Datei installiert: $SERVICE_FILE"

# ── systemd neu laden und aktivieren ──────────────────────────────────────────
info "Aktiviere und starte FluxBot Service..."
systemctl daemon-reload
systemctl enable fluxbot
systemctl start fluxbot

sleep 2

# Status prüfen
if systemctl is-active --quiet fluxbot; then
    success "FluxBot läuft!"
else
    warn "FluxBot konnte nicht gestartet werden. Logs prüfen:"
    echo "  journalctl -u fluxbot -n 50 --no-pager"
fi

# ── Zusammenfassung ───────────────────────────────────────────────────────────
echo ""
echo "╔══════════════════════════════════════════════════╗"
echo "║  Installation abgeschlossen                      ║"
echo "╚══════════════════════════════════════════════════╝"
echo ""
echo "  Binary:     $INSTALL_BIN"
echo "  Workspace:  $INSTALL_DIR/workspace"
echo "  Config:     $INSTALL_DIR/workspace/config.json"
echo "  Service:    $SERVICE_FILE"
echo ""
echo "  Nützliche Befehle:"
echo "    journalctl -u fluxbot -f          # Live-Logs"
echo "    systemctl status fluxbot          # Status"
echo "    systemctl restart fluxbot         # Neustart"
echo "    systemctl stop fluxbot            # Stoppen"
echo "    systemctl disable fluxbot         # Deaktivieren"
echo ""

if [ -f "$INSTALL_DIR/workspace/config.json" ]; then
    echo "  ⚠️  Bitte config.json prüfen und API-Keys eintragen:"
    echo "     nano $INSTALL_DIR/workspace/config.json"
    echo "     systemctl restart fluxbot"
    echo ""
fi
