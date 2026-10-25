package cloudcopy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"

	"github.com/SemperSupra/folio-relay/internal/state"
)

var ErrDeliveryUncertain = errors.New("cloud delivery uncertain; inspect before a separately authorized new action")

// Delivery is a projection of a durable intent. A dispatch found after restart
// is uncertain because a remote provider may have accepted the bytes.
type Delivery struct {
	ID         string
	Plan       Plan
	State      string
	Generation uint64
}

// Claim is a process-local dispatch capability. It is not persisted and cannot
// be recreated from a journal after restart.
type Claim struct {
	Delivery Delivery
	token    [32]byte
}

type Ledger struct {
	durable *state.DurableEngine
	active  sync.Map // effect ID -> process-local claim token
}

func OpenLedger(path string) (*Ledger, error) {
	d, err := state.OpenDurableEngine(path)
	if err != nil {
		return nil, err
	}
	return &Ledger{durable: d}, nil
}

func (l *Ledger) Close() error {
	if l == nil || l.durable == nil {
		return nil
	}
	return l.durable.Close()
}

func deliveryID(actionID string) string {
	sum := sha256.Sum256([]byte("foliorelay-cloud-delivery/v1\x00" + actionID))
	return hex.EncodeToString(sum[:])
}

func validDeliveryID(id string) bool { return digestPattern.MatchString(id) }

// Enqueue binds one action ID to one exact destination and artifact. Repeating
// identical admission is harmless; changing any field is rejected by the
// durable engine's semantic fingerprint.
func (l *Ledger) Enqueue(p Plan) (Delivery, error) {
	if l == nil || l.durable == nil {
		return Delivery{}, ErrDeliveryUncertain
	}
	if err := validatePlan(p); err != nil {
		return Delivery{}, err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return Delivery{}, err
	}
	sum := sha256.Sum256(b)
	id := deliveryID(p.ActionID)
	_, err = l.durable.Apply(state.Command{
		Type: "cloud-enqueue", AggregateID: "cloud/" + id, IdempotencyKey: "enqueue-v1",
		SemanticFingerprint: hex.EncodeToString(sum[:]), Metadata: map[string]string{"plan": string(b)},
	}, func(old state.Snapshot, _ state.Command) (state.TransitionResult, error) {
		if old.State != "" {
			return state.TransitionResult{}, ErrDeliveryUncertain
		}
		return state.TransitionResult{State: "pending", Changed: true}, nil
	})
	if err != nil {
		return Delivery{}, err
	}
	return l.Get(id)
}

func (l *Ledger) Get(id string) (Delivery, error) {
	if l == nil || l.durable == nil || !validDeliveryID(id) {
		return Delivery{}, ErrDeliveryUncertain
	}
	aggregate := "cloud/" + id
	snap := l.durable.Snapshot(aggregate)
	if snap.State == "" {
		return Delivery{}, ErrDeliveryUncertain
	}
	for _, r := range l.durable.Records() {
		if r.Command.AggregateID != aggregate || r.Command.Type != "cloud-enqueue" {
			continue
		}
		var p Plan
		if err := json.Unmarshal([]byte(r.Command.Metadata["plan"]), &p); err != nil {
			return Delivery{}, err
		}
		status := snap.State
		if status == "dispatching" {
			if _, ok := l.active.Load(id); !ok {
				status = "delivery_unknown"
			}
		}
		return Delivery{ID: id, Plan: p, State: status, Generation: snap.Generation}, nil
	}
	return Delivery{}, ErrDeliveryUncertain
}

// Begin reserves the single external attempt durably before any upload I/O.
// A replayed dispatch is never a second claim, even under concurrent callers.
func (l *Ledger) Begin(id string) (Claim, error) {
	if l == nil || l.durable == nil || !validDeliveryID(id) {
		return Claim{}, ErrDeliveryUncertain
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return Claim{}, err
	}
	aggregate := "cloud/" + id
	snap := l.durable.Snapshot(aggregate)
	if snap.State != "pending" {
		return Claim{}, ErrDeliveryUncertain
	}
	result, err := l.durable.Apply(state.Command{
		Type: "cloud-dispatch", AggregateID: aggregate, IdempotencyKey: "dispatch-v1",
		SemanticFingerprint: id, ExpectedGeneration: &snap.Generation,
	}, func(old state.Snapshot, _ state.Command) (state.TransitionResult, error) {
		if old.State != "pending" {
			return state.TransitionResult{}, ErrDeliveryUncertain
		}
		return state.TransitionResult{State: "dispatching", Changed: true}, nil
	})
	if err != nil {
		return Claim{}, err
	}
	if result.Replayed {
		return Claim{}, ErrDeliveryUncertain
	}
	l.active.Store(id, token)
	delivery, err := l.Get(id)
	if err != nil {
		return Claim{}, err
	}
	return Claim{Delivery: delivery, token: token}, nil
}

// Complete records the result of the one claimed attempt. failed means a
// definite failure before remote acceptance; unknown means acceptance cannot
// be ruled out. Neither state permits automatic retry of the same action.
func (l *Ledger) Complete(c Claim, outcome string) (Delivery, error) {
	if l == nil || l.durable == nil || !validDeliveryID(c.Delivery.ID) {
		return Delivery{}, ErrDeliveryUncertain
	}
	if outcome != "accepted" && outcome != "failed" && outcome != "delivery_unknown" {
		return Delivery{}, ErrDeliveryUncertain
	}
	current, ok := l.active.Load(c.Delivery.ID)
	if !ok || current != c.token {
		return Delivery{}, ErrDeliveryUncertain
	}
	aggregate := "cloud/" + c.Delivery.ID
	snap := l.durable.Snapshot(aggregate)
	if snap.State != "dispatching" {
		return Delivery{}, ErrDeliveryUncertain
	}
	result, err := l.durable.Apply(state.Command{
		Type: "cloud-complete", AggregateID: aggregate, IdempotencyKey: "complete-v1",
		SemanticFingerprint: c.Delivery.ID + "/" + outcome, ExpectedGeneration: &snap.Generation,
	}, func(old state.Snapshot, _ state.Command) (state.TransitionResult, error) {
		if old.State != "dispatching" {
			return state.TransitionResult{}, ErrDeliveryUncertain
		}
		return state.TransitionResult{State: outcome, Changed: true}, nil
	})
	if err != nil {
		return Delivery{}, err
	}
	if result.Replayed {
		return Delivery{}, ErrDeliveryUncertain
	}
	l.active.Delete(c.Delivery.ID)
	return l.Get(c.Delivery.ID)
}
