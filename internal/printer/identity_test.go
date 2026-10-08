package printer

import (
	"os"
	"path/filepath"
	"runtime"
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
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o640 {
		t.Fatalf("identity permissions = %o, want 640", info.Mode().Perm())
	}
	if runtime.GOOS != "windows" {
		dirInfo, err := os.Stat(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		if dirInfo.Mode().Perm() != 0o750 {
			t.Fatalf("identity directory permissions = %o, want 750", dirInfo.Mode().Perm())
		}
	}

	second, err := LoadOrCreate(path, "Changed bootstrap", "Elsewhere", "ipp://different.local:9999/printers/Other")
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("existing durable identity was replaced: first=%+v second=%+v", first, second)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadOrCreate(path, "Ignored", "", "ipp://ignored.local:8634/printers/Ignored"); err != nil {
			t.Fatal(err)
		}
		info, _ = os.Stat(path)
		dirInfo, _ := os.Stat(filepath.Dir(path))
		if info.Mode().Perm() != 0o640 || dirInfo.Mode().Perm() != 0o750 {
			t.Fatalf("existing identity permissions were not reconciled: file=%o dir=%o", info.Mode().Perm(), dirInfo.Mode().Perm())
		}
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

func TestIdentityRejectsControlCharactersInHumanFields(t *testing.T) {
	cases := []struct {
		name        string
		displayName string
		location    string
	}{
		{name: "display newline", displayName: "FolioRelay\nServerName attacker"},
		{name: "display tab", displayName: "FolioRelay\tInjected"},
		{name: "location newline", displayName: "FolioRelay", location: "Office\nUUID urn:uuid:00000000-0000-4000-8000-000000000000"},
		{name: "location delete", displayName: "FolioRelay", location: "Office\x7f"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.displayName, tc.location, "ipp://foliorelay.local:8634/printers/FolioRelay"); err == nil {
				t.Fatal("expected control-character rejection")
			}
		})
	}
}
