// Package email delivers Aurify's transactional mail (address verification and
// password reset).
//
// The SMTP adapter targets SMTP2GO but speaks plain SMTP, so any provider works
// by changing configuration. Speaking SMTP rather than a vendor HTTP API keeps
// the provider swappable and avoids a dependency: net/smtp is standard library.
package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// SMTPSender delivers mail through an SMTP relay.
type SMTPSender struct {
	addr string // host:port
	host string // host alone, for TLS verification
	auth smtp.Auth
	from string
}

// SMTPConfig is the connection detail for the relay.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// NewSMTPSender builds an SMTP-backed sender. It fails fast on incomplete
// configuration rather than discovering the problem when the first user tries
// to sign up.
func NewSMTPSender(cfg SMTPConfig) (*SMTPSender, error) {
	if cfg.Host == "" || cfg.Port == "" || cfg.From == "" {
		return nil, errors.New("email: smtp sender requires a host, port and from address")
	}
	return &SMTPSender{
		addr: net.JoinHostPort(cfg.Host, cfg.Port),
		host: cfg.Host,
		auth: smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host),
		from: cfg.From,
	}, nil
}

// ErrHeaderInjection is returned when a recipient or subject contains a line
// break. Such a value is rejected rather than sanitised: it would also be handed
// to the SMTP RCPT command, where a newline is command injection rather than
// merely a malformed header, and a silently repaired address just fails to
// deliver with no explanation.
var ErrHeaderInjection = errors.New("email: header value contains a line break")

// Send delivers one message.
//
// STARTTLS is mandatory, not opportunistic: smtp.PlainAuth refuses to hand
// credentials to an unencrypted connection, and we would rather fail to send
// than transmit the relay password in the clear.
func (s *SMTPSender) Send(ctx context.Context, to, subject, body string) error {
	if containsLineBreak(to) || containsLineBreak(subject) {
		return ErrHeaderInjection
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("email: dial %s: %w", s.addr, err)
	}

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("email: smtp handshake: %w", err)
	}
	defer func() { _ = client.Close() }()

	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("email: relay does not offer STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("email: starttls: %w", err)
	}
	if err := client.Auth(s.auth); err != nil {
		return fmt.Errorf("email: auth: %w", err)
	}

	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("email: set sender: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("email: set recipient: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: open data: %w", err)
	}
	if _, err := w.Write([]byte(buildMessage(s.from, to, subject, body))); err != nil {
		_ = w.Close()
		return fmt.Errorf("email: write message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: close data: %w", err)
	}
	return client.Quit()
}

// buildMessage assembles a minimal RFC 5322 message.
//
// Header values are stripped of CR and LF as defence in depth. Send already
// rejects such values, so reaching the stripping here means a caller bypassed
// that check; the message must still come out with exactly the headers we wrote
// rather than whatever the caller smuggled in.
func buildMessage(from, to, subject, body string) string {
	var b strings.Builder
	b.WriteString("From: " + sanitizeHeader(from) + "\r\n")
	b.WriteString("To: " + sanitizeHeader(to) + "\r\n")
	b.WriteString("Subject: " + sanitizeHeader(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}

func sanitizeHeader(v string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(v)
}

func containsLineBreak(v string) bool {
	return strings.ContainsAny(v, "\r\n")
}

// LogSender writes mail to the application log instead of sending it. It backs
// local development, where there is no relay configured and the verification
// link simply needs to be reachable by a developer.
//
// It logs the body, which contains a live single-use token, so it must never be
// selected in an environment with real users.
type LogSender struct{}

// Send records the message at info level.
func (LogSender) Send(ctx context.Context, to, subject, body string) error {
	slog.InfoContext(ctx, "email not sent (no SMTP configured), logging instead",
		"to", to, "subject", subject, "body", body)
	return nil
}
