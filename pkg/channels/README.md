# FluxBot Channels Package

Channel-Implementierungen für verschiedene Chat-Plattformen (Slack, Matrix, Discord, Telegram, WhatsApp).

## Architektur

Alle Channels implementieren das gemeinsame Interface:

```go
type Channel interface {
    Name() string
    Start(ctx context.Context, bus chan<- Message) error
    Stop()
    Send(chatID string, text string) error
    TypingIndicator(chatID string)
}
```

## Channel Types

### 1. Slack (`slack.go`)

**Protokoll:** HTTP Events API Webhook  
**Port:** 3000 (konfigurierbar)  
**Auth:** HMAC-SHA256 Signature

```
                                  ┌─────────────────────────────┐
Event (message.im, message.channels) │ Slack Events API Server     │
             ──────────────────>  │ http://localhost:3000/slack │
                                  │ /events                     │
                                  └────────┬────────────────────┘
                                           │
                                           │ HMAC verify
                                           │
                                  ┌────────▼─────────────────────┐
                                  │ SlackChannel.handleEvent()    │
                                  │ Parse event → Message struct  │
                                  └────────┬─────────────────────┘
                                           │
                                           │ Send to bus
                                           │
                                  ┌────────▼──────────────────────┐
                                  │ Agent.Run() processes message │
                                  │ AI response generation        │
                                  └────────┬──────────────────────┘
                                           │
                    Response message       │
                                           │
                        ┌──────────────────▼──────────────────┐
                        │ SlackChannel.Send()                 │
                        │ POST /_matrix/client/v3/chat.post   │
                        │ Message via Slack Web API           │
                        └─────────────────────────────────────┘
```

**Key Features:**
- URL-Challenge-Verifizierung beim Setup
- HMAC-Signature-Verifizierung mit `X-Slack-Request-Timestamp`
- Bot-Mention-Entfernung aus Message-Text
- Message-Chunking (3000 chars max pro Slack-Nachricht)
- Whitelist-Support (User/Channel-IDs)

**Typen:**
- `SlackChannel` – Main handler
- `slackEventPayload` – Event-Struktur von Slack
- `slackEvent` – Event-Details

### 2. Matrix (`matrix.go`)

**Protokoll:** Client-Server API mit Long-Polling  
**Auth:** Bearer Token  
**Long-Poll Timeout:** 30 Sekunden

```
                                  ┌──────────────────────────────┐
                            ┌────> GET /_matrix/client/v3/sync  │
                            │     (timeout: 30s, since: batch)  │
                            │     Bearer: syt_xxx               │
                            │     Filter: room timeline limit=10│
                    ┌───────┴──────┐                            │
                    │              └────────────────────────────┤
                    │ Response: new events + next_batch         │
         ┌──────────┴──────┐       │
         │                 │       │
    5 Sec(timeout      (next_batch)
  retry)              │
         │            │
      ┌──┴─────┬──────▼────────────┐
      │         │                   │
      │    [loop] MatrixChannel.sync()
      │         │   ↓
      │         │ processSync()
      │         │   ↓
      │         │ Filter for m.room.message
      │         │   ↓
      │         │ Send to bus
      │         │
      └─────────┘
      
      Agent processes → MatrixChannel.Send()
                        PUT /_matrix/client/v3/rooms/{roomID}/send/m.room.message/{txID}
                        {"msgtype":"m.text", "body":"response"}
```

**Key Features:**
- Continuous `/sync` Long-Polling (30s default)
- Token-based authentication
- Automatic retry on error (5s backoff)
- Filter support (room timeline limit)
- Typing indicator via `/typing` endpoint
- Connection health check via `whoami`
- Room filtering (join, invite, leave)

**Typen:**
- `MatrixChannel` – Main handler
- `matrixSyncResponse` – /sync endpoint response
- `matrixRoom` – Room state and events
- `matrixEvent` – m.room.message and other events

### 3. Discord (`discord.go`) – STUB

**Status:** ⚠️ Not implemented (Phase 2.5)  
**Protocol:** WebSocket Gateway (gRPC-like binary frames)  
**Library:** github.com/bwmarrin/discordgo recommended

**Stub Implementation:**
- Logs hint to install `discordgo`
- Returns "not implemented" errors
- Ready for Phase 2.5 upgrade

---

## Message Bus Architecture

