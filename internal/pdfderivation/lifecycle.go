package pdfderivation

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "strings"
)

// OutputKind selects a *derived* PDF rendition, independent of its immutable
// canonical original. A searchable rendition need not prove visible edits;
// an editable rendition must pass that stronger visible-object oracle.
type OutputKind string
const (
    SearchablePDF OutputKind = "searchable-pdf"
    EditablePDF OutputKind = "reconstructed-editable-pdf"
)

type Spec struct {
    SourceSHA256 string `json:"source_sha256"`
    SourceMIME string `json:"source_mime"`
    ProcessorProfile string `json:"processor_profile"`
    OutputKind OutputKind `json:"output_kind"`
}

func (s Spec) Key() (string,error) {
    if !digestValid(s.SourceSHA256) {
        return "",errors.New("invalid_source_digest")
    }
    if s.SourceMIME!=SourcePDF && s.SourceMIME!=SourceURF {
        return "",errors.New("unsupported_source_mime")
    }
    if len(s.ProcessorProfile)<1 || len(s.ProcessorProfile)>180 ||
       strings.TrimSpace(s.ProcessorProfile)!=s.ProcessorProfile {
        return "",errors.New("invalid_processor_profile")
    }
    for _,r:=range s.ProcessorProfile {
        if r<0x20 || r==0x7f {return "",errors.New("invalid_processor_profile")}
    }
    if s.OutputKind!=SearchablePDF && s.OutputKind!=EditablePDF {
        return "",errors.New("unsupported_pdf_derivation_kind")
    }
    // JSON struct field order is fixed; length delimiting prevents ambiguous
    // multi-field concatenation. Domain prefix prevents cross-product hash
    // ambiguity. This identifier MUST NOT be used as a filesystem path.
    data,err:=json.Marshal(struct {
        Version int `json:"version"`
        SourceSHA256 string `json:"source_sha256"`
        SourceMIME string `json:"source_mime"`
        ProcessorProfile string `json:"processor_profile"`
        OutputKind OutputKind `json:"output_kind"`
    }{1,s.SourceSHA256,s.SourceMIME,s.ProcessorProfile,s.OutputKind})
    if err!=nil{return "",errors.New("invalid_derivation_spec")}
    sum:=sha256.Sum256(append([]byte("foliorelay/pdf-derivation/v1\x00"),data...))
    return "pdf-derivation/v1/"+hex.EncodeToString(sum[:]),nil
}

type JobState string
const (
    JobPending JobState="pending"
    JobRunning JobState="running"
    JobNeedsReview JobState="needs_review"
    JobReady JobState="ready"
    JobFailed JobState="failed"
    JobCancelled JobState="cancelled"
)

type JobAction string
const (
    Start JobAction="start"
    Complete JobAction="complete"
    RequireReview JobAction="require_review"
    Fail JobAction="fail"
    Cancel JobAction="cancel"
)

// JobEvent is a proposed append-only state record, not an actual effect
// authority. The future durable store must authenticate and authorize callers
// *before* writing an event; this reducer does NOT grant that authority.
type JobEvent struct {
    ID string `json:"id"`
    Key string `json:"key"`
    Sequence uint64 `json:"sequence"`
    Action JobAction `json:"action"`
    ReasonCode string `json:"reason_code,omitempty"`
    Receipt *Receipt `json:"receipt,omitempty"`
}

type Projection struct {
    Key string `json:"key"`
    State JobState `json:"state"`
    Revision uint64 `json:"revision"`
    ReasonCode string `json:"reason_code,omitempty"`
    Receipt *Receipt `json:"receipt,omitempty"`
    NextActions []JobAction `json:"next_actions"`
}

func next(state JobState) []JobAction {
    switch state {
    case JobPending:return []JobAction{Start,Cancel}
    case JobRunning:return []JobAction{Complete,RequireReview,Fail,Cancel}
    case JobNeedsReview:return []JobAction{Start,Cancel}
    default:return []JobAction{}
    }
}

// Replay has no I/O and no provider side effects. Exact repeated events are
// idempotent; inconsistent duplicate IDs or sequence gaps fail closed.
// Completed/failed/cancelled states cannot silently retry or overwrite an
// already accepted artifact. A fresh processor profile or new attempt must
// be explicitly represented in a future separately reviewed work contract.
func Replay(spec Spec, events []JobEvent) (Projection,error) {
    key,err:=spec.Key()
    if err!=nil{return Projection{},err}
    state:=Projection{Key:key,State:JobPending,NextActions:next(JobPending)}
    seen:=map[string][]byte{}
    for _,event:=range events {
        if len(event.ID)<1 || len(event.ID)>128 || strings.ContainsAny(event.ID,"\r\n\x00") {
            return Projection{},errors.New("invalid_event_identity")
        }
        if event.Key!=key {return Projection{},errors.New("derivation_key_mismatch")}
        wire,err:=json.Marshal(event)
        if err!=nil{return Projection{},errors.New("invalid_derivation_event")}
        if prior,ok:=seen[event.ID];ok {
            if string(prior)!=string(wire) {
                return Projection{},errors.New("idempotency_conflict")
            }
            continue
        }
        if event.Sequence!=state.Revision+1 {
            return Projection{},errors.New("derivation_sequence_gap")
        }
        var newState JobState
        switch event.Action {
        case Start:
            if state.State!=JobPending && state.State!=JobNeedsReview {
                return Projection{},errors.New("invalid_derivation_transition")
            }
            if event.Receipt!=nil || event.ReasonCode!="" {
                return Projection{},errors.New("invalid_start_payload")
            }
            newState=JobRunning
        case Complete:
            if state.State!=JobRunning || event.Receipt==nil ||
                event.ReasonCode!="" {
                return Projection{},errors.New("invalid_complete_transition")
            }
            r:=event.Receipt
            if err:=r.ValidateReady();err!=nil{return Projection{},err}
            if r.SourceSHA256!=spec.SourceSHA256 ||
               r.SourceMIME!=spec.SourceMIME ||
               r.ProcessorProfile!=spec.ProcessorProfile ||
               (spec.OutputKind==EditablePDF && r.Editability==SearchableImage) {
                return Projection{},errors.New("pdf_result_does_not_match_derivation")
            }
            newState=JobReady
            copied:=*r
            state.Receipt=&copied
        case RequireReview,Fail:
            if state.State!=JobRunning || event.Receipt!=nil ||
               event.ReasonCode=="" || len(event.ReasonCode)>128 {
                return Projection{},errors.New("invalid_failure_or_review_payload")
            }
            if event.Action==RequireReview {newState=JobNeedsReview
            } else {newState=JobFailed}
        case Cancel:
            if state.State!=JobPending && state.State!=JobRunning &&
               state.State!=JobNeedsReview {
                return Projection{},errors.New("invalid_derivation_transition")
            }
            if event.Receipt!=nil {return Projection{},errors.New("invalid_cancel_payload")}
            newState=JobCancelled
        default:
            return Projection{},errors.New("unknown_derivation_action")
        }
        seen[event.ID]=wire
        state.State=newState
        state.Revision=event.Sequence
        state.ReasonCode=event.ReasonCode
        state.NextActions=next(newState)
    }
    return state,nil
}
