#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
#  FluxBot Installer für Linux & macOS
#  Verwendung:  curl -fsSL https://fluxbot.ki-werke.de/install.sh | bash
#
#  Zwei Modi:
#    Nativ  – Lädt Binary direkt von GitHub Releases (empfohlen)
#    Docker – Startet via docker-compose
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

# ── Konfiguration ─────────────────────────────────────────────────────────────
GH_REPO="kiwerkepro-org/fluxbot"
GH_API="https://api.github.com/repos/${GH_REPO}/releases/latest"
COMPOSE_URL="https://raw.githubusercontent.com/ki-werke/fluxbot/main/docker-compose.prod.yml"
INSTALL_DIR="$HOME/FluxBot"
DATA_DIR="$INSTALL_DIR/fluxbot-data"

# ── Farben ─────────────────────────────────────────────────────────────────────
CYAN='\033[0;36m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'
GRAY='\033[0;90m'; BOLD='\033[1m'; RESET='\033[0m'

banner() {
  echo ""
  echo -e "${CYAN}  ███████╗██╗     ██╗   ██╗██╗  ██╗██████╗  ██████╗ ████████╗${RESET}"
  echo -e "${CYAN}  ██╔════╝██║     ██║   ██║╚██╗██╔╝██╔══██╗██╔═══██╗╚══██╔══╝${RESET}"
  echo -e "${CYAN}  █████╗  ██║     ██║   ██║ ╚███╔╝ ██████╔╝██║   ██║   ██║   ${RESET}"
  echo -e "${CYAN}  ██╔══╝  ██║     ██║   ██║ ██╔██╗ ██╔══██╗██║   ██║   ██║   ${RESET}"
  echo -e "${CYAN}  ██║     ███████╗╚██████╔╝██╔╝ ██╗██████╔╝╚██████╔╝   ██║   ${RESET}"
  echo -e "${CYAN}  ╚═╝     ╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚═════╝  ╚═════╝   ╚═╝   ${RESET}"
  echo ""
  echo -e "${CYAN}  Multi-Channel AI Agent  ·  ki-werke.de${RESET}"
  echo ""
}

step()  { echo -e "  ${YELLOW}[$1]${RESET} $2"; }
ok()    { echo -e "  ${GREEN}✔${RESET}  $1"; }
fail()  { echo -e "  ${RED}✘${RESET}  $1"; }
info()  { echo -e "  ${GRAY}$1${RESET}"; }

# ── Betriebssystem ermitteln ───────────────────────────────────────────────────
OS="$(uname -s)"
ARCH="$(uname -m)"

# GitHub Asset-Namen nach Plattform
case "${OS}-${ARCH}" in
  Linux-x86_64)   ASSET_NAME="fluxbot-linux-amd64" ;;
  Linux-aarch64)  ASSET_NAME="fluxbot-linux-arm64" ;;
  Darwin-arm64)   ASSET_NAME="fluxbot-darwin-arm64" ;;
  Darwin-x86_64)  ASSET_NAME="fluxbot-darwin-amd64" ;;
  *)
    echo -e "  ${YELLOW}⚠${RESET}  Unbekannte Plattform: ${OS}-${ARCH}"
    echo -e "     Nativ-Installation möglicherweise nicht verfügbar."
    ASSET_NAME=""
    ;;
esac

# ── Hilfsfunktionen ───────────────────────────────────────────────────────────
download() {
  local url="$1" dest="$2"
  if command -v curl &>/dev/null; then
    curl -fsSL "$url" -o "$dest"
  elif command -v wget &>/dev/null; then
    wget -qO "$dest" "$url"
  else
    fail "Weder curl noch wget gefunden. Bitte eines davon installieren."
    exit 1
  fi
}

open_browser() {
  local url="$1"
  case "$OS" in
    Darwin) open "$url" 2>/dev/null || true ;;
    Linux)  command -v xdg-open &>/dev/null && xdg-open "$url" 2>/dev/null || true ;;
  esac
}

# ── Modus wählen ──────────────────────────────────────────────────────────────
banner

echo -e "  ${BOLD}Installationsmodus wählen:${RESET}"
echo ""
echo -e "  [1] Nativ  – Binary direkt (empfohlen, kein Docker nötig)"
echo -e "  [2] Docker – via Docker Compose"
echo ""
printf "  Auswahl [1/2]: "
read -r CHOICE

