package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	frprinter "github.com/SemperSupra/folio-relay/internal/printer"
)

func TestRenderProjectsCanonicalIdentity(t *testing.T) {
	root := t.TempDir()
	identity := frprinter.Identity{
		DisplayName: "FolioRelay",
		PrinterUUID: "urn:uuid:12345678-1234-4234-8234-123456789abc",
		Scheme: "ipp",
		Host: "foliorelay.local",
		Port: 8634,
		ResourcePath: "/printers/FolioRelay",
	}
	raw, err := json.Marshal(identity)
	if err != nil { t.Fatal(err) }
	identityFile := filepath.Join(root, "identity.json")
	if err := os.WriteFile(identityFile, raw, 0o600); err != nil { t.Fatal(err) }
	cupsdTemplate := filepath.Join(root, "cupsd.conf")
	printersTemplate := filepath.Join(root, "printers.conf")
	ppd := filepath.Join(root, "FolioRelay.ppd")
	if err := os.WriteFile(cupsdTemplate, []byte("ServerName __FOLIORELAY_PUBLIC_HOST__\nServerAlias __FOLIORELAY_PUBLIC_HOST__\n"), 0o600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(printersTemplate, []byte("<Printer FolioRelay>\nUUID __FOLIORELAY_PRINTER_UUID__\nType 4100\n</Printer>\n"), 0o600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(ppd, []byte("*PPD-Adobe: \"4.3\"\n"), 0o600); err != nil { t.Fatal(err) }

	out := filepath.Join(root, "runtime")
	if err := render(identityFile, cupsdTemplate, printersTemplate, ppd, out); err != nil { t.Fatal(err) }

	cupsd, err := os.ReadFile(filepath.Join(out, "cupsd.conf"))
	if err != nil { t.Fatal(err) }
	if string(cupsd) != "ServerName foliorelay.local\nServerAlias foliorelay.local\n" { t.Fatalf("unexpected cupsd: %q", cupsd) }
	printers, err := os.ReadFile(filepath.Join(out, "printers.conf"))
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(printers), identity.PrinterUUID) { t.Fatalf("UUID not projected: %s", printers) }
	if !strings.Contains(string(printers), "Type 4100") {
		t.Fatalf("printer type not preserved: %s", printers)
	}
	if _, err := os.Stat(filepath.Join(out, "ppd", "FolioRelay.ppd")); err != nil { t.Fatal(err) }
}

func TestRenderRejectsSplitPublicEndpoint(t *testing.T) {
	root := t.TempDir()
	identity := frprinter.Identity{
		DisplayName: "FolioRelay",
		PrinterUUID: "urn:uuid:12345678-1234-4234-8234-123456789abc",
		Scheme: "ipp",
		Host: "foliorelay.local",
		Port: 631,
		ResourcePath: "/printers/FolioRelay",
	}
	raw, _ := json.Marshal(identity)
	identityFile := filepath.Join(root, "identity.json")
	_ = os.WriteFile(identityFile, raw, 0o600)
	for _, name := range []string{"cupsd", "printers", "ppd"} {
		_ = os.WriteFile(filepath.Join(root, name), []byte("__FOLIORELAY_PUBLIC_HOST__ __FOLIORELAY_PRINTER_UUID__"), 0o600)
	}
	err := render(identityFile, filepath.Join(root, "cupsd"), filepath.Join(root, "printers"), filepath.Join(root, "ppd"), filepath.Join(root, "out"))
	if err == nil || !strings.Contains(err.Error(), "public port 8634") { t.Fatalf("expected port rejection, got %v", err) }
}

func TestReplaceRequiredReplacesAllAndFailsClosed(t *testing.T) {
	got, err := replaceRequired("a __M__ b __M__", "__M__", "v")
	if err != nil { t.Fatal(err) }
	if got != "a v b v" { t.Fatalf("unexpected replacement: %q", got) }
	if _, err := replaceRequired("no marker", "__M__", "v"); err == nil {
		t.Fatal("expected missing-marker failure")
	}
}

func TestReplaceExactlyOnceFailsClosed(t *testing.T) {
	for _, input := range []string{"", "x __M__ y __M__"} {
		if _, err := replaceExactlyOnce(input, "__M__", "v"); err == nil {
			t.Fatalf("expected marker-count failure for %q", input)
		}
	}
}


func TestFolioRelayPrinterTypeIsBWAndSmallMedia(t *testing.T) {
	const (
		cupsPTypeBW    = 0x0004
		cupsPTypeSmall = 0x1000
	)
	if got := cupsPTypeBW | cupsPTypeSmall; got != 4100 {
		t.Fatalf("CUPS printer type contract drifted: got %d", got)
	}
}
