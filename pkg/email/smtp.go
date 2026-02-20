package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
)

// Sender sendet E-Mails über SMTP (STARTTLS oder SSL/TLS).
// Provider-agnostisch: Gmail, Outlook, Strato, jeder Standard-SMTP-Server.
type Sender struct {
	host string
	port string
	user string
	pass string
	from string
}

// Message enthält alle Felder einer ausgehenden E-Mail.
type Message struct {
	To      string // Empfänger-Adresse (z.B. "max@example.com")
	Subject string // Betreff
	Body    string // Nachrichtentext (plain text, UTF-8)
}

// NewSender erstellt einen neuen SMTP-Sender.
// from ist die Absender-Adresse – leer = user-Adresse wird verwendet.
// port leer → Standardwert 587 (STARTTLS).
func NewSender(host, port, user, pass, from string) *Sender {
	if from == "" {
		from = user
	}
	if port == "" {
		port = "587"
	}
	return &Sender{host: host, port: port, user: user, pass: pass, from: from}
}

// IsConfigured gibt true zurück wenn alle Pflichtfelder gesetzt sind.
func (s *Sender) IsConfigured() bool {
	return s != nil && s.host != "" && s.user != "" && s.pass != ""
}

// From gibt die konfigurierte Absender-Adresse zurück.
func (s *Sender) From() string {
	if s == nil {
		return ""
	}
	return s.from
}

// Send sendet eine E-Mail.
// Port 465  → Direktes TLS (SMTPS)
// Port 587+ → STARTTLS
func (s *Sender) Send(msg Message) error {
	if !s.IsConfigured() {
		return fmt.Errorf("SMTP nicht konfiguriert – trage SMTP_HOST, SMTP_USER und SMTP_PASSWORD in den Integrationen ein")
	}
	addr := net.JoinHostPort(s.host, s.port)
	portNum, _ := strconv.Atoi(s.port)

	// E-Mail-Inhalt als RFC 2822-konformer String
	body := buildRFC822(s.from, msg.To, msg.Subject, msg.Body)

	if portNum == 465 {
		return s.sendTLS(addr, body, msg.To)
	}
	return s.sendSTARTTLS(addr, body, msg.To)
}

// sendSTARTTLS stellt eine STARTTLS-Verbindung her (Standard: Port 587).
// Geeignet für Gmail (smtp.gmail.com:587), Outlook (smtp.office365.com:587), etc.
func (s *Sender) sendSTARTTLS(addr, body, to string) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("SMTP-Verbindung zu %s fehlgeschlagen: %w", addr, err)
	}
	defer c.Close()

	tlsCfg := &tls.Config{ServerName: s.host}
	if err := c.StartTLS(tlsCfg); err != nil {
		return fmt.Errorf("STARTTLS-Handshake fehlgeschlagen: %w", err)
	}

	auth := smtp.PlainAuth("", s.user, s.pass, s.host)
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("SMTP-Authentifizierung fehlgeschlagen (falsches Passwort oder App-Passwort?): %w", err)
	}

	return s.writeMessage(c, body, to)
}

// sendTLS stellt eine direkte TLS-Verbindung her (SMTPS, Port 465).
func (s *Sender) sendTLS(addr, body, to string) error {
	tlsCfg := &tls.Config{ServerName: s.host}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("TLS-Verbindung zu %s fehlgeschlagen: %w", addr, err)
	}
	defer conn.Close()

	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("SMTP-Client konnte nicht initialisiert werden: %w", err)
	}
	defer c.Close()

	auth := smtp.PlainAuth("", s.user, s.pass, s.host)
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("SMTP-Authentifizierung fehlgeschlagen: %w", err)
	}

	return s.writeMessage(c, body, to)
}

// writeMessage schreibt Absender, Empfänger und Body in die SMTP-Session.
func (s *Sender) writeMessage(c *smtp.Client, body, to string) error {
	if err := c.Mail(s.from); err != nil {
		return fmt.Errorf("MAIL FROM fehlgeschlagen: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("RCPT TO fehlgeschlagen (ungültige Adresse?): %w", err)
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA-Befehl fehlgeschlagen: %w", err)
	}
	defer wc.Close()

	if _, err := fmt.Fprint(wc, body); err != nil {
		return fmt.Errorf("E-Mail-Body konnte nicht übertragen werden: %w", err)
	}
	return nil
}

// buildRFC822 erstellt den E-Mail-Inhalt im RFC 2822-Format.
func buildRFC822(from, to, subject, body string) string {
	var sb strings.Builder
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + to + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	sb.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)
	return sb.String()
}
