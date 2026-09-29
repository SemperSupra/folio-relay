package state

import (
	"path/filepath"
	"testing"
)

func TestDurableEngineRecordsPreserveMetadataAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.frj")
	d, err := OpenDurableEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	cmd := Command{
		Type:                "ingest-artifact",
		AggregateID:         "ingress/pappl/job-7",
		IdempotencyKey:      "ingress/pappl/job-7",
		SemanticFingerprint: "sha256:fingerprint",
		Metadata: map[string]string{
			"artifact_sha256": "0123456789abcdef",
			"media_type":      "application/pdf",
			"artifact_bytes":  "1234",
			"copies":          "1",
		},
	}
	if _, err := d.Apply(cmd, acceptOnce); err != nil {
		t.Fatal(err)
	}

	records := d.Records()
	if len(records) != 1 {
		t.Fatalf("expected one durable record, got %d", len(records))
	}
	if got := records[0].Command.Metadata["media_type"]; got != "application/pdf" {
		t.Fatalf("metadata not retained: %q", got)
	}

	// Records must be defensive copies so a projection cannot mutate authority.
	records[0].Command.Metadata["media_type"] = "mutated"
	if got := d.Records()[0].Command.Metadata["media_type"]; got != "application/pdf" {
		t.Fatalf("projection mutated durable record: %q", got)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}

	d, err = OpenDurableEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	recovered := d.Records()
	if len(recovered) != 1 {
		t.Fatalf("expected one recovered record, got %d", len(recovered))
	}
	if got := recovered[0].Command.Metadata["artifact_bytes"]; got != "1234" {
		t.Fatalf("recovered metadata mismatch: %q", got)
	}
	if got := recovered[0].Command.Metadata["media_type"]; got != "application/pdf" {
		t.Fatalf("recovered media type mismatch: %q", got)
	}
}
