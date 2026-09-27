package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SemperSupra/folio-relay/internal/ingress"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: foliorelay-ingress-probe FILE")
		os.Exit(64)
	}

	source := ingress.Source{
		Substrate:  getenvRequired("FOLIORELAY_SUBSTRATE"),
		InstanceID: getenvRequired("FOLIORELAY_SUBSTRATE_INSTANCE"),
		JobID:      firstNonEmpty(os.Getenv("FOLIORELAY_SOURCE_JOB_ID"), os.Getenv("IPP_JOB_ID")),
	}
	if source.Substrate == "" || source.InstanceID == "" || source.JobID == "" {
		fmt.Fprintln(os.Stderr, "missing ingress source identity")
		os.Exit(64)
	}

	receipt, err := ingress.PrepareFile(source, os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	encoded, err := json.Marshal(receipt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if dir := os.Getenv("FOLIORELAY_RECEIPT_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		name := receipt.Command.IdempotencyKey
		for _, c := range []string{":", "/", "\\"} {
			name = replaceAll(name, c, "_")
		}
		path := filepath.Join(dir, name+".json")
		if err := writeExclusiveOrMatch(path, encoded); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	os.Stdout.Write(encoded)
	os.Stdout.Write([]byte("\n"))
}

func getenvRequired(name string) string {
	return os.Getenv(name)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func replaceAll(value, old, replacement string) string {
	for {
		next := stringReplace(value, old, replacement)
		if next == value {
			return value
		}
		value = next
	}
}

func stringReplace(value, old, replacement string) string {
	if old == "" {
		return value
	}
	out := make([]byte, 0, len(value))
	for {
		i := index(value, old)
		if i < 0 {
			out = append(out, value...)
			return string(out)
		}
		out = append(out, value[:i]...)
		out = append(out, replacement...)
		value = value[i+len(old):]
	}
}

func index(value, sub string) int {
	for i := 0; i+len(sub) <= len(value); i++ {
		if value[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func writeExclusiveOrMatch(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if _, werr := f.Write(data); werr != nil {
			f.Close()
			return werr
		}
		return f.Close()
	}
	if !errors.Is(err, os.ErrExist) {
		return err
	}
	existing, rerr := os.ReadFile(path)
	if rerr != nil {
		return rerr
	}
	if string(existing) != string(data) {
		return fmt.Errorf("idempotency conflict for existing receipt %s", path)
	}
	return nil
}
