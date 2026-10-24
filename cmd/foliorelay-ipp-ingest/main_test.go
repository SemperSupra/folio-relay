package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoundedEnvRejectsControlCharacters(t *testing.T) {
	t.Setenv("X_TEST", "good\nbad")
	if _, err := boundedEnv("X_TEST", 64, true); err == nil {
		t.Fatal("expected control-character rejection")
	}
}

func TestStoreBlobContentAddressed(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.bin")
	content := []byte("folio-relay-ingress-test")
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(root, "store")
	digest1, size1, err := storeBlob(source, store, 1024)
	if err != nil {
		t.Fatal(err)
	}
	digest2, size2, err := storeBlob(source, store, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if digest1 != digest2 || size1 != size2 {
		t.Fatalf("content-addressed replay changed identity: %s/%d vs %s/%d", digest1, size1, digest2, size2)
	}
	target := filepath.Join(store, "sha256", digest1[:2], digest1)
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatal("stored content differs")
	}
}

func TestStoreBlobRejectsOversize(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "large.bin")
	if err := os.WriteFile(source, make([]byte, 65), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := storeBlob(source, filepath.Join(root, "store"), 64); err == nil {
		t.Fatal("expected oversize rejection")
	}
}

func TestPositiveIntEnv(t *testing.T) {
	t.Setenv("IPP_COPIES", "1000000")
	if value, err := positiveIntEnv("IPP_COPIES", 1, 10_000_000); err != nil || value != 1000000 {
		t.Fatalf("adapter should carry bounded value to state authority: value=%d err=%v", value, err)
	}
	t.Setenv("IPP_COPIES", "10000001")
	if _, err := positiveIntEnv("IPP_COPIES", 1, 10_000_000); err == nil {
		t.Fatal("expected absurd copy count to be rejected at adapter bound")
	}
}

func TestStableJobIdentityPrefersIPPJobUUID(t *testing.T) {
	got, err := stableJobIdentity("urn:uuid:abc", "1", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "urn:uuid:abc" {
		t.Fatalf("unexpected identity %q", got)
	}
}

func TestStableJobIdentityRequiresInstanceWhenUUIDMissing(t *testing.T) {
	if _, err := stableJobIdentity("", "1", ""); err == nil {
		t.Fatal("expected substrate instance requirement")
	}
	got, err := stableJobIdentity("", "1", "boot-a")
	if err != nil {
		t.Fatal(err)
	}
	if got != "instance-boot-a/job-1" {
		t.Fatalf("unexpected fallback identity %q", got)
	}
}

func TestNewStateRequestAddsBearerAndIdempotencyHeaders(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "state.token")
	if err := os.WriteFile(tokenFile, []byte("0123456789abcdef0123456789abcdef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	req, err := newStateRequest(
		"http://state:18080",
		[]byte(`{"ok":true}`),
		"ingress/cups/job-1",
		tokenFile,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer 0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected authorization header %q", got)
	}
	if got := req.Header.Get("Idempotency-Key"); got != "ingress/cups/job-1" {
		t.Fatalf("unexpected idempotency header %q", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected content type %q", got)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("request body changed: %q", body)
	}
}

func TestNewStateRequestKeepsLegacyFixtureCompatibilityWithoutToken(t *testing.T) {
	req, err := newStateRequest(
		"http://state:18080",
		[]byte("{}"),
		"legacy-key",
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("unexpected authorization header %q", got)
	}
	if got := req.Header.Get("Idempotency-Key"); got != "" {
		t.Fatalf("legacy fixture unexpectedly requires header %q", got)
	}
}

func TestNewStateRequestRejectsInvalidToken(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "state.token")
	if err := os.WriteFile(tokenFile, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := newStateRequest("http://state:18080", []byte("{}"), "key", tokenFile)
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid token error, got %v", err)
	}
}
