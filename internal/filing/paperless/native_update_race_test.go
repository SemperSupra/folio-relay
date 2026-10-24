//go:build paperless_integration

package paperless

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "net/http"
 "os"
 "testing"
 "time"
)

// TestPaperless313ConditionalUpdateRace is a NEGATIVE interoperability test:
// it intentionally PATCHes one disposable synthetic document using a bogus
// If-Match revision precondition. It NEVER uses a real document or target.
// Only the disposable hosted Paperless 3.1.3 Docker fixture can enable it.
func TestPaperless313ConditionalUpdateRace(t *testing.T){
 addr:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_URL")
 token:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_TOKEN")
 if addr==""||token=="" {t.Skip("disposable pinned Paperless fixture required")}
 c,err:=NewClient(Config{BaseURL:addr,Token:token,TestOnlyLoopbackHTTP:true},nil)
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),3*time.Minute)
 defer cancel()
 payload:=synthPDF("DISPOSABLE SYNTHETIC PAPERLESS RACE CASE ONLY")
 hash:=sha256.Sum256(payload)
 tid,err:=c.Upload(ctx,Document{
   Content:payload,SHA256:hex.EncodeToString(hash[:]),MediaType:"application/pdf",
 })
 if err!=nil{t.Fatal(err)}
 var task Task
 for {
   if err:=ctx.Err();err!=nil{t.Fatalf("disposable archive timeout: %v",err)}
   task,err=c.Observe(ctx,tid)
   if err!=nil{t.Fatal(err)}
   if task.Outcome=="archived"||task.Outcome=="duplicate"||task.Outcome=="rejected"{break}
   time.Sleep(time.Second)
 }
 if task.Outcome!="archived"{t.Fatalf("disposable doc not archived: %s",task.Outcome)}
 original,err:=c.ReadRecord(ctx,task.DocumentID)
 if err!=nil{t.Fatal(err)}
 // Both independent test clients observe the *same* initial version; the
 // second does not refresh after the first writes. Send a deliberately invalid
 // If-Match to detect whether the server checks precondition headers.
 sendPatch:=func(title string)int{
   body,_:=json.Marshal(map[string]string{"title":title})
   req,err:=http.NewRequestWithContext(ctx,http.MethodPatch,
       c.endpoint+fmt.Sprintf("/api/documents/%d/",task.DocumentID),
       bytes.NewReader(body))
   if err!=nil{t.Fatal(err)}
   req.Header.Set("Authorization","Token "+c.token)
   req.Header.Set("Accept","application/json; version=10")
   req.Header.Set("Content-Type","application/json")
   req.Header.Set("If-Match",`"bogus-etag-from-stale-reader"`)
   resp,err:=c.httpClient.Do(req)
   if err!=nil{t.Fatal(err)}
   defer resp.Body.Close()
   return resp.StatusCode
 }
 first,second:=sendPatch("Synthetic reviewer A"),sendPatch("Synthetic reviewer B")
 last,err:=c.ReadRecord(ctx,task.DocumentID)
 if err!=nil{t.Fatal(err)}
 report:=struct{
   Source string `json:"source"`
   OriginalNativeTitlePresent bool `json:"original_native_title_present"`
   FirstStatus int `json:"first_status"`
   SecondStatus int `json:"second_status"`
   LastWriterWon bool `json:"last_writer_won"`
   IfMatchRejected bool `json:"if_match_rejected"`
   SyntheticOnly bool `json:"synthetic_only"`
 }{"paperless-ngx-3.1.3",original.Title!="",first,second,last.Title=="Synthetic reviewer B",
      first==http.StatusPreconditionFailed||second==http.StatusPreconditionFailed,true}
 b,err:=json.Marshal(report)
 if err!=nil{t.Fatal(err)}
 t.Logf("FOLIO_PAPERLESS_METADATA_WRITE_RACE=%s",b)
 // The observed implementation is currently non-conditional. Treat any
 // difference as new vendor behavior requiring fresh review, not success.
 if first<200||first>=300||second<200||second>=300||!report.LastWriterWon {
   t.Fatalf("Paperless conditional-update behavior changed; review upstream, statuses %d/%d last title matched=%v",first,second,report.LastWriterWon)
 }
}
