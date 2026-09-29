package state

import (
	"errors"
	"sync"
	"testing"
)

func acceptOnce(current Snapshot, _ Command) (TransitionResult, error) {
	if current.State == "" {
		return TransitionResult{
			State: "accepted", Changed: true,
			Effects: []EffectSpec{{Kind: "render", PayloadDigest: "sha256:doc"}},
		}, nil
	}
	return TransitionResult{State: current.State, Changed: false}, nil
}

func TestConcurrentReplayHasOneTransitionAndOneEffectIdentity(t *testing.T) {
	var engine Engine
	const callers = 100
	results := make(chan ApplyResult, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := engine.Apply(Command{
				Type: "accept",
				AggregateID: "job-1",
				IdempotencyKey: "client-command-1",
				SemanticFingerprint: "sha256:same-command",
			}, acceptOnce)
			results <- result
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent replay failed: %v", err)
		}
	}

	var effectID string
	for result := range results {
		if result.Snapshot.Generation != 1 || result.Snapshot.State != "accepted" {
			t.Fatalf("unexpected snapshot: %+v", result.Snapshot)
		}
		if len(result.Effects) != 1 {
			t.Fatalf("expected one durable effect intent, got %d", len(result.Effects))
		}
		if effectID == "" {
			effectID = result.Effects[0].ID
		} else if result.Effects[0].ID != effectID {
			t.Fatalf("replay changed effect identity: %q != %q", result.Effects[0].ID, effectID)
		}
	}

	if got := engine.Snapshot("job-1").Generation; got != 1 {
		t.Fatalf("generation advanced more than once: %d", got)
	}
}

func TestSameIdempotencyKeyDifferentCommandConflicts(t *testing.T) {
	var engine Engine
	base := Command{
		Type: "accept", AggregateID: "job-1",
		IdempotencyKey: "key-1", SemanticFingerprint: "sha256:a",
	}
	if _, err := engine.Apply(base, acceptOnce); err != nil {
		t.Fatal(err)
	}
	base.SemanticFingerprint = "sha256:b"
	if _, err := engine.Apply(base, acceptOnce); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}

func TestGenerationConflictDoesNotConsumeIdempotencyKey(t *testing.T) {
	var engine Engine
	if _, err := engine.Apply(Command{
		Type: "accept", AggregateID: "job-1",
		IdempotencyKey: "first", SemanticFingerprint: "sha256:first",
	}, acceptOnce); err != nil {
		t.Fatal(err)
	}

	stale := uint64(0)
	cmd := Command{
		Type: "noop", AggregateID: "job-1",
		IdempotencyKey: "retryable-key", SemanticFingerprint: "sha256:noop",
		ExpectedGeneration: &stale,
	}
	if _, err := engine.Apply(cmd, acceptOnce); !errors.Is(err, ErrGenerationConflict) {
		t.Fatalf("expected generation conflict, got %v", err)
	}

	fresh := uint64(1)
	cmd.ExpectedGeneration = &fresh
	if _, err := engine.Apply(cmd, acceptOnce); err != nil {
		t.Fatalf("idempotency key was incorrectly consumed by stale command: %v", err)
	}
}