if [ "$CHOICE" = "2" ]; then
  install_docker
else
  install_native
fi

# ── Native Installation ────────────────────────────────────────────────────────
install_native() {
  echo ""
  echo -e "  ${CYAN}── Native Installation (${OS}/${ARCH}) ──────────────────────────${RESET}"

  if [ -z "$ASSET_NAME" ]; then
    fail "Kein Binary für ${OS}-${ARCH} verfügbar."
    info "Bitte manuell herunterladen: https://github.com/${GH_REPO}/releases"
    exit 1
  fi

  # 1. Neueste Version ermitteln
  step "1/5" "Suche neueste Version auf GitHub..."
  RELEASE_JSON=$(download "$GH_API" /dev/stdout 2>/dev/null || echo "")
  if [ -z "$RELEASE_JSON" ]; then
    fail "GitHub Releases nicht erreichbar."
    info "Bitte Internetverbindung prüfen."
    exit 1
  fi

  VERSION=$(echo "$RELEASE_JSON" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
  DOWNLOAD_URL=$(echo "$RELEASE_JSON" | grep "browser_download_url" | grep "$ASSET_NAME" | head -1 | sed 's/.*"browser_download_url": *"\([^"]*\)".*/\1/')

  if [ -z "$DOWNLOAD_URL" ]; then
    fail "Kein Asset '${ASSET_NAME}' in Release ${VERSION} gefunden."
    info "Bitte manuell herunterladen: https://github.com/${GH_REPO}/releases"
    exit 1
  fi
  ok "Neueste Version: $VERSION"

  # 2. Verzeichnisse
  step "2/5" "Richte Verzeichnisse ein..."
  mkdir -p "$INSTALL_DIR" "$DATA_DIR" "$DATA_DIR/skills" "$DATA_DIR/logs"
  ok "Verzeichnis: $INSTALL_DIR"

  # 3. Binary herunterladen
  step "3/5" "Lade FluxBot $VERSION herunter..."
  BINARY_PATH="$INSTALL_DIR/fluxbot"
  download "$DOWNLOAD_URL" "$BINARY_PATH"
  chmod +x "$BINARY_PATH"
  ok "Binary: $BINARY_PATH"

  # 4. Playwright-Browser installieren
  step "4/5" "Installiere Browser-Komponenten (Playwright)..."
  if "$BINARY_PATH" --install-playwright 2>/dev/null; then
    ok "Playwright-Browser installiert"
  else
    info "Browser-Installation übersprungen (optional – läuft nach manuellem: apt-get install -y libgbm1 libnss3)"
  fi

  # 5. Systemd-Service oder LaunchAgent
  step "5/5" "Richte Autostart ein..."
  case "$OS" in
    Linux)
      setup_systemd "$BINARY_PATH" "$DATA_DIR"
      ;;
    Darwin)
      setup_launchagent "$BINARY_PATH" "$DATA_DIR"
      ;;
  esac

  show_native_success "$VERSION" "$INSTALL_DIR" "$DATA_DIR" "$BINARY_PATH"
}

# ── Systemd-Service (Linux) ───────────────────────────────────────────────────
setup_systemd() {
  local binary="$1" data="$2"
  local service_file="$HOME/.config/systemd/user/fluxbot.service"
  mkdir -p "$(dirname "$service_file")"
  cat > "$service_file" <<EOF
[Unit]
Description=FluxBot – Multi-Channel AI Agent
After=network.target

[Service]
ExecStart=$binary --config $data/config.json
WorkingDirectory=$INSTALL_DIR
Restart=on-failure
RestartSec=10
StandardOutput=append:$data/logs/fluxbot.log
StandardError=append:$data/logs/fluxbot.log

[Install]
WantedBy=default.target
EOF

  systemctl --user daemon-reload
  systemctl --user enable fluxbot
  systemctl --user start  fluxbot
  ok "Systemd-Service 'fluxbot' (User-Unit) aktiv"
}

