package transporttls

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestEnsureSelfSignedPersistsExactMaterial(t *testing.T) {
	now := time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC)
	dir := t.TempDir()

	first, err := EnsureSelfSigned(dir, "FolioRelay.Test.", now)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Generated {
		t.Fatal("first materialization must report generated")
	}
	if first.CertFile != first.KeyFile {
		t.Fatalf("self-signed material should use one atomic PEM file: %+v", first)
	}
	info, err := os.Stat(first.CertFile)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("TLS material mode = %o, want 600", got)
		}
	}
	if first.FingerprintSHA256 == "" {
		t.Fatal("missing TLS fingerprint")
	}

	pair, err := tls.LoadX509KeyPair(first.CertFile, first.KeyFile)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := cert.VerifyHostname("foliorelay.test"); err != nil {
		t.Fatal(err)
	}

	second, err := EnsureSelfSigned(dir, "foliorelay.test", now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if second.Generated {
		t.Fatal("existing valid material must be reused")
	}
	if second.FingerprintSHA256 != first.FingerprintSHA256 {
		t.Fatalf("fingerprint drifted: %s != %s", second.FingerprintSHA256, first.FingerprintSHA256)
	}
}

func TestValidatePairRejectsWrongHost(t *testing.T) {
	now := time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC)
	material, err := EnsureSelfSigned(t.TempDir(), "foliorelay.test", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidatePair(material.CertFile, material.KeyFile, "other.test", now); err == nil {
		t.Fatal("wrong TLS host must fail closed")
	}
}
