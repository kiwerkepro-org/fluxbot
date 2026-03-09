.PHONY: build run run-debug test clean tidy \
        build-linux build-windows build-mac build-all \
        docker-build docker-up docker-down docker-logs \
        install-linux help

# ── Build-Variablen ────────────────────────────────────────────────────────────
BINARY  = fluxbot
MODULE  = github.com/ki-werke/fluxbot
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Build-Flags: kein CGO, Debug-Symbole entfernen (kleinere Binaries)
LDFLAGS         = -s -w -X main.version=$(VERSION)
# Windows: kein -H windowsgui – stattdessen FreeConsole() in main() für Terminal-Unabhängigkeit
LDFLAGS_WINDOWS = -s -w -X main.version=$(VERSION)

# Ausgabe-Verzeichnis für Cross-Builds
DIST = dist

# ── Lokale Entwicklung ─────────────────────────────────────────────────────────

## build: Binary für das aktuelle System kompilieren
build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/fluxbot

## run: Bot lokal starten
run:
	go run ./cmd/fluxbot --config workspace/config.json

## run-debug: Bot mit Debug-Logging starten
run-debug:
	go run ./cmd/fluxbot --config workspace/config.json --debug

## test: Tests ausführen
test:
	go test ./...

## tidy: go.mod und go.sum aufräumen
tidy:
	go mod tidy

## clean: Kompilate entfernen
clean:
	rm -f $(BINARY)
	rm -rf $(DIST)
	go clean

# ── Cross-Platform Builds ──────────────────────────────────────────────────────

## build-linux: Linux Binary (amd64)
build-linux:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags="$(LDFLAGS)" \
		-o $(DIST)/fluxbot-linux-amd64 \
		./cmd/fluxbot
	@echo "✅ $(DIST)/fluxbot-linux-amd64"

## build-linux-arm: Linux Binary (arm64, z.B. Raspberry Pi / AWS Graviton)
build-linux-arm:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
		-ldflags="$(LDFLAGS)" \
		-o $(DIST)/fluxbot-linux-arm64 \
		./cmd/fluxbot
	@echo "✅ $(DIST)/fluxbot-linux-arm64"

## build-windows: Windows Binary (amd64) – läuft ohne Konsolenfenster
build-windows:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
		-ldflags="$(LDFLAGS_WINDOWS)" \
		-o $(DIST)/fluxbot-windows-amd64.exe \
		./cmd/fluxbot
	@echo "✅ $(DIST)/fluxbot-windows-amd64.exe"

## build-mac: macOS Binary (Apple Silicon / M1+)
build-mac:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
		-ldflags="$(LDFLAGS)" \
		-o $(DIST)/fluxbot-darwin-arm64 \
		./cmd/fluxbot
	@echo "✅ $(DIST)/fluxbot-darwin-arm64"

## build-mac-intel: macOS Binary (Intel x86_64)
build-mac-intel:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
		-ldflags="$(LDFLAGS)" \
		-o $(DIST)/fluxbot-darwin-amd64 \
		./cmd/fluxbot
	@echo "✅ $(DIST)/fluxbot-darwin-amd64"

## build-all: Alle Plattformen auf einmal bauen
build-all: build-linux build-linux-arm build-windows build-mac build-mac-intel
	@echo ""
	@echo "✅ Alle Builds fertig:"
	@ls -lh $(DIST)/

# ── Native Installation ────────────────────────────────────────────────────────

## install-linux: FluxBot als Linux-Systemd-Dienst installieren (als root)
install-linux: build-linux
	@echo "Installiere FluxBot auf Linux..."
	sudo bash deploy/linux/install.sh $(DIST)/fluxbot-linux-amd64

# ── Docker ─────────────────────────────────────────────────────────────────────

## docker-build: Docker-Image bauen
docker-build:
	docker-compose build

## docker-up: Docker-Stack starten (mit Rebuild)
docker-up:
	docker-compose up --build -d fluxbot

## docker-down: Docker-Stack stoppen
docker-down:
	docker-compose down

## docker-logs: Docker-Logs live verfolgen
docker-logs:
	docker-compose logs -f fluxbot

## docker-restart: FluxBot Container neu starten (ohne Rebuild)
docker-restart:
	docker-compose restart fluxbot; docker-compose logs -f fluxbot

# ── Hilfe ──────────────────────────────────────────────────────────────────────

## help: Alle verfügbaren Befehle anzeigen
help:
	@echo ""
	@echo "FluxBot Makefile – verfügbare Befehle:"
	@echo ""
	@echo "  Lokale Entwicklung:"
	@echo "    make build          - Binary für aktuelles System"
	@echo "    make run            - Bot starten"
	@echo "    make run-debug      - Bot mit Debug-Logging"
	@echo "    make test           - Tests ausführen"
	@echo "    make tidy           - go.mod aufräumen"
	@echo "    make clean          - Kompilate entfernen"
	@echo ""
	@echo "  Cross-Platform Builds (in ./dist/):"
	@echo "    make build-linux    - Linux amd64"
	@echo "    make build-linux-arm- Linux arm64 (Raspberry Pi)"
	@echo "    make build-windows  - Windows amd64 (.exe)"
	@echo "    make build-mac      - macOS arm64 (M1+)  → fluxbot-darwin-arm64"
	@echo "    make build-mac-intel- macOS amd64 (Intel) → fluxbot-darwin-amd64"
	@echo "    make build-all      - Alle Plattformen"
	@echo ""
	@echo "  Native Installation:"
	@echo "    make install-linux  - Als Systemd-Dienst installieren"
	@echo "    (Windows: deploy\windows\install.ps1 als Admin)"
	@echo ""
	@echo "  Docker:"
	@echo "    make docker-up      - Starten (mit Rebuild)"
	@echo "    make docker-down    - Stoppen"
	@echo "    make docker-logs    - Logs live"
	@echo "    make docker-restart - Neustart ohne Rebuild"
	@echo ""
