package email

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildMessageHasHeadersAndBody(t *testing.T) {
	msg := buildMessage("from@aurify.test", "to@example.com", "Verify your email", "click here")

	for _, want := range []string{
		"From: from@aurify.test\r\n",
		"To: to@example.com\r\n",
		"Subject: Verify your email\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q:\n%s", want, msg)
		}
	}

	// Headers and body must be separated by a blank line, or the body is parsed
	// as more headers and silently disappears.
	if !strings.Contains(msg, "\r\n\r\nclick here") {
		t.Fatalf("body not separated from headers:\n%s", msg)
	}
}

// Send is the real guard: a line break in a recipient or subject is rejected,
// because that value also reaches the SMTP RCPT command where a newline is
// command injection rather than a merely malformed header.
func TestSendRejectsHeaderInjection(t *testing.T) {
	sender, err := NewSMTPSender(SMTPConfig{Host: "smtp.invalid", Port: "587", From: "a@b.test"})
	if err != nil {
		t.Fatalf("new sender: %v", err)
	}

	if err := sender.Send(
		t.Context(),
		"victim@example.com\r\nBcc: attacker@evil.test",
		"hi",
		"body",
	); !errors.Is(
		err,
		ErrHeaderInjection,
	) {
		t.Fatalf("injected recipient: err = %v, want ErrHeaderInjection", err)
	}
	if err := sender.Send(
		t.Context(),
		"victim@example.com",
		"Subject\nX-Injected: yes",
		"body",
	); !errors.Is(
		err,
		ErrHeaderInjection,
	) {
		t.Fatalf("injected subject: err = %v, want ErrHeaderInjection", err)
	}
}

// Defence in depth: even if a caller reaches buildMessage directly, the message
// must carry exactly the five headers we write and no smuggled extras.
func TestBuildMessageCannotGainHeaderLines(t *testing.T) {
	msg := buildMessage(
		"from@aurify.test",
		"victim@example.com\r\nBcc: attacker@evil.test",
		"Subject\r\nX-Injected: yes",
		"body",
	)

	headers, body, found := strings.Cut(msg, "\r\n\r\n")
	if !found {
		t.Fatalf("no header/body separator:\n%s", msg)
	}
	if body != "body" {
		t.Fatalf("body = %q, want %q", body, "body")
	}

	lines := strings.Split(headers, "\r\n")
	if len(lines) != 5 {
		t.Fatalf("header count = %d, want 5:\n%s", len(lines), headers)
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "Bcc:") || strings.HasPrefix(line, "X-Injected:") {
			t.Fatalf("injected header line survived: %q", line)
		}
	}
}

func TestNewSMTPSenderRequiresConfig(t *testing.T) {
	if _, err := NewSMTPSender(SMTPConfig{}); err == nil {
		t.Fatal("expected an error for empty config")
	}
	if _, err := NewSMTPSender(SMTPConfig{Host: "smtp.test", Port: "587"}); err == nil {
		t.Fatal("expected an error when the from address is missing")
	}
	if _, err := NewSMTPSender(SMTPConfig{Host: "smtp.test", Port: "587", From: "a@b.test"}); err != nil {
		t.Fatalf("valid config: %v", err)
	}
}