```go
type Message struct {
    ID        string         // Platform-specific message ID
    ChannelID string         // "slack", "matrix", "discord"
    ChatID    string         // Slack: channel, Matrix: room_id, Discord: channel_id
    SenderID  string         // User identifier on platform
    Type      MessageType    // MessageTypeText (for now)
    Text      string         // Message content
}
```

**Flow:**
1. Channel receives external message
2. Parses into `Message` struct
3. Sends to `manager.bus` (buffered channel, size=100)
4. `Agent.Run()` reads from bus
5. Processes with AI Provider + Skills
6. Calls `channel.Send()` with response

---

## Configuration

Each channel has its own config struct:

```go
// Slack
type SlackConfig struct {
    BotToken      string   // xoxb-...
    AppToken      string   // xapp-...
    SigningSecret string   // HMAC verification
    WebhookPort   int      // HTTP server port
    AllowFrom     []string // User/Channel whitelist
}

// Matrix
type MatrixConfig struct {
    HomeServer string   // https://matrix.org
    UserID     string   // @fluxbot:matrix.org
    Token      string   // syt_... access token
    AllowFrom  []string // User whitelist
}

// Discord
type DiscordConfig struct {
    Token     string   // Bot token
    AllowFrom []string // User whitelist
}
```

---

## Error Handling

### Slack
- **Invalid signature** → 401 Unauthorized + log
- **URL validation** → Return challenge in response
- **API errors** → Logged, message sent to stderr, continues
- **Bot messages** → Skipped (check bot_id + subtype)

### Matrix
- **Invalid token** → Error on Start(), exit
- **HTTP 401** → Log, 5s retry
- **Connection timeout** → 5s retry
- **Sync parse error** → 5s retry
- **Send failure** → Return error to agent, log

### Discord
- Not implemented

---

## Security

### Slack
- HMAC-SHA256 verification required
- Timestamp validation (prevents replay attacks)
- Only handles POST requests
- Verifies `X-Slack-Request-Timestamp` header
- Whitelist support

### Matrix
- Bearer token authentication
- HTTPS recommended for production
- Token stored in config (use env vars in production)
- User/room filtering

### Discord
- Token in config (env vars recommended)
- User whitelist

---

## Performance

| Metric | Slack | Matrix | Discord |
|--------|-------|--------|---------|
| Latency | < 1s | 0-30s (polling) | < 100ms |
| Message Limit | 1000/min | 100/min | Unlimited |
| CPU (idle) | 1-5% | 1-3% | TBD |
| Memory (idle) | 15MB | 10MB | TBD |

### Optimization Tips

**Slack:**
- Increase `webhookPort` load-balancing if high traffic
- Use message chunking for large responses

**Matrix:**
- Adjust `/sync` timeout for latency/load tradeoff
- Filter parameter reduces response size
- Local homeserver recommended for high traffic

---

## Testing

### Unit Tests
```bash
go test ./pkg/channels -v
```

### Integration Tests

**Slack:**
```bash
curl -X POST http://localhost:3000/slack/events \
  -H "X-Slack-Request-Timestamp: $(date +%s)" \
  -H "X-Slack-Signature: v0=..." \
  -d '{"type":"event_callback","event":{"type":"message","text":"test"}}'
```

**Matrix:**
```bash
curl -H "Authorization: Bearer syt_xxx" \
  https://matrix.org/_matrix/client/v3/account/whoami
```

---

## File Structure

```
pkg/channels/
├── channels.go          # Manager + Message types
├── discord.go           # Discord stub
├── slack.go             # Slack Events API
├── matrix.go            # Matrix Long-Polling
├── telegram.go          # Telegram webhook (existing)
├── whatsapp.go          # WhatsApp Cloud API (existing)
└── README.md            # This file
```

---

## Dependencies

**No external dependencies required** for Slack and Matrix.

Discord implementation will require:
```
github.com/bwmarrin/discordgo
```

Standard library used:
- `net/http` – HTTP server (Slack, Matrix API)
- `crypto/hmac` – HMAC verification (Slack)
- `encoding/json` – JSON parsing
- `context` – Cancellation
- `log` – Logging

---

## Contributing

1. Implement `Channel` interface
2. Add config struct in `pkg/config/config.go`
3. Add validation in `pkg/config/validator.go`
4. Register in `cmd/fluxbot/main.go`
5. Document in `CHANNELS_SETUP.md`
6. Test with production/development setup

---

**Last Updated:** 2026-02-18  
**FluxBot Version:** 1.0  
**Package Version:** 1.0
