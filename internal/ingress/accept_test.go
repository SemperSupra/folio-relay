package ingress

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSameSourceAndBytesProduceStableCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job.pdf")
	if err := os.WriteFile(path, []byte("same bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := Source{Substrate: "ippeveprinter", InstanceID: "boot-1", JobID: "42"}
	a, err := PrepareFile(source, path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := PrepareFile(source, path)
	if err != nil {
		t.Fatal(err)
	}
	if a.Command != b.Command || a.ArtifactDigest != b.ArtifactDigest {
		t.Fatalf("replay identity changed: %#v %#v", a, b)
	}
}

func TestSameArtifactDifferentJobIsNotCollapsed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job.pdf")
	if err := os.WriteFile(path, []byte("same bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := PrepareFile(Source{Substrate: "cups", InstanceID: "boot-1", JobID: "1"}, path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := PrepareFile(Source{Substrate: "cups", InstanceID: "boot-1", JobID: "2"}, path)
	if err != nil {
		t.Fatal(err)
	}
	if a.ArtifactDigest != b.ArtifactDigest {
		t.Fatal("same file should have same artifact digest")
	}
	if a.Command.IdempotencyKey == b.Command.IdempotencyKey {
		t.Fatal("distinct print intents were incorrectly deduplicated")
	}
}

func TestSourceInstancePreventsJobIDReuseCollision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job.pdf")
	if err := os.WriteFile(path, []byte("same bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, _ := PrepareFile(Source{Substrate: "ippeveprinter", InstanceID: "boot-a", JobID: "1"}, path)
	b, _ := PrepareFile(Source{Substrate: "ippeveprinter", InstanceID: "boot-b", JobID: "1"}, path)
	if a.Command.IdempotencyKey == b.Command.IdempotencyKey {
		t.Fatal("job-id reuse across substrate instances collided")
	}
}

func TestChangedBytesWithSameSourceConflictsSemantically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job.pdf")
	source := Source{Substrate: "ippeveprinter", InstanceID: "boot-1", JobID: "7"}
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, _ := PrepareFile(source, path)
	if err := os.WriteFile(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ := PrepareFile(source, path)
	if a.Command.IdempotencyKey != b.Command.IdempotencyKey {
		t.Fatal("same source identity must reuse the same idempotency key")
	}
	if a.Command.SemanticFingerprint == b.Command.SemanticFingerprint {
		t.Fatal("changed bytes must produce an idempotency conflict fingerprint")
	}
}

func TestRejectsMissingSourceIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job.pdf")
	_ = os.WriteFile(path, []byte("x"), 0o600)
	_, err := PrepareFile(Source{Substrate: "cups"}, path)
	if !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("expected source identity rejection, got %v", err)
	}
}
