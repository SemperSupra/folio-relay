package transporttls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const selfSignedLifetime = 397 * 24 * time.Hour

type Material struct {
	CertFile          string
	KeyFile           string
	FingerprintSHA256 string
	Generated         bool
}

func EnsureSelfSigned(dir, host string, now time.Time) (Material, error) {
	host = normalizeHost(host)
	if dir == "" {
		return Material{}, errors.New("TLS state directory is required")
	}
	if host == "" {
		return Material{}, errors.New("TLS host is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Material{}, fmt.Errorf("create TLS state directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return Material{}, fmt.Errorf("chmod TLS state directory: %w", err)
	}

	path := filepath.Join(dir, "management-tls.pem")
	if _, err := os.Stat(path); err == nil {
		fingerprint, cert, err := ValidatePair(path, path, host, now)
		if err == nil {
			return Material{CertFile: path, KeyFile: path, FingerprintSHA256: fingerprint}, nil
		}
		if cert != nil && now.After(cert.NotAfter) {
			// Expired product-owned material is no longer usable; renew in place
			// while preserving the stable path and canonical host identity.
		} else {
			return Material{}, fmt.Errorf("validate persisted TLS material: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Material{}, fmt.Errorf("stat persisted TLS material: %w", err)
	}

	pemBytes, certDER, err := newSelfSigned(host, now)
	if err != nil {
		return Material{}, err
	}
	if err := atomicWrite(path, pemBytes, 0o600); err != nil {
		return Material{}, err
	}
	sum := sha256.Sum256(certDER)
	return Material{
		CertFile:          path,
		KeyFile:           path,
		FingerprintSHA256: hex.EncodeToString(sum[:]),
		Generated:         true,
	}, nil
}

func ValidatePair(certFile, keyFile, host string, now time.Time) (string, *x509.Certificate, error) {
	host = normalizeHost(host)
	if certFile == "" || keyFile == "" {
		return "", nil, errors.New("TLS certificate and key files are required")
	}
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return "", nil, fmt.Errorf("load TLS certificate/key pair: %w", err)
	}
	if len(pair.Certificate) == 0 {
		return "", nil, errors.New("TLS certificate chain is empty")
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return "", nil, fmt.Errorf("parse TLS leaf certificate: %w", err)
	}
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
		return "", cert, errors.New("TLS leaf certificate is not currently valid")
	}
	if err := cert.VerifyHostname(host); err != nil {
		return "", cert, fmt.Errorf("TLS leaf certificate does not cover %q: %w", host, err)
	}
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:]), cert, nil
}

func newSelfSigned(host string, now time.Time) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate TLS private key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("generate TLS certificate serial: %w", err)
	}
	if serial.Sign() == 0 {
		serial = big.NewInt(1)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(selfSignedLifetime),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create self-signed TLS certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("encode TLS private key: %w", err)
	}
	out := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	out = append(out, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})...)
	return out, der, nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".management-tls-*")
	if err != nil {
		return fmt.Errorf("create TLS staging file: %w", err)
	}
	name := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(name)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return fmt.Errorf("chmod TLS staging file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write TLS staging file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("fsync TLS staging file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close TLS staging file: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("commit TLS material: %w", err)
	}
	cleanup = false
	if dirHandle, err := os.Open(dir); err == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}
	return nil
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}
