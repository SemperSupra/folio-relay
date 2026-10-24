package outbox

import (
	"errors"
	"fmt"
	"net"
	"path"
	"regexp"
	"strings"

	"github.com/SemperSupra/folio-relay/internal/security"
)

// Profile is non-secret, post-install configuration. Credential material is
// referenced but never returned by this contract or stored in Git.
type Profile struct {
	ID string `json:"id"`
	Provider string `json:"provider"`
	Host string `json:"smtp_host"`
	Port int `json:"smtp_port"`
	TLS string `json:"tls_mode"`
	Auth string `json:"auth_method"`
	Username string `json:"username"`
	From string `json:"from"`
	SecretRef string `json:"secret_ref,omitempty"`
	Enabled bool `json:"enabled"`
}

var (
	ErrInvalidProfile = errors.New("invalid email provider profile")
	hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.-]{0,251}[a-zA-Z0-9]$`)
	secretPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,80}$`)
)

func (p Profile) Validate() error {
	if !idPattern.MatchString(p.ID) || p.Port<1 || p.Port>65535 ||
		(p.TLS!="starttls_required" && p.TLS!="implicit_tls") ||
		(p.Auth!="xoauth2" && p.Auth!="app_password") {
		return ErrInvalidProfile
	}
	if strings.TrimSpace(p.Host)!=p.Host || len(p.Host)<3 ||
		!hostnamePattern.MatchString(p.Host) || strings.Contains(p.Host,"..") ||
		strings.HasSuffix(p.Host,".") || net.ParseIP(p.Host)!=nil ||
		strings.EqualFold(p.Host,"localhost") || strings.HasSuffix(strings.ToLower(p.Host),".localhost") {
		return ErrInvalidProfile
	}
	recipientPolicy:=security.EmailRecipientPolicy{AllowSMTPUTF8:false}
	if _,err:=security.ValidateEmailRecipient(p.From,recipientPolicy);err!=nil { return fmt.Errorf("%w: sender mailbox",ErrInvalidProfile) }
	if _,err:=security.ValidateEmailRecipient(p.Username,recipientPolicy);err!=nil { return fmt.Errorf("%w: username mailbox",ErrInvalidProfile) }
	switch p.Provider {
	case "google":
		if !strings.EqualFold(p.Host,"smtp.gmail.com") ||
			!((p.Port==587 && p.TLS=="starttls_required")||(p.Port==465 && p.TLS=="implicit_tls")) { return ErrInvalidProfile }
	case "icloud":
		if !strings.EqualFold(p.Host,"smtp.mail.me.com") || p.Port!=587 || p.TLS!="starttls_required" || p.Auth!="app_password" { return ErrInvalidProfile }
	case "microsoft":
		if !strings.EqualFold(p.Host,"smtp.office365.com") || p.Port!=587 || p.TLS!="starttls_required" || p.Auth!="xoauth2" { return ErrInvalidProfile }
	case "outlook_com":
		// Consumer Outlook.com has a distinct SMTP endpoint from Microsoft 365.
		if !strings.EqualFold(p.Host,"smtp-mail.outlook.com") || p.Port!=587 || p.TLS!="starttls_required" || p.Auth!="xoauth2" { return ErrInvalidProfile }
	case "custom":
		// Custom DNS/IP resolution, peer validation and authorized egress
		// require a separate runtime security gate before network access.
	default:
		return ErrInvalidProfile
	}
	if p.SecretRef=="" {
		if p.Enabled { return ErrInvalidProfile }
		return nil
	}
	if !strings.HasPrefix(p.SecretRef,"/run/secrets/foliorelay-email/") ||
		path.Clean(p.SecretRef)!=p.SecretRef ||
		!secretPattern.MatchString(path.Base(p.SecretRef)) {
		return ErrInvalidProfile
	}
	return nil
}
