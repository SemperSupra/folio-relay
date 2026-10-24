// Package outbox is the durable, networkless submission-control primitive.
// Transport adapters must never infer recipients from document content.
package outbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"strings"

	"github.com/SemperSupra/folio-relay/internal/security"
	"github.com/SemperSupra/folio-relay/internal/state"
)

var (
	ErrWrongState = errors.New("email effect not in required state")
	ErrInvalidEffect = errors.New("invalid email effect")
	ErrUnresolvedDelivery = errors.New("email delivery needs explicit operator resolution")
	idPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
)

type Request struct {
	Recipient string
	ArtifactSHA256 string
	SenderProfileID string
	RouteLegID string
	SubjectPolicyID string
}

type Effect struct {
	ID string `json:"effect_id"`
	Recipient string `json:"recipient"`
	ArtifactSHA256 string `json:"artifact_sha256"`
	SenderProfileID string `json:"sender_profile_id"`
	RouteLegID string `json:"route_leg_id"`
	SubjectPolicyID string `json:"subject_policy_id"`
	State string `json:"state"`
	Generation uint64 `json:"generation"`
}

type Outbox struct {
	durable *state.DurableEngine
	active sync.Map // process-local dispatches; not recovered after restart
}

func Open(path string) (*Outbox, error) {
	d,err:=state.OpenDurableEngine(path)
	if err != nil { return nil, err }
	return &Outbox{durable:d},nil
}
func (o *Outbox) Close() error {
	if o==nil || o.durable==nil { return nil }
	return o.durable.Close()
}

func validDigest(s string) bool {
	if len(s)!=64 { return false }
	d,err:=hex.DecodeString(s)
	return err==nil && len(d)==32 && hex.EncodeToString(d)==s
}
func effectID(r Request) string {
	b,_:=json.Marshal(r)
	sum:=sha256.Sum256(append([]byte("foliorelay-email-artifact/v1\x00"),b...))
	return hex.EncodeToString(sum[:])
}
func validate(r Request, policy security.EmailRecipientPolicy) (Request,error) {
	addr,err:=security.ValidateEmailRecipient(r.Recipient,policy)
	if err!=nil { return Request{},err }
	r.Recipient=addr.Mailbox
	if !validDigest(r.ArtifactSHA256) ||
		!idPattern.MatchString(r.SenderProfileID) ||
		!idPattern.MatchString(r.RouteLegID) ||
		!idPattern.MatchString(r.SubjectPolicyID) {
		return Request{},ErrInvalidEffect
	}
	return r,nil
}

func (o *Outbox) Enqueue(request Request, policy security.EmailRecipientPolicy) (Effect,error) {
	if o==nil || o.durable==nil { return Effect{},ErrInvalidEffect }
	r,err:=validate(request,policy)
	if err!=nil { return Effect{},err }
	id:=effectID(r)
	agg:="email/"+id
	cmd:=state.Command{
		Type:"email-enqueue",AggregateID:agg,IdempotencyKey:"enqueue-v1",
		SemanticFingerprint:id,
		Metadata:map[string]string{
			"recipient":r.Recipient, "artifact_sha256":r.ArtifactSHA256,
			"sender_profile_id":r.SenderProfileID,"route_leg_id":r.RouteLegID,
			"subject_policy_id":r.SubjectPolicyID,
		},
	}
	_,err=o.durable.Apply(cmd,func(old state.Snapshot,_ state.Command)(state.TransitionResult,error){
		if old.State!="" { return state.TransitionResult{},ErrWrongState }
		return state.TransitionResult{State:"pending",Changed:true},nil
	})
	if err!=nil { return Effect{},err }
	return o.Get(id)
}

func (o *Outbox) Get(id string) (Effect,error) {
	if o==nil || o.durable==nil || !validDigest(id) { return Effect{},ErrInvalidEffect }
	agg:="email/"+id
	snap:=o.durable.Snapshot(agg)
	if snap.State=="" { return Effect{},ErrInvalidEffect }
	for _,record:=range o.durable.Records() {
		if record.Command.AggregateID!=agg || record.Command.Type!="email-enqueue" { continue }
		m:=record.Command.Metadata
		if m==nil { return Effect{},ErrInvalidEffect }
		// A crash after external dispatch may have delivered the message.
		// Surface it as unknown and refuse automated resubmission.
		stateName:=snap.State
		if stateName=="dispatching" {
			if _,active:=o.active.Load(id); !active { stateName="delivery_unknown" }
		}
		return Effect{
			ID:id, Recipient:m["recipient"],ArtifactSHA256:m["artifact_sha256"],
			SenderProfileID:m["sender_profile_id"],RouteLegID:m["route_leg_id"],
			SubjectPolicyID:m["subject_policy_id"],State:stateName,Generation:snap.Generation,
		},nil
	}
	return Effect{},ErrInvalidEffect
}

