package security

import (
	"errors"
	"net/mail"
	"strings"
	"unicode"
)

var (
	ErrInvalidRecipient = errors.New("invalid email recipient")
	ErrSMTPUTF8Required = errors.New("SMTPUTF8/EAI capability required")
)

type EmailRecipientPolicy struct {
	AllowSMTPUTF8   bool
	AllowedDomains  map[string]struct{}
}

type EmailRecipient struct {
	Original string
	Mailbox  string
	Domain   string
}

func ValidateEmailRecipient(input string, policy EmailRecipientPolicy) (EmailRecipient, error) {
	if input == "" || strings.TrimSpace(input) != input {
		return EmailRecipient{}, ErrInvalidRecipient
	}
	for _, r := range input {
		if r == '\r' || r == '\n' || r == 0 || unicode.IsControl(r) {
			return EmailRecipient{}, ErrInvalidRecipient
		}
	}

	address, err := mail.ParseAddress(input)
	if err != nil {
		return EmailRecipient{}, ErrInvalidRecipient
	}
	// One route leg accepts a mailbox, not a display-name/header fragment.
	if address.Name != "" || address.Address != input {
		return EmailRecipient{}, ErrInvalidRecipient
	}
	at := strings.LastIndexByte(address.Address, '@')
	if at <= 0 || at == len(address.Address)-1 {
		return EmailRecipient{}, ErrInvalidRecipient
	}
	local := address.Address[:at]
	domain := strings.ToLower(address.Address[at+1:])
	if !isASCII(local) || !isASCII(domain) {
		if !policy.AllowSMTPUTF8 {
			return EmailRecipient{}, ErrSMTPUTF8Required
		}
	}
	if len(policy.AllowedDomains) > 0 {
		if _, ok := policy.AllowedDomains[domain]; !ok {
			return EmailRecipient{}, ErrInvalidRecipient
		}
	}
	return EmailRecipient{Original: input, Mailbox: address.Address, Domain: domain}, nil
}

func isASCII(value string) bool {
	for _, r := range value {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}
