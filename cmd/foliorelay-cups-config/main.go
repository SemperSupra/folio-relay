package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	frprinter "github.com/SemperSupra/folio-relay/internal/printer"
)

const (
	expectedPort = 8634
	expectedPath = "/printers/FolioRelay"
)

func main() {
	identityPath := flag.String("identity-file", "", "durable FolioRelay printer identity")
	cupsdTemplate := flag.String("cupsd-template", "", "cupsd.conf template")
	printersTemplate := flag.String("printers-template", "", "printers.conf template")
	ppdSource := flag.String("ppd-source", "", "FolioRelay PPD source")
	outputRoot := flag.String("output-root", "", "runtime CUPS ServerRoot")
	flag.Parse()

	if err := render(*identityPath, *cupsdTemplate, *printersTemplate, *ppdSource, *outputRoot); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(2)
	}
}

func render(identityPath, cupsdTemplate, printersTemplate, ppdSource, outputRoot string) error {
	for label, value := range map[string]string{
		"identity-file": identityPath,
		"cupsd-template": cupsdTemplate,
		"printers-template": printersTemplate,
		"ppd-source": ppdSource,
		"output-root": outputRoot,
	} {
		if value == "" {
			return fmt.Errorf("%s is required", label)
		}
	}

	identity, err := loadIdentity(identityPath)
	if err != nil {
		return err
	}
	if identity.Scheme != "ipp" {
		return errors.New("MVP CUPS projector requires ipp canonical URI")
	}
	if identity.Port != expectedPort {
		return fmt.Errorf("MVP CUPS projector requires public port %d", expectedPort)
	}
	if identity.ResourcePath != expectedPath {
		return fmt.Errorf("MVP CUPS projector requires resource path %s", expectedPath)
	}
	if strings.Contains(identity.Host, ":") {
		return errors.New("MVP CUPS projector does not yet admit a literal IPv6 host")
	}

	cupsdRaw, err := os.ReadFile(cupsdTemplate)
	if err != nil {
		return fmt.Errorf("read cupsd template: %w", err)
	}
	printersRaw, err := os.ReadFile(printersTemplate)
	if err != nil {
		return fmt.Errorf("read printers template: %w", err)
	}
	ppdRaw, err := os.ReadFile(ppdSource)
	if err != nil {
		return fmt.Errorf("read PPD source: %w", err)
	}

	cupsd, err := replaceRequired(string(cupsdRaw), "__FOLIORELAY_PUBLIC_HOST__", identity.Host)
	if err != nil {
		return fmt.Errorf("render cupsd template: %w", err)
	}
	printers, err := replaceExactlyOnce(string(printersRaw), "__FOLIORELAY_PRINTER_UUID__", identity.PrinterUUID)
	if err != nil {
		return fmt.Errorf("render printers template: %w", err)
	}

	if err := os.MkdirAll(filepath.Join(outputRoot, "ppd"), 0o700); err != nil {
		return fmt.Errorf("create runtime ServerRoot: %w", err)
	}
	for _, item := range []struct {
		path string
		data []byte
		mode os.FileMode
	}{
		{filepath.Join(outputRoot, "cupsd.conf"), []byte(cupsd), 0o600},
		{filepath.Join(outputRoot, "printers.conf"), []byte(printers), 0o600},
		{filepath.Join(outputRoot, "ppd", "FolioRelay.ppd"), ppdRaw, 0o600},
	} {
		if err := writeAtomic(item.path, item.data, item.mode); err != nil {
			return err
		}
	}
	return nil
}

func loadIdentity(path string) (frprinter.Identity, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return frprinter.Identity{}, fmt.Errorf("read printer identity: %w", err)
	}
	var identity frprinter.Identity
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&identity); err != nil {
		return frprinter.Identity{}, fmt.Errorf("decode printer identity: %w", err)
	}
	if err := identity.Validate(); err != nil {
		return frprinter.Identity{}, fmt.Errorf("validate printer identity: %w", err)
	}
	return identity, nil
}

func replaceRequired(input, marker, value string) (string, error) {
	if strings.Count(input, marker) < 1 {
		return "", fmt.Errorf("expected at least one %s marker", marker)
	}
	return strings.ReplaceAll(input, marker, value), nil
}

func replaceExactlyOnce(input, marker, value string) (string, error) {
	if strings.Count(input, marker) != 1 {
		return "", fmt.Errorf("expected exactly one %s marker", marker)
	}
	return strings.Replace(input, marker, value, 1), nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".foliorelay-cups-*")
	if err != nil {
		return fmt.Errorf("create %s staging file: %w", filepath.Base(path), err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		return fmt.Errorf("chmod %s staging file: %w", filepath.Base(path), err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write %s staging file: %w", filepath.Base(path), err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("fsync %s staging file: %w", filepath.Base(path), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s staging file: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("commit %s: %w", filepath.Base(path), err)
	}
	cleanup = false
	return nil
}
