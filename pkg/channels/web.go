package channels

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// wsIncoming ist ein eingehender WebSocket-Frame vom Browser.
type wsIncoming struct {
	Type string `json:"type"` // "text", "image", "voice"
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"` // base64-kodierte Binärdaten (Bild/Audio)
	Mime string `json:"mime,omitempty"` // z.B. "image/jpeg", "audio/wav"
}

// WsOutgoing ist ein ausgehender WebSocket-Frame an den Browser.
// Exportiert damit Dashboard-Package es nutzen kann.
type WsOutgoing struct {
	Type string `json:"type"` // "chunk", "message", "typing", "done", "error", "audio"
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"` // base64-kodierte Audio-Bytes (OGG/Opus)
	Mime string `json:"mime,omitempty"` // z.B. "audio/ogg"
}

// wsConn bündelt eine WebSocket-Verbindung mit einem exklusiven Schreib-Mutex.
// gorilla/websocket erlaubt genau einen gleichzeitigen Writer – daher Mutex nötig.
type wsConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *wsConn) writeJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

func (c *wsConn) writeMessage(msgType int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(msgType, data)
}

// WebChannel implementiert Channel für direkten Browser-WebSocket-Zugriff.
// Anders als andere Channels nutzt er keinen Bus, sondern einen direkten Stream-Handler.
type WebChannel struct {
	conns         sync.Map                                                              // connID → *wsConn
	streamHandler func(ctx context.Context, msg Message, sendChunk func(string))       // Agent-Callback
	upgrader      websocket.Upgrader
	tempDir       string // Verzeichnis für temporäre Mediendateien
}

// NewWebChannel erstellt einen neuen WebChannel.
func NewWebChannel() *WebChannel {
	return &WebChannel{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024 * 1024, // 1 MB
			WriteBufferSize: 1024 * 1024,
			CheckOrigin:     func(r *http.Request) bool { return true }, // localhost only
		},
		tempDir: os.TempDir(),
	}
}

// SetStreamHandler setzt den Agent-Callback für Streaming-Antworten.
// Wird in main.go aufgerufen nachdem der Agent erstellt wurde.
func (c *WebChannel) SetStreamHandler(h func(ctx context.Context, msg Message, sendChunk func(string))) {
	c.streamHandler = h
}

// Name gibt den eindeutigen Kanalnamen zurück.
func (c *WebChannel) Name() string { return "web" }

// Start implementiert Channel-Interface – WebChannel nutzt keinen Bus.
// Verbindungen werden direkt via HandleConnection vom Dashboard-Server verwaltet.
func (c *WebChannel) Start(ctx context.Context, bus chan<- Message) error {
	<-ctx.Done()
	return nil
}

// Stop schließt alle aktiven WebSocket-Verbindungen.
func (c *WebChannel) Stop() {
	c.conns.Range(func(key, value interface{}) bool {
		if wc, ok := value.(*wsConn); ok {
			wc.conn.Close()
		}
		return true
	})
}

// Send schickt eine vollständige Textnachricht an eine WebSocket-Verbindung.
func (c *WebChannel) Send(chatID string, text string) error {
	if v, ok := c.conns.Load(chatID); ok {
		return v.(*wsConn).writeJSON(WsOutgoing{Type: "message", Text: text})
	}
	return fmt.Errorf("web: keine Verbindung für chatID %s", chatID)
}

// SendPhoto sendet ein Bild als Text-Link an den Browser.
func (c *WebChannel) SendPhoto(chatID string, imageURL string, caption string) error {
	text := imageURL
	if caption != "" {
		text = caption + "\n" + imageURL
	}
	return c.Send(chatID, text)
}

// TypingIndicator sendet einen Tipp-Indikator an den Browser.
func (c *WebChannel) TypingIndicator(chatID string) {
	if v, ok := c.conns.Load(chatID); ok {
		v.(*wsConn).writeJSON(WsOutgoing{Type: "typing"}) //nolint:errcheck
	}
}

// SendChunk sendet einen Streaming-Chunk an eine WebSocket-Verbindung.
// Sonderfälle (via Präfix-Konvention):
//   - "\x00typing"      → Typing-Indikator-Frame
//   - "\x00audio:<b64>" → Audio-Frame (OGG/Opus, base64-kodiert)
func (c *WebChannel) SendChunk(chatID, chunk string) {
	if v, ok := c.conns.Load(chatID); ok {
		wc := v.(*wsConn)
		switch {
		case chunk == "\x00typing":
			wc.writeJSON(WsOutgoing{Type: "typing"}) //nolint:errcheck
		case len(chunk) > 7 && chunk[:7] == "\x00audio:":
			wc.writeJSON(WsOutgoing{Type: "audio", Data: chunk[7:], Mime: "audio/ogg"}) //nolint:errcheck
		case len(chunk) > 12 && chunk[:12] == "\x00transcript:":
			wc.writeJSON(WsOutgoing{Type: "transcript", Text: chunk[12:]}) //nolint:errcheck
		default:
			wc.writeJSON(WsOutgoing{Type: "chunk", Text: chunk}) //nolint:errcheck
		}
	}
}

