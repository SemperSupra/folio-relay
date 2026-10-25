package documents

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SemperSupra/folio-relay/internal/inbox"
	"github.com/SemperSupra/folio-relay/internal/state"
)

type ownerPolicy struct{}

func (ownerPolicy) Authorize(p Principal, _ inbox.Record, _ string) error {
	if p.Issuer != "https://idp.example.test" || p.ID != "owner-1" || p.Kind != "human" {
		return ErrUnauthorized
	}
	return nil
}

func acceptFixture(t *testing.T, d *state.DurableEngine) string {
	t.Helper()
	agg := "ingress/cups/42"
	_, err := d.Apply(state.Command{
		Type: "ingest-artifact", AggregateID: agg, IdempotencyKey: "accept-42", SemanticFingerprint: "accepted-42",
		Metadata: map[string]string{
			"inbox_version": "1", "accepted_at": time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
			"artifact_sha256": strings.Repeat("a", 64), "artifact_bytes": "12", "media_type": "application/pdf",
			"copies": "1", "substrate": "cups", "substrate_job_id": "42", "source_title": "Original.pdf",
		},
	}, func(old state.Snapshot, _ state.Command) (state.TransitionResult, error) {
		if old.State != "" {
			t.Fatal("fixture reused")
		}
		return state.TransitionResult{State: "accepted", Changed: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return inbox.JobID(agg)
}

func TestMetadataArchiveRestoreAndReplaySurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.frj")
	d, err := state.OpenDurableEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	jobID := acceptFixture(t, d)
	s, err := NewService(d, ownerPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	actor := Principal{Issuer: "https://idp.example.test", ID: "owner-1", Kind: "human"}
	initial, err := s.Get(actor, jobID)
	if err != nil || initial.Revision != 0 || initial.Job.SourceTitle != "Original.pdf" {
		t.Fatalf("initial: %#v %v", initial, err)
	}
	edit := Command{JobID: jobID, ActionID: "action_edit_1", Kind: "set_metadata", ExpectedRevision: 0,
		Metadata: Annotation{Label: "Bericht รายงาน", Tags: []string{"finance", "สำคัญ"}, Note: "Reviewed"}}
	first, err := s.Apply(actor, edit)
	if err != nil || first.Revision != 1 || first.Replayed {
		t.Fatalf("edit: %#v %v", first, err)
	}
	archive := Command{JobID: jobID, ActionID: "action_archive_1", Kind: "archive", ExpectedRevision: 1}
	second, err := s.Apply(actor, archive)
	if err != nil || second.Revision != 2 || second.State != "archived" {
		t.Fatalf("archive: %#v %v", second, err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = state.OpenDurableEngine(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s, err = NewService(d, ownerPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := s.Apply(actor, edit)
	if err != nil || !replayed.Replayed || replayed.Revision != 1 || replayed.State != "active" {
		t.Fatalf("replay: %#v %v", replayed, err)
	}
	view, err := s.Get(actor, jobID)
	if err != nil || view.Revision != 2 || !view.Annotation.Archived || view.Annotation.Label != "Bericht รายงาน" ||
		view.Job.SourceTitle != "Original.pdf" || view.Job.ArtifactSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("view: %#v %v", view, err)
	}
	view.Annotation.Tags[0] = "caller-mutated"
	again, err := s.Get(actor, jobID)
	if err != nil || again.Annotation.Tags[0] != "finance" {
		t.Fatalf("projection aliased caller data: %#v %v", again, err)
	}
	restore := Command{JobID: jobID, ActionID: "action_restore_1", Kind: "restore", ExpectedRevision: 2}
	third, err := s.Apply(actor, restore)
	if err != nil || third.Revision != 3 || third.State != "active" {
		t.Fatalf("restore: %#v %v", third, err)
	}
	if _, err := s.Apply(actor, archive); err != nil {
		t.Fatalf("original archive replay: %v", err)
	}
}

func TestDenyUnknownJobUnauthorizedActorAndChangedReplay(t *testing.T) {
	d, err := state.OpenDurableEngine(filepath.Join(t.TempDir(), "journal.frj"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	jobID := acceptFixture(t, d)
	s, err := NewService(d, ownerPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	actor := Principal{Issuer: "https://idp.example.test", ID: "owner-1", Kind: "human"}
	cmd := Command{JobID: jobID, ActionID: "action_edit_1", Kind: "set_metadata", Metadata: Annotation{Label: "Review"}}
	wrongActor := Principal{Issuer: "https://idp.example.test", ID: "stranger", Kind: "human"}
	if _, err := s.Apply(wrongActor, cmd); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unauthorized apply: %v", err)
	}
	if _, err := s.Get(wrongActor, jobID); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unauthorized read: %v", err)
	}
	sameNameOtherIssuer := Principal{Issuer: "https://other-idp.example.test", ID: "owner-1", Kind: "human"}
	if _, err := s.Get(sameNameOtherIssuer, jobID); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("cross-issuer read: %v", err)
	}
	if _, err := s.Apply(sameNameOtherIssuer, cmd); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("cross-issuer apply: %v", err)
	}
	if _, err := s.Apply(Principal{ID: "owner-1", Kind: "human"}, cmd); !errors.Is(err, ErrInvalid) {
		t.Fatalf("issuerless principal: %v", err)
	}
	unknown := cmd
	unknown.JobID = "job-" + strings.Repeat("0", 64)
	if _, err := s.Apply(actor, unknown); !errors.Is(err, ErrNotFound) {
		t.Fatalf("forged job: %v", err)
	}
	if _, err := s.Apply(actor, cmd); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(wrongActor, cmd); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("different actor reused another actor's receipt: %v", err)
	}
	commits := d.Records()
	if len(commits) != 2 || commits[1].Command.Metadata["actor_issuer"] != "https://idp.example.test" || commits[1].Command.Metadata["actor_id"] != "owner-1" ||
		commits[1].Command.Metadata["action"] != "set_metadata" ||
		commits[1].Command.Metadata["source_sha256"] != strings.Repeat("a", 64) ||
		commits[1].Command.Metadata["previous"] == "" || commits[1].Command.Metadata["next"] == "" ||
		len(commits[1].Result.Effects) != 0 {
		t.Fatalf("missing audit or unexpected effect: %#v", commits[1])
	}
	changed := cmd
	changed.Metadata.Label = "Different"
	if _, err := s.Apply(actor, changed); !errors.Is(err, state.ErrIdempotencyConflict) {
		t.Fatalf("changed replay: %v", err)
	}
	stale := cmd
	stale.ActionID = "action_edit_2"
	if _, err := s.Apply(actor, stale); !errors.Is(err, state.ErrGenerationConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	noChange := cmd
	noChange.ActionID = "action_edit_4"
	noChange.ExpectedRevision = 1
	if _, err := s.Apply(actor, noChange); !errors.Is(err, ErrNoChange) {
		t.Fatalf("unchanged metadata wrote another revision: %v", err)
	}
	invalid := cmd
	invalid.ActionID = "action_edit_3"
	invalid.ExpectedRevision = 1
	invalid.Metadata.Label = "bad\nlabel"
	if _, err := s.Apply(actor, invalid); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid metadata: %v", err)
	}
	if len(d.Records()) != 2 {
		t.Fatalf("denied/stale action wrote journal: %d records", len(d.Records()))
	}
}

func TestConcurrentRevisionHasOneWinner(t *testing.T) {
	d, err := state.OpenDurableEngine(filepath.Join(t.TempDir(), "journal.frj"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	jobID := acceptFixture(t, d)
	s, err := NewService(d, ownerPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := Command{JobID: jobID, ActionID: fmt.Sprintf("action_concurrent_%02d", i),
				Kind: "set_metadata", ExpectedRevision: 0,
				Metadata: Annotation{Label: fmt.Sprintf("Candidate%02d", i)}}
			if _, err := s.Apply(Principal{Issuer: "https://idp.example.test", ID: "owner-1", Kind: "human"}, cmd); err == nil {
				won.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatalf("concurrent winners: %d", won.Load())
	}
	view, err := s.Get(Principal{Issuer: "https://idp.example.test", ID: "owner-1", Kind: "human"}, jobID)
	if err != nil || view.Revision != 1 {
		t.Fatalf("revision: %#v %v", view, err)
	}
}
