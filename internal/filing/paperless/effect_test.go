package paperless

import (
 "errors"
 "path/filepath"
 "strings"
 "testing"


)
func testIntent() Intent {
 return Intent{
   CanonicalSHA256:strings.Repeat("a",64),
   TargetID:"paperless-main",PolicyVersion:"v1",
   ApprovalID:"user-approved-001",
 }
}
func TestExplicitApprovalAndStableEnqueue(t *testing.T){
 o,err:=OpenOutbox(filepath.Join(t.TempDir(),"journal.frj"))
 if err!=nil{t.Fatal(err)}
 defer o.Close()
 i:=testIntent()
 a,err:=o.Enqueue(i)
 if err!=nil||a.State!="approved"{t.Fatalf("enqueue %#v %v",a,err)}
 b,err:=o.Enqueue(i)
 if err!=nil||a!=b{t.Fatalf("non-idempotent enqueue %#v %#v %v",a,b,err)}
 i.ApprovalID="user-approved-002"
 reapproved,err:=o.Enqueue(i)
 if err!=nil||reapproved.ID!=a.ID||reapproved.Intent.ApprovalID!="user-approved-001"{
   t.Fatalf("repeat approval must return original single durable effect: %#v %v",reapproved,err)
 }
 i.TargetID="paperless-alternate"
 changed,err:=o.Enqueue(i)
 if err!=nil||changed.ID==a.ID{t.Fatalf("new authorized destination must have distinct identity %#v %v",changed,err)}
 i.ApprovalID=""
 if _,err:=o.Enqueue(i);!errors.Is(err,ErrApproval){t.Fatalf("unauthorized intent %v",err)}
}

func TestCrashBeforeTaskReceiptStopsReplay(t *testing.T) {
 file:=filepath.Join(t.TempDir(),"journal.frj")
 o,err:=OpenOutbox(file)
 if err!=nil {t.Fatal(err)}
 item,err:=o.Enqueue(testIntent())
 if err!=nil {t.Fatal(err)}
 reserved,err:=o.Begin(item.ID)
 if err!=nil||reserved.State!="submitting"{t.Fatalf("reservation %#v %v",reserved,err)}
 if _,err:=o.Begin(item.ID);!errors.Is(err,ErrHold){t.Fatalf("double send %v",err)}
 if err:=o.Close();err!=nil{t.Fatal(err)}
 recovered,err:=OpenOutbox(file)
 if err!=nil{t.Fatal(err)}
 defer recovered.Close()
 unknown,err:=recovered.Get(item.ID)
 if err!=nil||unknown.State!="destination_unknown"{t.Fatalf("recovered %#v %v",unknown,err)}
 if _,err:=recovered.Begin(item.ID);!errors.Is(err,ErrHold){t.Fatalf("resubmitted unknown %v",err)}
 if _,err:=recovered.Accept(item.ID,sampleTaskID);!errors.Is(err,ErrHold){t.Fatalf("forged acceptance %v",err)}
}

func TestAcceptedTaskCanReconcileAfterRestart(t *testing.T) {
 file:=filepath.Join(t.TempDir(),"journal.frj")
 o,err:=OpenOutbox(file)
 if err!=nil {t.Fatal(err)}
 item,err:=o.Enqueue(testIntent())
 if err!=nil {t.Fatal(err)}
 if _,err:=o.Begin(item.ID);err!=nil{t.Fatal(err)}
 accepted,err:=o.Accept(item.ID,sampleTaskID)
 if err!=nil||accepted.State!="accepted"||accepted.TaskID!=sampleTaskID{t.Fatalf("accepted %#v %v",accepted,err)}
 if err:=o.Close();err!=nil{t.Fatal(err)}
 reopened,err:=OpenOutbox(file)
 if err!=nil {t.Fatal(err)}
 defer reopened.Close()
 cur,err:=reopened.Get(item.ID)
 if err!=nil||cur.State!="accepted"||cur.TaskID!=sampleTaskID{t.Fatalf("recovery %#v %v",cur,err)}
 if _,err:=reopened.MarkTask(item.ID,Task{TaskID:sampleTaskID,Outcome:"pending"});!errors.Is(err,ErrHold){t.Fatalf("pending as final: %v",err)}
 archived,err:=reopened.MarkTask(item.ID,Task{TaskID:sampleTaskID,Status:"SUCCESS",Outcome:"archived",DocumentID:74})
 if err!=nil||archived.State!="archived"||archived.DocumentID!=74{t.Fatalf("terminal %#v %v",archived,err)}
 if _,err:=reopened.Begin(item.ID);!errors.Is(err,ErrHold){t.Fatalf("double submit final state %v",err)}
 if _,err:=reopened.MarkTask(item.ID,Task{TaskID:sampleTaskID,Outcome:"archived",DocumentID:74});!errors.Is(err,ErrHold){t.Fatalf("double mark final state %v",err)}
}

