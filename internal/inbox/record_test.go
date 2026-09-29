package inbox

import (
	"testing"
	"time"

	frstate "github.com/SemperSupra/folio-relay/internal/state"
)

func TestProjectReturnsNewestFirstAndIgnoresLegacyRecords(t *testing.T) {
	first := committed("ingress/pappl/job-1", "2026-09-29T18:00:00Z", "1")
	legacy := first
	legacy.Command.AggregateID = "legacy"
	legacy.Command.IdempotencyKey = "legacy"
	legacy.Command.Metadata = nil
	second := committed("ingress/pappl/job-2", "2026-09-29T18:01:00Z", "2")

	got, err := Project([]frstate.CommitRecord{first, legacy, second})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected two projected jobs, got %d", len(got))
	}
	if got[0].JobID != "ingress/pappl/job-2" || got[1].JobID != "ingress/pappl/job-1" {
		t.Fatalf("unexpected projection order: %+v", got)
	}
	if got[0].Copies != 2 || got[0].ArtifactBytes != 1234 {
		t.Fatalf("projection lost metadata: %+v", got[0])
	}
}

func TestProjectFailsClosedOnMarkedMalformedMetadata(t *testing.T) {
	record := committed("ingress/cups/job-1", "2026-09-29T18:00:00Z", "1")
	record.Command.Metadata["artifact_sha256"] = "not-a-digest"
	if _, err := Project([]frstate.CommitRecord{record}); err == nil {
		t.Fatal("expected malformed current inbox metadata to fail")
	}
}

func committed(id, acceptedAt, copies string) frstate.CommitRecord {
	return frstate.CommitRecord{
		Version: 1,
		Command: frstate.Command{
			Type:                "ingest-artifact",
			AggregateID:         id,
			IdempotencyKey:      id,
			SemanticFingerprint: "sha256:fingerprint",
			Metadata: map[string]string{
				"inbox_version":   "1",
				"accepted_at":      acceptedAt,
				"artifact_sha256":  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"artifact_bytes":   "1234",
				"media_type":       "application/pdf",
				"copies":           copies,
				"substrate":        "pappl",
				"substrate_job_id": "7",
			},
		},
		Changed: true,
		Result: frstate.ApplyResult{
			Snapshot: frstate.Snapshot{State: "accepted", Generation: 1},
		},
		Previous: frstate.Snapshot{},
	}
}

func TestAcceptedAtUsesUTCCompatibleTimestamp(t *testing.T) {
	record := committed("ingress/pappl/job-1", time.Now().UTC().Format(time.RFC3339Nano), "1")
	got, err := Project([]frstate.CommitRecord{record})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].AcceptedAt.Location() != time.UTC {
		t.Fatalf("expected UTC accepted time, got %+v", got)
	}
}
