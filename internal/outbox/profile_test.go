package outbox

import (
	"errors"
	"strings"
	"testing"
)

func emailProfile() Profile {
	return Profile{ID:"icloud-home",Provider:"icloud",Host:"smtp.mail.me.com",Port:587,
		TLS:"starttls_required",Auth:"app_password",Username:"owner@icloud.com",From:"owner@icloud.com",
		SecretRef:"/run/secrets/foliorelay-email/icloud-home",Enabled:true}
}

func TestProviderProfilesOnlySecureKnownBindings(t *testing.T) {
	base:=emailProfile()
	if err:=base.Validate();err!=nil {t.Fatal(err)}
	g:=base
	g.Provider="google";g.Host="smtp.gmail.com";g.Auth="xoauth2"
	if err:=g.Validate();err!=nil {t.Fatal(err)}
	g.Port=465;g.TLS="implicit_tls"
	if err:=g.Validate();err!=nil {t.Fatal(err)}
	m:=base
	m.Provider="microsoft";m.Host="smtp.office365.com";m.Auth="xoauth2"
	if err:=m.Validate();err!=nil {t.Fatal(err)}
	c:=base
	c.Provider="custom";c.Host="mail.example.net";c.Port=2465;c.TLS="implicit_tls"
	if err:=c.Validate();err!=nil {t.Fatal(err)}
	disabled:=base
	disabled.Enabled=false;disabled.SecretRef=""
	if err:=disabled.Validate();err!=nil {t.Fatal(err)}
}

func TestProfileRejectsInsecureAndInjectedSettings(t *testing.T) {
	for _,change:=range []func(*Profile){
		func(p *Profile){p.TLS="none"},
		func(p *Profile){p.Port=0},
		func(p *Profile){p.Port=250000},
		func(p *Profile){p.Host="smtp.evil.com"},
		func(p *Profile){p.Provider="unknown"},
		func(p *Profile){p.Auth="plaintext"},
		func(p *Profile){p.Auth="xoauth2"},
		func(p *Profile){p.Username="sender@icloud.com\r\nBcc:someone@example.com"},
		func(p *Profile){p.From="sender@icloud.com,person@example.com"},
		func(p *Profile){p.SecretRef="../../password"},
		func(p *Profile){p.SecretRef="/run/secrets/foliorelay-email/../secret"},
		func(p *Profile){p.SecretRef="/run/secrets/foliorelay-email/.hidden"},
		func(p *Profile){p.SecretRef=""},
		func(p *Profile){p.ID="../bad"},
		func(p *Profile){p.Host="localhost";p.Provider="custom"},
		func(p *Profile){p.Host="127.0.0.1";p.Provider="custom"},
		func(p *Profile){p.Host="mail..example.com";p.Provider="custom"},
	} {
		p:=emailProfile();change(&p)
		if err:=p.Validate();!errors.Is(err,ErrInvalidProfile) {
			t.Errorf("accepted %+v error=%v",p,err)
		}
	}
}

func TestProfileDoesNotContainCredentials(t *testing.T) {
	p:=emailProfile()
	if strings.Contains(p.SecretRef,"password") {t.Fatal("unexpected inline secret")}
	if err:=p.Validate();err!=nil {t.Fatal(err)}
}
