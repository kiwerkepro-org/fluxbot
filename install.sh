#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
#  FluxBot Installer für Linux & macOS (Docker-Variante)
#  Verwendung:  curl -fsSL https://fluxbot.ki-werke.de/install.sh | bash
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

# ── Farben ───────────────────────────────────────────────────────────────────
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
GRAY='\033[0;90m'
RESET='\033[0m'

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

# ── Hauptskript ───────────────────────────────────────────────────────────────
banner

OS="$(uname -s)"

# ── 1. Docker prüfen ─────────────────────────────────────────────────────────
step "1/4" "Prüfe Docker..."

if ! command -v docker &>/dev/null; then
  fail "Docker ist nicht installiert."
  info ""
  case "$OS" in
    Darwin)
      info "Bitte Docker Desktop für macOS installieren:"
      info "https://docs.docker.com/desktop/install/mac-install/"
      ;;
    Linux)
      info "Bitte Docker installieren:"
      info "https://docs.docker.com/engine/install/"
      ;;
  esac
  info ""
  info "Nach der Installation diesen Befehl erneut ausführen."
  exit 1
fi

if ! docker info &>/dev/null 2>&1; then
  fail "Docker läuft nicht."
  info ""
  case "$OS" in
    Darwin) info "Bitte Docker Desktop starten." ;;
    Linux)  info "Bitte Docker-Daemon starten:  sudo systemctl start docker" ;;
  esac
  info ""
  exit 1
fi

ok "Docker läuft"

# ── 2. Installationsverzeichnis ───────────────────────────────────────────────
step "2/4" "Richte Installationsverzeichnis ein..."

INSTALL_DIR="$HOME/FluxBot"
DATA_DIR="$INSTALL_DIR/fluxbot-data"

mkdir -p "$INSTALL_DIR" "$DATA_DIR"
ok "Verzeichnis: $INSTALL_DIR"

# ── 3. docker-compose.yml herunterladen ───────────────────────────────────────
step "3/4" "Lade Konfiguration herunter..."

COMPOSE_URL="https://raw.githubusercontent.com/ki-werke/fluxbot/main/docker-compose.prod.yml"
COMPOSE_PATH="$INSTALL_DIR/docker-compose.yml"

if command -v curl &>/dev/null; then
  curl -fsSL "$COMPOSE_URL" -o "$COMPOSE_PATH"
elif command -v wget &>/dev/null; then
  wget -qO "$COMPOSE_PATH" "$COMPOSE_URL"
else
  fail "Weder curl noch wget gefunden. Bitte eines davon installieren."
  exit 1
fi

ok "docker-compose.yml heruntergeladen"

# ── 4. FluxBot starten ────────────────────────────────────────────────────────
step "4/4" "Starte FluxBot..."

cd "$INSTALL_DIR"
docker compose pull --quiet
docker compose up -d

ok "FluxBot läuft!"

# ── Fertig ────────────────────────────────────────────────────────────────────
echo ""
echo -e "  ${GREEN}╔══════════════════════════════════════════════════════╗${RESET}"
echo -e "  ${GREEN}║                                                      ║${RESET}"
echo -e "  ${GREEN}║   FluxBot wurde erfolgreich installiert!             ║${RESET}"
echo -e "  ${GREEN}║                                                      ║${RESET}"
echo -e "  ${GREEN}║   👉  http://localhost:8090                          ║${RESET}"
echo -e "  ${GREEN}║                                                      ║${RESET}"
echo -e "  ${GREEN}║   Der Einrichtungsassistent öffnet sich gleich.      ║${RESET}"
echo -e "  ${GREEN}║   Folge den Schritten um FluxBot zu konfigurieren.   ║${RESET}"
echo -e "  ${GREEN}║                                                      ║${RESET}"
echo -e "  ${GREEN}╚══════════════════════════════════════════════════════╝${RESET}"
echo ""
info "Installationsverzeichnis: $INSTALL_DIR"
info "Daten (config, Skills):   $DATA_DIR"
info ""
info "FluxBot stoppen:   docker compose -f \"$COMPOSE_PATH\" down"
info "FluxBot updaten:   docker compose -f \"$COMPOSE_PATH\" pull && docker compose -f \"$COMPOSE_PATH\" up -d"
echo ""

# Browser öffnen (kurz warten damit der Container hochfährt)
sleep 3
case "$OS" in
  Darwin) open "http://localhost:8090" 2>/dev/null || true ;;
  Linux)
    if command -v xdg-open &>/dev/null; then
      xdg-open "http://localhost:8090" 2>/dev/null || true
    fi
    ;;
esac
