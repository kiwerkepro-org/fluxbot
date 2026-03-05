# --- STAGE 1: BUILDER ---
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Installiere Build-Abhängigkeiten (git für go mod download)
RUN apk add --no-cache git

# Abhängigkeiten zuerst cachen (besseres Layer-Caching)
COPY go.mod go.sum ./
RUN go mod download

# Quellcode kopieren
COPY . .

# Version als Build-Argument (wird vom Release-Workflow übergeben)
ARG VERSION=dev

# Kompiliere den Bot (Version wird per ldflags eingebettet)
RUN CGO_ENABLED=0 GOOS=linux go build -mod=mod \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o fluxbot ./cmd/fluxbot

# --- STAGE 2: FINAL IMAGE ---
FROM alpine:latest

# Installiere Laufzeit-Abhängigkeiten
RUN apk add --no-cache ffmpeg ca-certificates tzdata

# Deutsche Zeitzone als Standard
ENV TZ=Europe/Berlin

WORKDIR /app

# Kompilierter Bot aus Builder-Stage
COPY --from=builder /build/fluxbot /app/

# Workspace-Verzeichnis erstellen
RUN mkdir -p /app/workspace

# Kein Root im Container
RUN addgroup -S fluxbot && adduser -S fluxbot -G fluxbot
USER fluxbot

# Starte FluxBot
ENTRYPOINT ["/app/fluxbot", "--config", "/app/workspace/config.json"]
