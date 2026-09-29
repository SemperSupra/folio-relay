package printer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreatePersistsCanonicalIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "printer.json")
	first, err := LoadOrCreate(path, "FolioRelay", "Office", "ipp://foliorelay.local:8634/printers/FolioRelay")
	if err != nil {
		t.Fatal(err)
	}
	if first.URI() != "ipp://foliorelay.local:8634/printers/FolioRelay" {
		t.Fatalf("unexpected URI: %s", first.URI())
	}
	if first.PrinterUUID == "" {
		t.Fatal("printer UUID was not generated")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("identity permissions = %o, want 600", info.Mode().Perm())
	}

	second, err := LoadOrCreate(path, "Changed bootstrap", "Elsewhere", "ipp://different.local:9999/printers/Other")
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("existing durable identity was replaced: first=%+v second=%+v", first, second)
	}
}

func TestNewRejectsNonPublicIdentity(t *testing.T) {
	for _, raw := range []string{
		"ipp://localhost:631/printers/FolioRelay",
		"ipp://127.0.0.1:631/printers/FolioRelay",
		"ipp://0.0.0.0:631/printers/FolioRelay",
		"http://foliorelay.local/printers/FolioRelay",
		"ipp://foliorelay.local/",
	} {
		if _, err := New("FolioRelay", "", raw); err == nil {
			t.Fatalf("expected rejection for %s", raw)
		}
	}
}