// HandleConnection wird vom Dashboard-Server aufgerufen wenn ein WebSocket-Upgrade erfolgt.
// Verwaltet eine einzelne Verbindung komplett: Upgrade → Loop → Cleanup.
func (c *WebChannel) HandleConnection(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebChannel] Upgrade-Fehler: %v", err)
		return
	}

	// Eindeutige ConnID
	connID := fmt.Sprintf("web-%d-%d", time.Now().UnixNano(), rand.Intn(99999))
	wc := &wsConn{conn: conn}
	c.conns.Store(connID, wc)
	defer func() {
		c.conns.Delete(connID)
		conn.Close()
		log.Printf("[WebChannel] Verbindung getrennt: %s", connID)
	}()

	log.Printf("[WebChannel] Neue Verbindung: %s", connID)

	// Ping-Pong Keep-Alive: Pong-Handler setzt nur Read-Deadline zurück (kein Write nötig)
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-pingTicker.C:
				if v, ok := c.conns.Load(connID); ok {
					v.(*wsConn).writeMessage(websocket.PingMessage, nil) //nolint:errcheck
				} else {
					return
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Großzügiges Timeout: 120s – genug für lange AI-Antworten + Ping/Pong
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))

		var raw json.RawMessage
		if err := conn.ReadJSON(&raw); err != nil {
			if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				return
			}
			log.Printf("[WebChannel] Read-Fehler: %v", err)
			return
		}

		var frame wsIncoming
		if err := json.Unmarshal(raw, &frame); err != nil {
			log.Printf("[WebChannel] Frame-Parse-Fehler: %v", err)
			continue
		}

		log.Printf("[WebChannel] Frame empfangen: type=%s connID=%s", frame.Type, connID)

		msg, err := c.buildMessage(connID, frame)
		if err != nil {
			log.Printf("[WebChannel] buildMessage-Fehler: %v", err)
			wc.writeJSON(WsOutgoing{Type: "error", Text: err.Error()}) //nolint:errcheck
			continue
		}

		if c.streamHandler == nil {
			log.Printf("[WebChannel] WARNUNG: kein streamHandler gesetzt!")
			wc.writeJSON(WsOutgoing{Type: "error", Text: "Kein Agent verbunden."}) //nolint:errcheck
			continue
		}

		// Streaming in eigener Goroutine – non-blocking
		go func(m Message, connID string) {
			log.Printf("[WebChannel] streamHandler gestartet für %s", connID)
			sendChunk := func(chunk string) {
				c.SendChunk(connID, chunk)
			}
			c.streamHandler(ctx, m, sendChunk)
			log.Printf("[WebChannel] streamHandler abgeschlossen für %s", connID)
			// Stream-Ende signalisieren
			if v, ok := c.conns.Load(connID); ok {
				v.(*wsConn).writeJSON(WsOutgoing{Type: "done"}) //nolint:errcheck
			}
		}(msg, connID)
	}
}

// buildMessage konvertiert einen eingehenden WebSocket-Frame in eine channels.Message.
func (c *WebChannel) buildMessage(connID string, frame wsIncoming) (Message, error) {
	msg := Message{
		ChannelID: "web",
		ChatID:    connID,
		SenderID:  connID,
		UserName:  "Web-User",
	}

	switch frame.Type {
	case "text":
		msg.Type = MessageTypeText
		msg.Text = frame.Text

	case "voice":
		if frame.Data == "" {
			return msg, fmt.Errorf("voice: keine Daten")
		}
		data, err := base64.StdEncoding.DecodeString(frame.Data)
		if err != nil {
			// Versuche RawStdEncoding (ohne Padding)
			data, err = base64.RawStdEncoding.DecodeString(frame.Data)
			if err != nil {
				return msg, fmt.Errorf("voice: base64-Decode-Fehler: %v", err)
			}
		}
		msg.Type = MessageTypeVoice
		msg.VoiceData = data

	case "image":
		if frame.Data == "" {
			return msg, fmt.Errorf("image: keine Daten")
		}
		// base64-Daten → temporäre Datei
		data, err := base64.StdEncoding.DecodeString(frame.Data)
		if err != nil {
			data, err = base64.RawStdEncoding.DecodeString(frame.Data)
			if err != nil {
				return msg, fmt.Errorf("image: base64-Decode-Fehler: %v", err)
			}
		}
		ext := ".jpg"
		if frame.Mime == "image/png" {
			ext = ".png"
		} else if frame.Mime == "image/gif" {
			ext = ".gif"
		} else if frame.Mime == "image/webp" {
			ext = ".webp"
		}
		tmpPath := filepath.Join(c.tempDir, fmt.Sprintf("fluxweb-%d%s", time.Now().UnixNano(), ext))
		if err := os.WriteFile(tmpPath, data, 0600); err != nil {
			return msg, fmt.Errorf("image: konnte Datei nicht schreiben: %v", err)
		}
		msg.Type = MessageTypeImage
		msg.MediaPath = tmpPath

	default:
		return msg, fmt.Errorf("unbekannter Frame-Typ: %s", frame.Type)
	}

	return msg, nil
}
