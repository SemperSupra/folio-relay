package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDurableEngineReplaySurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.frj")
	d, err := OpenDurableEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	cmd := Command{
		Type: "accept", AggregateID: "job-1",
		IdempotencyKey: "client-1", SemanticFingerprint: "sha256:a",
	}
	first, err := d.Apply(cmd, acceptOnce)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || first.Snapshot.Generation != 1 || len(first.Effects) != 1 {
		t.Fatalf("unexpected first result: %+v", first)
	}
	effectID := first.Effects[0].ID
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}

	d, err = OpenDurableEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	replayed, err := d.Apply(cmd, acceptOnce)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed {
		t.Fatal("accepted command was not recovered as replay")
	}
	if replayed.Snapshot.Generation != 1 || len(replayed.Effects) != 1 || replayed.Effects[0].ID != effectID {
		t.Fatalf("recovery changed durable identity: %+v", replayed)
	}

	records, err := d.Journal.Recover()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("replay appended duplicate journal record: %d", len(records))
	}
}

func TestDurableEngineConflictSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.frj")
	d, err := OpenDurableEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	cmd := Command{
		Type: "accept", AggregateID: "job-1",
		IdempotencyKey: "client-1", SemanticFingerprint: "sha256:a",
	}
	if _, err := d.Apply(cmd, acceptOnce); err != nil {
		t.Fatal(err)
	}
	_ = d.Close()

	d, err = OpenDurableEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	cmd.SemanticFingerprint = "sha256:b"
	if _, err := d.Apply(cmd, acceptOnce); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected recovered idempotency conflict, got %v", err)
	}
}

func TestCommitFailureDoesNotPublishState(t *testing.T) {
	var engine Engine
	cmd := Command{
		Type: "accept", AggregateID: "job-1",
		IdempotencyKey: "key", SemanticFingerprint: "sha256:a",
	}
	sentinel := errors.New("durability failed")
	_, err := engine.ApplyCommitted(cmd, acceptOnce, func(CommitRecord) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected commit error, got %v", err)
	}
	if got := engine.Snapshot("job-1"); got.Generation != 0 || got.State != "" {
		t.Fatalf("state published before durable commit: %+v", got)
	}
	result, err := engine.Apply(cmd, acceptOnce)
	if err != nil {
		t.Fatalf("failed command incorrectly consumed idempotency key: %v", err)
	}
	if result.Snapshot.Generation != 1 {
		t.Fatalf("unexpected generation after retry: %+v", result)
	}
}

func TestDurableEngineFailsClosedOnRecordTamper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.frj")
	d, err := OpenDurableEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Apply(Command{
		Type: "accept", AggregateID: "job-1",
		IdempotencyKey: "key", SemanticFingerprint: "sha256:a",
	}, acceptOnce); err != nil {
		t.Fatal(err)
	}
	_ = d.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 0xff
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDurableEngine(path); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("expected fail-closed journal error, got %v", err)
	}
}
