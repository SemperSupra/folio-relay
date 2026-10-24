//go:build paperless_integration

package paperless

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

// synthPDF constructs a valid one-page PDF with deterministic selectable text.
// No personal or licensed input is used in the disposable end-to-end test.
func synthPDF(content string) []byte {
 escaped:=strings.NewReplacer("\\","\\\\","(","\\(",")","\\)","\r"," ","\n"," ").Replace(content)
 stream:=fmt.Sprintf("BT /F1 18 Tf 50 740 Td (%s) Tj ET\n",escaped)
 objects:=[]string{
  "<< /Type /Catalog /Pages 2 0 R >>",
  "<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
  fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream",len(stream),stream),
 }
 var pdf bytes.Buffer
 pdf.WriteString("%PDF-1.4\n")
 offsets:=[]int{0}
 for index,obj:=range objects {
    offsets=append(offsets,pdf.Len())
    fmt.Fprintf(&pdf,"%d 0 obj\n%s\nendobj\n",index+1,obj)
 }
 xref:=pdf.Len()
 fmt.Fprintf(&pdf,"xref\n0 %d\n0000000000 65535 f \n",len(objects)+1)
 for _,v:=range offsets[1:] {fmt.Fprintf(&pdf,"%010d 00000 n \n",v)}
 fmt.Fprintf(&pdf,"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",len(objects)+1,xref)
 return pdf.Bytes()
}

func TestDisposableRealPaperless313(t *testing.T){
 addr:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_URL")
 token:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_TOKEN")
 if addr==""||token==""{t.Skip("only explicitly activated disposable hosted Paperless test")}
 c,err:=NewClient(Config{BaseURL:addr,Token:token,TestOnlyLoopbackHTTP:true},nil)
 if err!=nil{t.Fatal(err)}
 content:=synthPDF("TEST INVOICE 2026-10-24 COMPANY ALPHA DEMO ONLY")
 sum:=sha256.Sum256(content)
 doc:=Document{Content:content,SHA256:hex.EncodeToString(sum[:]),MediaType:"application/pdf"}
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute)
 defer cancel()
 t0:=time.Now()
 journalPath:=filepath.Join(t.TempDir(),"filing.frj")
 outbox,err:=OpenOutbox(journalPath)
 if err!=nil{t.Fatal(err)}
 defer outbox.Close()
 filing:=Intent{
   CanonicalSHA256:doc.SHA256,
   TargetID:"paperless-fixture",
   PolicyVersion:"v1",
   ApprovalID:"approved-fixture-1",
 }
 ticket,err:=outbox.Enqueue(filing)
 if err!=nil{t.Fatal(err)}
 if _,err:=outbox.Begin(ticket.ID);err!=nil{t.Fatal(err)}
 taskID,err:=c.Upload(ctx,doc)
 if err!=nil{t.Fatalf("upload into pinned fixture: %v",err)}
 if _,err:=outbox.Accept(ticket.ID,taskID);err!=nil{t.Fatalf("durable task receipt: %v",err)}
 t.Logf("synthetic bytes=%d task accepted=true",len(doc.Content))
 var observed Task
 for {
  if err:=ctx.Err();err!=nil{t.Fatalf("timed out waiting for Paperless task: %v",err)}
  observed,err=c.Observe(ctx,taskID)
  if err!=nil{t.Fatalf("task API v10 contract: %v",err)}
  if observed.Outcome=="archived"||observed.Outcome=="duplicate"||observed.Outcome=="rejected" {break}
  time.Sleep(2*time.Second)
 }
 if observed.Outcome!="archived"||observed.DocumentID<=0 {
    t.Fatalf("Paperless did not archive valid generated PDF, status=%s outcome=%s",observed.Status,observed.Outcome)
 }
 if final,err:=outbox.MarkTask(ticket.ID,observed);err!=nil||final.State!="archived"{
   t.Fatalf("durable archive receipt %#v: %v",final,err)
 }
 record,err:=c.ReadRecord(ctx,observed.DocumentID)
 if err!=nil{t.Fatalf("native metadata readback: %v",err)}
 if record.ID!=observed.DocumentID||record.Title=="" {t.Fatalf("invalid native record: %#v",record)}
 classic,err:=c.ReadClassicSuggestions(ctx,observed.DocumentID)
 if err!=nil {t.Fatalf("native classic suggestions contract: %v",err)}
 t.Logf("archived=true native_title_present=true tags=%d classic_type_candidates=%d seconds=%.2f",len(record.Tags),len(classic.DocumentTypes),time.Since(t0).Seconds())
 // Confirm that a restarted actor sees the previously committed result.
 if err:=outbox.Close();err!=nil{t.Fatal(err)}
 recovery,err:=OpenOutbox(journalPath)
 if err!=nil{t.Fatal(err)}
 defer recovery.Close()
 recovered,err:=recovery.Get(ticket.ID)
 if err!=nil||recovered.State!="archived"||recovered.DocumentID!=record.ID{
   t.Fatalf("durable end-to-end recovery %#v %v",recovered,err)
 }
 // Repeat the identical bytes as a brand-new explicitly authorized intent
 // to establish the DMS' duplicate handling without damaging its original.
 filing.ApprovalID="approved-fixture-2"
 secondEffect,err:=recovery.Enqueue(filing)
 if err!=nil{t.Fatal(err)}
 if _,err:=recovery.Begin(secondEffect.ID);err!=nil{t.Fatal(err)}
 task2,err:=c.Upload(ctx,doc)
 if err!=nil {t.Fatalf("duplicate attempt did not yield a verifiable Paperless task: %v",err)}
 if _,err:=recovery.Accept(secondEffect.ID,task2);err!=nil{t.Fatal(err)}
 for {
  if err:=ctx.Err();err!=nil{t.Fatalf("duplicate outcome timeout: %v",err)}
  second,err:=c.Observe(ctx,task2)
  if err!=nil{t.Fatal(err)}
  if second.Outcome=="duplicate" || second.Outcome=="rejected" {
    final,err:=recovery.MarkTask(secondEffect.ID,second)
    if err!=nil{t.Fatalf("durable duplicate/rejection: %v",err)}
    t.Logf("duplicate candidate no archive outcome=%s durable=%s",second.Outcome,final.State)
    break
  }
  if second.Outcome=="archived" {
     t.Fatalf("duplicate created a second archived doc id=%d",second.DocumentID)
  }
  time.Sleep(2*time.Second)
 }
}
