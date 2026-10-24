package export

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sync"

	"github.com/SemperSupra/folio-relay/internal/state"
)

var (
	ErrReconcile = errors.New("export requires explicit reconciliation")
	actionPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,80}$`)
)

// Intent is supplied by an authorized control-plane command. Each explicit
// re-export must have a new ActionID; replayed ActionIDs cannot change payload.
type Intent struct {
	ActionID string `json:"action_id"`
	JobID string `json:"job_id"`
	Plan Plan `json:"plan"`
	Artifact Artifact `json:"artifact"`
}

type ExportEffect struct {
	ID string
	Intent Intent
	State string
	Generation uint64
}

type Ledger struct {
	durable *state.DurableEngine
	active sync.Map
}

func OpenLedger(path string) (*Ledger,error) {
	d,err:=state.OpenDurableEngine(path)
	if err!=nil { return nil,err }
	return &Ledger{durable:d},nil
}
func (l *Ledger) Close() error {
	if l==nil || l.durable==nil { return nil }
	return l.durable.Close()
}
func actionID(s string) string {
	d:=sha256.Sum256([]byte("foliorelay-managed-export/v1\x00"+s))
	return hex.EncodeToString(d[:])
}
func validEffectID(id string) bool {
	if len(id)!=64 { return false }
	b,err:=hex.DecodeString(id)
	return err==nil && len(b)==32 && hex.EncodeToString(b)==id
}
func (l *Ledger) Enqueue(i Intent) (ExportEffect,error) {
	if l==nil || l.durable==nil { return ExportEffect{},errors.New("ledger unavailable") }
	if !actionPattern.MatchString(i.ActionID) || !jobIDPattern.MatchString(i.JobID) {
		return ExportEffect{},errors.New("invalid export action or job identity")
	}
	if err:=validatePlan(i.Plan);err!=nil { return ExportEffect{},err }
	if err:=validateArtifact(i.Artifact);err!=nil { return ExportEffect{},err }
	b,err:=json.Marshal(i)
	if err!=nil { return ExportEffect{},err }
	digest:=sha256.Sum256(b)
	id:=actionID(i.ActionID)
	_,err=l.durable.Apply(state.Command{
		Type:"export-enqueue",AggregateID:"export/"+id,
		IdempotencyKey:"enqueue-v1",SemanticFingerprint:hex.EncodeToString(digest[:]),
		Metadata:map[string]string{"intent":string(b)},
	},func(old state.Snapshot,_ state.Command)(state.TransitionResult,error){
		if old.State!="" { return state.TransitionResult{},ErrReconcile }
		return state.TransitionResult{State:"pending",Changed:true},nil
	})
	if err!=nil { return ExportEffect{},err }
	return l.Get(id)
}

func (l *Ledger) Get(id string) (ExportEffect,error) {
	if l==nil || l.durable==nil || !validEffectID(id) { return ExportEffect{},ErrReconcile }
	aggregate:="export/"+id
	snap:=l.durable.Snapshot(aggregate)
	if snap.State=="" { return ExportEffect{},ErrReconcile }
	for _,r:=range l.durable.Records() {
		if r.Command.AggregateID!=aggregate || r.Command.Type!="export-enqueue" {continue}
		var i Intent
		if err:=json.Unmarshal([]byte(r.Command.Metadata["intent"]),&i);err!=nil {return ExportEffect{},err}
		status:=snap.State
		if status=="dispatching" {
			if _,ok:=l.active.Load(id);!ok {status="reconcile_required"}
		}
		return ExportEffect{ID:id,Intent:i,State:status,Generation:snap.Generation},nil
	}
	return ExportEffect{},ErrReconcile
}

// Begin durably reserves the effect before any filesystem publication. A
// restarted ledger refuses to begin an already reserved effect.
func (l *Ledger) Begin(id string) (ExportEffect,error) {
	if l==nil || l.durable==nil || !validEffectID(id) {return ExportEffect{},ErrReconcile}
	agg:="export/"+id
	snap:=l.durable.Snapshot(agg)
	if snap.State!="pending" {return ExportEffect{},ErrReconcile}
	_,err:=l.durable.Apply(state.Command{
		Type:"export-dispatch",AggregateID:agg,IdempotencyKey:"dispatch-v1",
		SemanticFingerprint:id,ExpectedGeneration:&snap.Generation,
	},func(old state.Snapshot,_ state.Command)(state.TransitionResult,error){
		if old.State!="pending" {return state.TransitionResult{},ErrReconcile}
		return state.TransitionResult{State:"dispatching",Changed:true},nil
	})
	if err!=nil{return ExportEffect{},err}
	l.active.Store(id,true)
	return l.Get(id)
}

// Complete is only allowed by the process holding the dispatch, not by a
// restarted caller that might accidentally replay an external effect.
func (l *Ledger) Complete(id string, success bool) (ExportEffect,error) {
	if l==nil || l.durable==nil || !validEffectID(id) {return ExportEffect{},ErrReconcile}
	if _,ok:=l.active.Load(id);!ok {return ExportEffect{},ErrReconcile}
	agg:="export/"+id
	snap:=l.durable.Snapshot(agg)
	if snap.State!="dispatching" {return ExportEffect{},ErrReconcile}
	status:="failed"
	if success {status="published"}
	_,err:=l.durable.Apply(state.Command{
		Type:"export-complete",AggregateID:agg,IdempotencyKey:"complete-v1",
		SemanticFingerprint:fmt.Sprintf("%s/%s",id,status),ExpectedGeneration:&snap.Generation,
	},func(old state.Snapshot,_ state.Command)(state.TransitionResult,error){
		if old.State!="dispatching" {return state.TransitionResult{},ErrReconcile}
		return state.TransitionResult{State:status,Changed:true},nil
	})
	if err!=nil {return ExportEffect{},err}
	l.active.Delete(id)
	return l.Get(id)
}

// ReconcileAfterRestart inspects only. A matching already-published copy can
// be credited without copying it again. Absent/modified exports require a
// separate authenticated action with a fresh ActionID; there is no replay.
func (l *Ledger) ReconcileAfterRestart(id string, w *Worker) (ExportEffect,error) {
	if l==nil || l.durable==nil || !validEffectID(id) {return ExportEffect{},ErrReconcile}
	agg:="export/"+id
	snap:=l.durable.Snapshot(agg)
	if snap.State!="dispatching" {return ExportEffect{},ErrReconcile}
	if _,active:=l.active.Load(id);active {return ExportEffect{},ErrReconcile}
	e,err:=l.Get(id)
	if err!=nil {return ExportEffect{},err}
	status,err:=w.Inspect(Receipt{
		RelativePath:e.Intent.Plan.RelativePath,
		SHA256:e.Intent.Artifact.SHA256,Bytes:e.Intent.Artifact.Bytes,
	})
	if err!=nil {return ExportEffect{},err}
	verdict:="reconcile_required"
	if status=="present" {verdict="published"}
	_,err=l.durable.Apply(state.Command{
		Type:"export-reconcile",AggregateID:agg,IdempotencyKey:"reconcile-v1",
		SemanticFingerprint:id+"/"+verdict,ExpectedGeneration:&snap.Generation,
	},func(old state.Snapshot,_ state.Command)(state.TransitionResult,error){
		if old.State!="dispatching" {return state.TransitionResult{},ErrReconcile}
		return state.TransitionResult{State:verdict,Changed:true},nil
	})
	if err!=nil {return ExportEffect{},err}
	return l.Get(id)
}
