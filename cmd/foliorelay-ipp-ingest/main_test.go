package main

import (
	"os"
	"path/filepath"
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
