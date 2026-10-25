package cloudcopy

import (
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/SemperSupra/folio-relay/internal/export"
	"github.com/SemperSupra/folio-relay/internal/state"
)

func ledgerFixture(t *testing.T) (*Ledger, Plan) {
	t.Helper()
	l, err := OpenLedger(filepath.Join(t.TempDir(), "cloud.journal"))
	if err != nil {
		t.Fatal(err)
	}
	profile, defaults, job, artifact := fixture()
	p, err := MakePlan(profile, "action_0001", defaults, export.Overrides{}, job, artifact)
	if err != nil {
		t.Fatal(err)
	}
	return l, p
}

func TestLedgerBindsActionToExactPlan(t *testing.T) {
	l, p := ledgerFixture(t)
	defer l.Close()
	a, err := l.Enqueue(p)
	if err != nil || a.State != "pending" {
		t.Fatalf("enqueue: %#v %v", a, err)
	}
	b, err := l.Enqueue(p)
	if err != nil || b != a {
		t.Fatalf("identical replay: %#v %v", b, err)
	}
	p.ArtifactBytes++
	if _, err := l.Enqueue(p); !errors.Is(err, state.ErrIdempotencyConflict) {
		t.Fatalf("changed artifact accepted: %v", err)
	}
	p = a.Plan
	p.RclonePath = "other:outside"
	if _, err := l.Enqueue(p); !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("raw remote accepted: %v", err)
	}
}

func TestConcurrentBeginHasOneOwner(t *testing.T) {
	l, p := ledgerFixture(t)
	defer l.Close()
	d, err := l.Enqueue(p)
	if err != nil {
		t.Fatal(err)
	}
	var owners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := l.Begin(d.ID); err == nil {
				owners.Add(1)
			}
		}()
	}
	wg.Wait()
	if owners.Load() != 1 {
		t.Fatalf("dispatch owners: %d", owners.Load())
	}
	if got, err := l.Get(d.ID); err != nil || got.State != "dispatching" {
		t.Fatalf("state: %#v %v", got, err)
	}
	if _, err := l.Begin(d.ID); !errors.Is(err, ErrDeliveryUncertain) {
		t.Fatalf("second begin: %v", err)
	}
}

func TestRestartNeverReplaysPossibleUpload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cloud.journal")
	l, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	profile, defaults, job, artifact := fixture()
	p, err := MakePlan(profile, "action_0001", defaults, export.Overrides{}, job, artifact)
	if err != nil {
		t.Fatal(err)
	}
	d, err := l.Enqueue(p)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := l.Begin(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	got, err := restarted.Get(d.ID)
	if err != nil || got.State != "delivery_unknown" {
		t.Fatalf("restart: %#v %v", got, err)
	}
	if _, err := restarted.Begin(d.ID); !errors.Is(err, ErrDeliveryUncertain) {
		t.Fatalf("replayed dispatch: %v", err)
	}
	if _, err := restarted.Complete(claim, "accepted"); !errors.Is(err, ErrDeliveryUncertain) {
		t.Fatalf("stale claim completed: %v", err)
	}
}

func TestClaimCompletionRequiresOwnerAndTerminalOutcome(t *testing.T) {
	l, p := ledgerFixture(t)
	defer l.Close()
	d, err := l.Enqueue(p)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := l.Begin(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	forged := claim
	forged.token[0] ^= 1
	if _, err := l.Complete(forged, "accepted"); !errors.Is(err, ErrDeliveryUncertain) {
		t.Fatalf("forged claim: %v", err)
	}
	if _, err := l.Complete(claim, "retrying"); !errors.Is(err, ErrDeliveryUncertain) {
		t.Fatalf("retry state: %v", err)
	}
	got, err := l.Complete(claim, "delivery_unknown")
	if err != nil || got.State != "delivery_unknown" {
		t.Fatalf("complete: %#v %v", got, err)
	}
	if _, err := l.Complete(claim, "accepted"); !errors.Is(err, ErrDeliveryUncertain) {
		t.Fatalf("double complete: %v", err)
	}
	if _, err := l.Begin(d.ID); !errors.Is(err, ErrDeliveryUncertain) {
		t.Fatalf("terminal retry: %v", err)
	}
}
