package state

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
)

var (
	ErrGenerationConflict  = errors.New("generation conflict")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	ErrInvalidCommand      = errors.New("invalid command")
)

type Command struct {
	Type               string
	AggregateID        string
	IdempotencyKey     string
	SemanticFingerprint string
	ExpectedGeneration *uint64
}

type Snapshot struct {
	State      string
	Generation uint64
}

type EffectSpec struct {
	Kind          string
	PayloadDigest string
}

type EffectIntent struct {
	ID            string
	AggregateID   string
	Generation    uint64
	Kind          string
	PayloadDigest string
}

type TransitionResult struct {
	State   string
	Changed bool
	Effects []EffectSpec
}

type ApplyResult struct {
	Snapshot Snapshot
	Effects  []EffectIntent
	Replayed bool
}

type Transition func(Snapshot, Command) (TransitionResult, error)

type storedResult struct {
	fingerprint string
	result      ApplyResult
}

type aggregate struct {
	mu      sync.Mutex
	snapshot Snapshot
	seen    map[string]storedResult
}

type Engine struct {
	aggregates sync.Map
}

func (e *Engine) aggregate(id string) *aggregate {
	value, _ := e.aggregates.LoadOrStore(id, &aggregate{
		seen: make(map[string]storedResult),
	})
	return value.(*aggregate)
}

func (e *Engine) Apply(cmd Command, transition Transition) (ApplyResult, error) {
	if cmd.AggregateID == "" || cmd.IdempotencyKey == "" || cmd.Type == "" || cmd.SemanticFingerprint == "" {
		return ApplyResult{}, ErrInvalidCommand
	}
	if transition == nil {
		return ApplyResult{}, ErrInvalidCommand
	}

	a := e.aggregate(cmd.AggregateID)
	a.mu.Lock()
	defer a.mu.Unlock()

	if previous, ok := a.seen[cmd.IdempotencyKey]; ok {
		if previous.fingerprint != cmd.SemanticFingerprint {
			return ApplyResult{}, ErrIdempotencyConflict
		}
		replayed := previous.result
		replayed.Replayed = true
		return replayed, nil
	}

	if cmd.ExpectedGeneration != nil && *cmd.ExpectedGeneration != a.snapshot.Generation {
		return ApplyResult{}, ErrGenerationConflict
	}

	next, err := transition(a.snapshot, cmd)
	if err != nil {
		return ApplyResult{}, err
	}

	snapshot := a.snapshot
	if next.Changed {
		snapshot.State = next.State
		snapshot.Generation++
	}

	effects := make([]EffectIntent, 0, len(next.Effects))
	for i, spec := range next.Effects {
		id := effectID(cmd.AggregateID, snapshot.Generation, i, spec)
		effects = append(effects, EffectIntent{
			ID: id, AggregateID: cmd.AggregateID, Generation: snapshot.Generation,
			Kind: spec.Kind, PayloadDigest: spec.PayloadDigest,
		})
	}

	result := ApplyResult{Snapshot: snapshot, Effects: effects}
	a.snapshot = snapshot
	a.seen[cmd.IdempotencyKey] = storedResult{
		fingerprint: cmd.SemanticFingerprint,
		result: result,
	}
	return result, nil
}

func (e *Engine) Snapshot(aggregateID string) Snapshot {
	a := e.aggregate(aggregateID)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshot
}

func effectID(aggregateID string, generation uint64, ordinal int, spec EffectSpec) string {
	h := sha256.New()
	fmt.Fprint(h, aggregateID, "\x00", strconv.FormatUint(generation, 10), "\x00",
		strconv.Itoa(ordinal), "\x00", spec.Kind, "\x00", spec.PayloadDigest)
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
