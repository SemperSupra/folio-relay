package state

import (
	"encoding/json"
	"fmt"
	"sync"
)

type DurableEngine struct {
	Engine  Engine
	Journal *Journal

	recordsMu sync.RWMutex
	records   []CommitRecord
}

func OpenDurableEngine(path string) (*DurableEngine, error) {
	journal, err := OpenJournal(path)
	if err != nil {
		return nil, err
	}
	d := &DurableEngine{Journal: journal}
	records, err := journal.Recover()
	if err != nil {
		journal.Close()
		return nil, err
	}
	for index, payload := range records {
		var record CommitRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			journal.Close()
			return nil, fmt.Errorf("%w: decode record %d: %v", ErrJournalCorrupt, index, err)
		}
		if err := d.Engine.Restore(record); err != nil {
			journal.Close()
			return nil, fmt.Errorf("%w: restore record %d: %v", ErrJournalCorrupt, index, err)
		}
		d.records = append(d.records, cloneCommitRecord(record))
	}
	return d, nil
}

func (d *DurableEngine) Apply(cmd Command, transition Transition) (ApplyResult, error) {
	if d == nil || d.Journal == nil {
		return ApplyResult{}, ErrInvalidCommand
	}
	return d.Engine.ApplyCommitted(cmd, transition, func(record CommitRecord) error {
		payload, err := json.Marshal(record)
		if err != nil {
			return fmt.Errorf("encode commit record: %w", err)
		}
		if err := d.Journal.Append(payload); err != nil {
			return err
		}
		d.recordsMu.Lock()
		d.records = append(d.records, cloneCommitRecord(record))
		d.recordsMu.Unlock()
		return nil
	})
}

func (d *DurableEngine) Records() []CommitRecord {
	if d == nil {
		return nil
	}
	d.recordsMu.RLock()
	defer d.recordsMu.RUnlock()
	out := make([]CommitRecord, len(d.records))
	for i := range d.records {
		out[i] = cloneCommitRecord(d.records[i])
	}
	return out
}

func cloneCommitRecord(record CommitRecord) CommitRecord {
	cloned := record
	if record.Command.Metadata != nil {
		cloned.Command.Metadata = make(map[string]string, len(record.Command.Metadata))
		for key, value := range record.Command.Metadata {
			cloned.Command.Metadata[key] = value
		}
	}
	if record.Result.Effects != nil {
		cloned.Result.Effects = append([]EffectIntent(nil), record.Result.Effects...)
	}
	return cloned
}

func (d *DurableEngine) Snapshot(aggregateID string) Snapshot {
	return d.Engine.Snapshot(aggregateID)
}

func (d *DurableEngine) Close() error {
	if d == nil || d.Journal == nil {
		return nil
	}
	return d.Journal.Close()
}