func (o *Outbox) Begin(id string) (Effect,error) {
	if o==nil || o.durable==nil || !validDigest(id) { return Effect{},ErrInvalidEffect }
	agg:="email/"+id
	snap:=o.durable.Snapshot(agg)
	if snap.State!="pending" { return Effect{},ErrUnresolvedDelivery }
	result,err:=o.durable.Apply(state.Command{
		Type:"email-dispatch",AggregateID:agg,IdempotencyKey:"dispatch-v1",
		SemanticFingerprint:id,ExpectedGeneration:&snap.Generation,
	},func(old state.Snapshot,_ state.Command)(state.TransitionResult,error){
		if old.State!="pending" { return state.TransitionResult{},ErrWrongState }
		return state.TransitionResult{State:"dispatching",Changed:true},nil
	})
	if err!=nil { return Effect{},err }
	// The same durable dispatch key can replay under concurrent callers that
	// both observed pending. Only the caller that committed may send mail.
	if result.Replayed { return Effect{},ErrUnresolvedDelivery }
	o.active.Store(id,true)
	return o.Get(id)
}

// Conclude must be called only after a definite provider verdict, or after an
// ambiguous post-DATA result. "delivered" means transport acceptance only.
// Automatic transitions out of delivery_unknown are not supported.
func (o *Outbox) Conclude(id, verdict string) (Effect,error) {
	if o==nil || o.durable==nil || !validDigest(id) { return Effect{},ErrInvalidEffect }
	if verdict!="delivered" && verdict!="failed" && verdict!="delivery_unknown" { return Effect{},ErrWrongState }
	agg:="email/"+id
	snap:=o.durable.Snapshot(agg)
	if snap.State!="dispatching" { return Effect{},ErrUnresolvedDelivery }
	if _,active:=o.active.Load(id); !active { return Effect{},ErrUnresolvedDelivery }
	_,err:=o.durable.Apply(state.Command{
		Type:"email-verdict",AggregateID:agg,IdempotencyKey:"verdict-v1",
		SemanticFingerprint:fmt.Sprintf("%s/%s",id,verdict),ExpectedGeneration:&snap.Generation,
	},func(old state.Snapshot,_ state.Command)(state.TransitionResult,error){
		if old.State!="dispatching" { return state.TransitionResult{},ErrWrongState }
		return state.TransitionResult{State:verdict,Changed:true},nil
	})
	if err!=nil { return Effect{},err }
	o.active.Delete(id)
	return o.Get(id)
}


// ResolveUnknown is a separate operator action. Never invoke from a sender's
// automatic retry loop; a definite external-delivery decision must be recorded.
// The reason is bounded, human-supplied evidence context, not a credential.
func (o *Outbox) ResolveUnknown(id, verdict, reason string) (Effect,error) {
	if o==nil || o.durable==nil || !validDigest(id) { return Effect{},ErrInvalidEffect }
	if verdict!="delivered" && verdict!="failed" || len(reason)==0 || len(reason)>160 || strings.ContainsAny(reason,"\r\n") {
		return Effect{},ErrWrongState
	}
	agg:="email/"+id
	snap:=o.durable.Snapshot(agg)
	if snap.State!="dispatching" && snap.State!="delivery_unknown" { return Effect{},ErrUnresolvedDelivery }
	if _,active:=o.active.Load(id); active { return Effect{},ErrUnresolvedDelivery }
	_,err:=o.durable.Apply(state.Command{
		Type:"email-operator-resolution",AggregateID:agg,IdempotencyKey:"operator-resolution-v1",
		SemanticFingerprint:fmt.Sprintf("%s/%s/%s",id,verdict,reason),ExpectedGeneration:&snap.Generation,
		Metadata:map[string]string{"reason":reason},
	},func(old state.Snapshot,_ state.Command)(state.TransitionResult,error){
		if old.State!="dispatching" && old.State!="delivery_unknown" { return state.TransitionResult{},ErrWrongState }
		return state.TransitionResult{State:verdict,Changed:true},nil
	})
	if err!=nil { return Effect{},err }
	return o.Get(id)
}
