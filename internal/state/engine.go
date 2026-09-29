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
	ErrInvalidTransition   = errors.New("invalid transition")
	ErrRestoreConflict     = errors.New("restore conflict")
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

type CommitRecord struct {
	Version  int         `json:"version"`
	Previous Snapshot    `json:"previous"`
	Command  Command     `json:"command"`
	Changed  bool        `json:"changed"`
	Result   ApplyResult `json:"result"`
}

type CommitFunc func(CommitRecord) error

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
	return e.ApplyCommitted(cmd, transition, nil)
}

func (e *Engine) ApplyCommitted(cmd Command, transition Transition, commit CommitFunc) (ApplyResult, error) {
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

	previousSnapshot := a.snapshot
	next, err := transition(previousSnapshot, cmd)
	if err != nil {
		return ApplyResult{}, err
	}
	if next.Changed && next.State == "" {
		return ApplyResult{}, ErrInvalidTransition
	}
	if !next.Changed && len(next.Effects) > 0 {
		return ApplyResult{}, ErrInvalidTransition
	}

	snapshot := previousSnapshot
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
	if commit != nil {
		record := CommitRecord{
			Version: 1, Previous: previousSnapshot, Command: cmd,
			Changed: next.Changed, Result: result,
		}
		if err := commit(record); err != nil {
			return ApplyResult{}, err
		}
	}

	a.snapshot = snapshot
	a.seen[cmd.IdempotencyKey] = storedResult{
		fingerprint: cmd.SemanticFingerprint,
		result: result,
	}
	return result, nil
}

func (e *Engine) Restore(record CommitRecord) error {
	if record.Version != 1 ||
		record.Command.AggregateID == "" ||
		record.Command.IdempotencyKey == "" ||
		record.Command.SemanticFingerprint == "" ||
		record.Result.Replayed {
		return ErrRestoreConflict
	}

	a := e.aggregate(record.Command.AggregateID)
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.seen[record.Command.IdempotencyKey]; exists {
		return ErrRestoreConflict
	}
	if a.snapshot != record.Previous {
		return ErrRestoreConflict
	}
	if record.Changed {
		if record.Result.Snapshot.Generation != record.Previous.Generation+1 ||
			record.Result.Snapshot.State == "" {
			return ErrRestoreConflict
		}
	} else if record.Result.Snapshot != record.Previous || len(record.Result.Effects) > 0 {
		return ErrRestoreConflict
	}

	for i, effect := range record.Result.Effects {
		if effect.AggregateID != record.Command.AggregateID ||
			effect.Generation != record.Result.Snapshot.Generation {
			return ErrRestoreConflict
		}
		spec := EffectSpec{Kind: effect.Kind, PayloadDigest: effect.PayloadDigest}
		if effect.ID != effectID(record.Command.AggregateID, record.Result.Snapshot.Generation, i, spec) {
			return ErrRestoreConflict
		}
	}

	a.snapshot = record.Result.Snapshot
	a.seen[record.Command.IdempotencyKey] = storedResult{
		fingerprint: record.Command.SemanticFingerprint,
		result: record.Result,
	}
	return nil
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
