package paperless

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

// Filing effects are independent of printing, email, and managed SMB output.
// Once submission could have occurred, missing responses must NEVER trigger
// an automatic retry, even after a process crash.
var (
 ErrApproval = errors.New("filing requires an explicit approved destination")
 ErrHold = errors.New("filing requires durable destination reconciliation")
 identRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,72}package paperless

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

// Filing effects are independent of printing, email, and managed SMB output.
// Once submission could have occurred, missing responses must NEVER trigger
// an automatic retry, even after a process crash.
var (
 ErrApproval = errors.New("filing requires an explicit approved destination")
 ErrHold = errors.New("filing requires durable destination reconciliation")
 )
)

type Intent struct {
 CanonicalSHA256 string `json:"canonical_sha256"`
 TargetID string `json:"target_id"`
 PolicyVersion string `json:"policy_version"`
 ApprovalID string `json:"approval_id"`
}

type Ticket struct {
 ID string
 Intent Intent
 State string
 TaskID string
 DocumentID int
}

type Outbox struct {
 durable *state.DurableEngine
 active sync.Map
}
func OpenOutbox(path string)(*Outbox,error){
 journal,err:=state.OpenDurableEngine(path)
 if err!=nil{return nil,err}
 return &Outbox{durable:journal},nil
}
func (o *Outbox) Close()error {
 if o==nil||o.durable==nil{return nil}
 return o.durable.Close()
}
func (i Intent) Validate()error {
 if !shaRe.MatchString(i.CanonicalSHA256) ||
 !identRe.MatchString(i.TargetID) ||
 !identRe.MatchString(i.PolicyVersion) ||
 !identRe.MatchString(i.ApprovalID) {
    return ErrApproval
 }
 return nil
}
func effectID(i Intent)string {
 b,_:=json.Marshal(i)
 h:=sha256.Sum256(append([]byte("foliorelay-paperless-filing/v1\x00"),b...))
 return hex.EncodeToString(h[:])
}
func (o *Outbox) Enqueue(i Intent)(Ticket,error){
 if o==nil||o.durable==nil{return Ticket{},ErrHold}
 if err:=i.Validate();err!=nil{return Ticket{},err}
 id:=effectID(i)
 b,_:=json.Marshal(i)
 _,err:=o.durable.Apply(state.Command{
   Type:"paperless-enqueue",AggregateID:"paperless/"+id,
   IdempotencyKey:"enqueue-v1",SemanticFingerprint:id,
   Metadata:map[string]string{"intent":string(b)},
 },func(previous state.Snapshot,_ state.Command)(state.TransitionResult,error){
    if previous.State!="" {return state.TransitionResult{},ErrHold}
    return state.TransitionResult{State:"approved",Changed:true},nil
 })
 if err!=nil{return Ticket{},err}
 return o.Get(id)
}
func (o *Outbox) Get(id string)(Ticket,error){
 if o==nil||o.durable==nil||!shaRe.MatchString(id){return Ticket{},ErrHold}
 aggregate:="paperless/"+id
 snap:=o.durable.Snapshot(aggregate)
 if snap.State==""{return Ticket{},ErrHold}
 ticket:=Ticket{ID:id,State:snap.State}
 if snap.State=="submitting" {
    if _,ok:=o.active.Load(id);!ok {ticket.State="destination_unknown"}
 }
 for _,record:=range o.durable.Records(){
    if record.Command.AggregateID!=aggregate {continue}
    if record.Command.Type=="paperless-enqueue" {
      if err:=json.Unmarshal([]byte(record.Command.Metadata["intent"]),&ticket.Intent);err!=nil{return Ticket{},err}
    }
    if record.Command.Type=="paperless-accepted" {ticket.TaskID=record.Command.Metadata["task_id"]}
    if record.Command.Type=="paperless-terminal" {
      raw:=record.Command.Metadata["document_id"]
      if _,err:=fmt.Sscanf(raw,"%d",&ticket.DocumentID);err!=nil{return Ticket{},ErrHold}
    }
 }
 if ticket.Intent.CanonicalSHA256==""{return Ticket{},ErrHold}
 return ticket,nil
}
func (o *Outbox) Begin(id string)(Ticket,error){
 if o==nil||o.durable==nil||!shaRe.MatchString(id){return Ticket{},ErrHold}
 agg:="paperless/"+id
 snap:=o.durable.Snapshot(agg)
 if snap.State!="approved"{return Ticket{},ErrHold}
 _,err:=o.durable.Apply(state.Command{
   Type:"paperless-submit",AggregateID:agg,IdempotencyKey:"submit-v1",
   SemanticFingerprint:id,ExpectedGeneration:&snap.Generation,
 },func(previous state.Snapshot,_ state.Command)(state.TransitionResult,error){
   if previous.State!="approved"{return state.TransitionResult{},ErrHold}
   return state.TransitionResult{State:"submitting",Changed:true},nil
 })
 if err!=nil{return Ticket{},err}
 o.active.Store(id,true)
 return o.Get(id)
}
// Accept is called only after Paperless returned a valid task UUID. No API
// call occurs inside this durable transition.
func (o *Outbox) Accept(id,taskID string)(Ticket,error){
 if o==nil||o.durable==nil||!shaRe.MatchString(id)||!uuidRe.MatchString(taskID){return Ticket{},ErrHold}
 if _,active:=o.active.Load(id);!active{return Ticket{},ErrHold}
 agg:="paperless/"+id
 snap:=o.durable.Snapshot(agg)
 if snap.State!="submitting"{return Ticket{},ErrHold}
 _,err:=o.durable.Apply(state.Command{
   Type:"paperless-accepted",AggregateID:agg,IdempotencyKey:"accepted-v1",
   SemanticFingerprint:id+"/"+taskID,ExpectedGeneration:&snap.Generation,
   Metadata:map[string]string{"task_id":taskID},
 },func(previous state.Snapshot,_ state.Command)(state.TransitionResult,error){
   if previous.State!="submitting"{return state.TransitionResult{},ErrHold}
   return state.TransitionResult{State:"accepted",Changed:true},nil
 })
 if err!=nil{return Ticket{},err}
 o.active.Delete(id)
 return o.Get(id)
}
// MarkTask persists only a terminal, observed native Paperless task result.
// Missing, pending, malformed and ambiguous outcomes are never marked done.
func (o *Outbox) MarkTask(id string,task Task)(Ticket,error){
 ticket,err:=o.Get(id)
 if err!=nil{return Ticket{},err}
 if ticket.State!="accepted"||ticket.TaskID!=task.TaskID{return Ticket{},ErrHold}
 final:=""
 targetDocument:=task.DocumentID
 switch task.Outcome{
 case "archived":
   if targetDocument>0&&task.DuplicateOf==0{final="archived"}
 case "duplicate":
   if task.DuplicateOf>0&&task.DocumentID==0{
     final="duplicate";targetDocument=task.DuplicateOf
   }
 case "rejected": final="rejected"
 }
 if final=="" {return Ticket{},ErrHold}
 agg:="paperless/"+id
 snap:=o.durable.Snapshot(agg)
 if snap.State!="accepted"{return Ticket{},ErrHold}
 _,err=o.durable.Apply(state.Command{
   Type:"paperless-terminal",AggregateID:agg,IdempotencyKey:"terminal-v1",
   SemanticFingerprint:fmt.Sprintf("%s/%s/%d",id,final,targetDocument),
   ExpectedGeneration:&snap.Generation,
   Metadata:map[string]string{"document_id":fmt.Sprintf("%d",targetDocument)},
 },func(previous state.Snapshot,_ state.Command)(state.TransitionResult,error){
    if previous.State!="accepted"{return state.TransitionResult{},ErrHold}
    return state.TransitionResult{State:final,Changed:true},nil
 })
 if err!=nil{return Ticket{},err}
 return o.Get(id)
}
