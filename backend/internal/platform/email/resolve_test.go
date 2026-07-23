package email

import (
	"testing"
)

func TestResolveReturnsSMTPSenderWhenConfigured(t *testing.T) {
	sender, err := Resolve(SMTPConfig{Host: "mail.smtp2go.com", Port: "2525", From: "a@b.test"})
	if err != nil {
		t.Fatalf("configured smtp: %v", err)
	}
	if _, ok := sender.(*SMTPSender); !ok {
		t.Fatalf("expected an SMTPSender, got %T", sender)
	}
}

// An unusable SMTP configuration must be an error, not a quiet downgrade to the
// logging sender. A typo'd relay host in production previously left the service
// healthy while posting reset tokens into the log.
func TestResolveRejectsInvalidSMTP(t *testing.T) {
	sender, err := Resolve(SMTPConfig{Host: "mail.smtp2go.com", Port: "", From: ""})
	if err == nil {
		t.Fatal("expected an error for an incomplete SMTP configuration")
	}
	if sender != nil {
		t.Fatal("a broken SMTP configuration must not fall back to a sender")
	}

}

// The TLS switch exists for a local mail catcher that does not terminate TLS.
// It must never become a way to put a relay password on the wire in the clear,
// so a username with TLS disabled is refused outright.
func TestSenderRefusesCredentialsWithoutTLS(t *testing.T) {
	_, err := NewSMTPSender(SMTPConfig{
		Host: "mailpit", Port: "1025", From: "a@b.test",
		Username: "someone", Password: "secret", TLS: false,
	})
	if err == nil {
		t.Fatal("expected a refusal when credentials would be sent without TLS")
	}

	// The same credentials with TLS on are fine.
	if _, err := NewSMTPSender(SMTPConfig{
		Host: "mail.smtp2go.com", Port: "2525", From: "a@b.test",
		Username: "someone", Password: "secret", TLS: true,
	}); err != nil {
		t.Fatalf("credentials with TLS: %v", err)
	}

	// And an unauthenticated local relay with TLS off is the development case.
	if _, err := NewSMTPSender(SMTPConfig{
		Host: "mailpit", Port: "1025", From: "a@b.test", TLS: false,
	}); err != nil {
		t.Fatalf("unauthenticated local relay: %v", err)
	}
}
