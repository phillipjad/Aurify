package config

import "testing"

// Outside dev mode a missing relay must stop the process. The failure it
// prevents is silent: the API would otherwise start, look healthy, and write
// live password-reset links into the application log.
func TestValidateRequiresSMTPWhenMissing(t *testing.T) {
	cfg := Config{
		SMTP: SMTPConfig{Host: "", From: "a@b.test"},
		Auth: AuthConfig{SigningKeySeed: "c2VlZA=="},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error when SMTP host is missing when missing")
	}

	cfg.SMTP.Host = "mail.smtp2go.com"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("configured SMTP when missing: %v", err)
	}
}

func TestValidateRequiresFromAddressWhenMissing(t *testing.T) {
	cfg := Config{SMTP: SMTPConfig{Host: "mail.smtp2go.com", From: ""}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error when the from address is missing")
	}
}

// A default-loaded config (no SMTP set) must not be considered valid, or the
// zero-configuration developer path would also be the production path.
func TestLoadedDefaultsAreInvalidWithoutDevMode(t *testing.T) {
	t.Setenv("AURIFY_SMTP_HOST", "")
	if err := Load().Validate(); err == nil {
		t.Fatal("default configuration must not validate when missing")
	}
}

// An ephemeral signing key silently signs every user out on restart, and makes
// two instances reject each other's tokens. It is a dev convenience only.
func TestValidateRequiresSigningKeyWhenMissing(t *testing.T) {
	cfg := Config{
		SMTP: SMTPConfig{Host: "mail.smtp2go.com", From: "a@b.test"},
		Auth: AuthConfig{SigningKeySeed: ""},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error when the signing key is missing when missing")
	}

	cfg.Auth.SigningKeySeed = "c2VlZA=="
	if err := cfg.Validate(); err != nil {
		t.Fatalf("configured signing key: %v", err)
	}
}