# ── LaunchAgent (macOS) ────────────────────────────────────────────────────────
setup_launchagent() {
  local binary="$1" data="$2"
  local plist="$HOME/Library/LaunchAgents/de.ki-werke.fluxbot.plist"
  mkdir -p "$(dirname "$plist")"
  cat > "$plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>             <string>de.ki-werke.fluxbot</string>
  <key>ProgramArguments</key>
  <array>
    <string>$binary</string>
    <string>--config</string>
    <string>$data/config.json</string>
  </array>
  <key>WorkingDirectory</key>  <string>$INSTALL_DIR</string>
  <key>RunAtLoad</key>         <true/>
  <key>KeepAlive</key>         <true/>
  <key>StandardOutPath</key>   <string>$data/logs/fluxbot.log</string>
  <key>StandardErrorPath</key> <string>$data/logs/fluxbot.log</string>
</dict>
</plist>
EOF

  launchctl load "$plist" 2>/dev/null || launchctl bootstrap "gui/$(id -u)" "$plist" 2>/dev/null || true
  ok "LaunchAgent 'de.ki-werke.fluxbot' geladen"
}

# ── Docker Installation ────────────────────────────────────────────────────────
install_docker() {
  echo ""
  echo -e "  ${CYAN}── Docker Installation ────────────────────────────────────────${RESET}"

  step "1/4" "Prüfe Docker..."
  if ! command -v docker &>/dev/null; then
    fail "Docker ist nicht installiert."
    case "$OS" in
      Darwin) info "https://docs.docker.com/desktop/install/mac-install/" ;;
      Linux)  info "https://docs.docker.com/engine/install/" ;;
    esac
    exit 1
  fi
  if ! docker info &>/dev/null 2>&1; then
    fail "Docker läuft nicht."
    case "$OS" in
      Darwin) info "Bitte Docker Desktop starten." ;;
      Linux)  info "sudo systemctl start docker" ;;
    esac
    exit 1
  fi
  ok "Docker läuft"

  step "2/4" "Richte Verzeichnisse ein..."
  mkdir -p "$INSTALL_DIR" "$DATA_DIR"
  ok "Verzeichnis: $INSTALL_DIR"

  step "3/4" "Lade Konfiguration herunter..."
  COMPOSE_PATH="$INSTALL_DIR/docker-compose.yml"
  download "$COMPOSE_URL" "$COMPOSE_PATH"
  ok "docker-compose.yml heruntergeladen"

  step "4/4" "Starte FluxBot..."
  cd "$INSTALL_DIR"
  docker compose pull --quiet
  docker compose up -d
  ok "FluxBot (Docker) läuft!"

  echo ""
  echo -e "  ${GREEN}╔════════════════════════════════════════════════════════════╗${RESET}"
  echo -e "  ${GREEN}║   FluxBot (Docker) erfolgreich installiert!               ║${RESET}"
  echo -e "  ${GREEN}║   👉  http://localhost:8090                                ║${RESET}"
  echo -e "  ${GREEN}╚════════════════════════════════════════════════════════════╝${RESET}"
  echo ""
  info "Stoppen:  docker compose -f \"$COMPOSE_PATH\" down"
  info "Updaten:  docker compose -f \"$COMPOSE_PATH\" pull && docker compose -f \"$COMPOSE_PATH\" up -d"
  echo ""
  sleep 3
  open_browser "http://localhost:8090"
}

# ── Erfolgs-Ausgabe: Nativ ────────────────────────────────────────────────────
show_native_success() {
  local version="$1" install_dir="$2" data_dir="$3" binary="$4"
  echo ""
  echo -e "  ${GREEN}╔════════════════════════════════════════════════════════════╗${RESET}"
  echo -e "  ${GREEN}║                                                            ║${RESET}"
  echo -e "  ${GREEN}║   FluxBot ${version} erfolgreich installiert!              ║${RESET}"
  echo -e "  ${GREEN}║                                                            ║${RESET}"
  echo -e "  ${GREEN}║   👉  http://localhost:9090   (Setup-Assistent)            ║${RESET}"
  echo -e "  ${GREEN}║                                                            ║${RESET}"
  echo -e "  ${GREEN}╚════════════════════════════════════════════════════════════╝${RESET}"
  echo ""
  info "Installationsverzeichnis: $install_dir"
  info "Daten (config, Skills):   $data_dir"
  info "Binary:                   $binary"
  info ""
  case "$OS" in
    Linux)  info "Stoppen:  systemctl --user stop fluxbot"
            info "Updaten:  Dashboard → Status → Update installieren" ;;
    Darwin) info "Stoppen:  launchctl unload ~/Library/LaunchAgents/de.ki-werke.fluxbot.plist"
            info "Updaten:  Dashboard → Status → Update installieren" ;;
  esac
  echo ""
  sleep 3
  open_browser "http://localhost:9090"
}