func TestDuplicateDetectionKeepsRemoteIdentity(t *testing.T){
 o,err:=OpenOutbox(filepath.Join(t.TempDir(),"journal.frj"))
 if err!=nil{t.Fatal(err)}
 defer o.Close()
 item,err:=o.Enqueue(testIntent())
 if err!=nil{t.Fatal(err)}
 if _,err:=o.Begin(item.ID);err!=nil{t.Fatal(err)}
 if _,err:=o.Accept(item.ID,sampleTaskID);err!=nil{t.Fatal(err)}
 result,err:=o.MarkTask(item.ID,Task{TaskID:sampleTaskID,Outcome:"duplicate",DuplicateOf:11})
 if err!=nil||result.State!="duplicate"||result.DocumentID!=11{t.Fatalf("duplicate %#v %v",result,err)}
}

func TestAdversarialIncompleteTaskAndBadApproval(t *testing.T){
 o,err:=OpenOutbox(filepath.Join(t.TempDir(),"journal.frj"))
 if err!=nil{t.Fatal(err)}
 defer o.Close()
 i:=testIntent()
 i.TargetID="../other-target"
 if _,err:=o.Enqueue(i);!errors.Is(err,ErrApproval){t.Fatalf("path-like target accepted: %v",err)}
 i=testIntent()
 i.CanonicalSHA256="garbage"
 if _,err:=o.Enqueue(i);!errors.Is(err,ErrApproval){t.Fatalf("invalid digest accepted: %v",err)}
 item,err:=o.Enqueue(testIntent())
 if err!=nil{t.Fatal(err)}
 if _,err:=o.Begin(item.ID);err!=nil{t.Fatal(err)}
 if _,err:=o.Accept(item.ID,"not-a-uuid");err==nil{t.Fatal("malformed task accepted")}
 if _,err:=o.Accept(item.ID,sampleTaskID);err!=nil{t.Fatal(err)}
 for _,task:=range []Task{
   {TaskID:"wrong",Outcome:"archived",DocumentID:1},
   {TaskID:sampleTaskID,Outcome:"archived",DocumentID:0},
   {TaskID:sampleTaskID,Outcome:"duplicate",DocumentID:1,DuplicateOf:2},
   {TaskID:sampleTaskID,Outcome:"unknown",DocumentID:9},
 }{
   if _,err:=o.MarkTask(item.ID,task);!errors.Is(err,ErrHold){t.Fatalf("accepted invalid verdict %#v: %v",task,err)}
 }
}

func TestEffectJournalContainsNoContentOrCredentials(t *testing.T){
 o,err:=OpenOutbox(filepath.Join(t.TempDir(),"journal.frj"))
 if err!=nil{t.Fatal(err)}
 defer o.Close()
 if _,err:=o.Enqueue(testIntent());err!=nil{t.Fatal(err)}
 rows:=o.durable.Records()
 if len(rows)!=1||rows[0].Command.Metadata["intent"]==""{t.Fatal("no durable metadata")}
 if rows[0].Command.Metadata["document"]!=""||rows[0].Command.Metadata["token"]!=""{t.Fatal("credentials or document persisted")}
}
