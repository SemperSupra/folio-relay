package security

import (
	"errors"
	"testing"
)

func TestEmailRecipientAcceptsSingleMailbox(t *testing.T) {
	got, err := ValidateEmailRecipient("user@example.com", EmailRecipientPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Domain != "example.com" {
		t.Fatalf("unexpected domain %q", got.Domain)
	}
}

func TestEmailRecipientRejectsHeaderInjection(t *testing.T) {
	_, err := ValidateEmailRecipient("user@example.com\r\nBcc: attacker@example.net", EmailRecipientPolicy{})
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Fatalf("expected invalid recipient, got %v", err)
	}
}

func TestEmailRecipientRejectsList(t *testing.T) {
	_, err := ValidateEmailRecipient("a@example.com,b@example.com", EmailRecipientPolicy{})
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Fatalf("expected invalid recipient, got %v", err)
	}
}

func TestEmailRecipientRejectsDisplayNameInSingleMailboxField(t *testing.T) {
	_, err := ValidateEmailRecipient("User <user@example.com>", EmailRecipientPolicy{})
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Fatalf("expected invalid recipient, got %v", err)
	}
}

func TestEmailRecipientDomainAllowlist(t *testing.T) {
	policy := EmailRecipientPolicy{AllowedDomains: map[string]struct{}{"example.com": {}}}
	if _, err := ValidateEmailRecipient("user@example.com", policy); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateEmailRecipient("user@example.net", policy); !errors.Is(err, ErrInvalidRecipient) {
		t.Fatalf("expected domain rejection, got %v", err)
	}
}
