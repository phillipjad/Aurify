package email

import (
	"context"
	"fmt"
)

// Sender is the behaviour Resolve returns. It matches ports.EmailSender; it is
// restated here so this package does not depend on the application layer.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Resolve builds the mail sender for a configuration.
//
// There is no fallback. Every environment has a real relay: production uses
// SMTP2GO, development uses Mailpit. An unusable configuration is an error
// rather than a quiet downgrade, since undelivered mail otherwise presents as
// users unable to sign up or recover an account with nothing looking unhealthy.
func Resolve(cfg SMTPConfig) (Sender, error) {
	sender, err := NewSMTPSender(cfg)
	if err != nil {
		return nil, fmt.Errorf("email: smtp configuration is invalid: %w", err)
	}
	return sender, nil
}
