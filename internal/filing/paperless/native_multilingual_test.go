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

 "github.com/SemperSupra/folio-relay/internal/filing/bench"
)

// Manual curated baseline. The same 24-case labeled holdout defined in
// native_benchmark_test.go is reused without modifying its expected labels.
// Regexes are hand-authored, not learned from the test observations.
var multilingualRules=[]struct{Kind,Pattern string}{
 {"invoice","INVOICE|RECHNUNG|FACTURE|FACTURA"},
 {"insurance","POLICY|VERSICHERUNG|POLICE|SEGURO"},
 {"lease","LEASE|MIETVERTRAG|BAIL|ARRENDAMIENTO"},
 {"appointment","APPOINTMENT|TERMIN|RENDEZVOUS|CITA"},
 {"statement","STATEMENT|KONTOAUSZUG|RELEVE|EXTRACTO"},
 {"registration","REGISTRATION|ANMELDUNG|INSCRIPTION|MATRICULA"},
}

// These contradict the naive keyword-matching assumption. A high-confidence
// wrong filing decision here disqualifies automatic filing by regex alone.
var negativeSamples=[]string{
 "NOTICE: this is NOT AN INVOICE. The invoice example is void, purely advertising.",
 "EXPLANATION: this is NOT AN INSURANCE POLICY. This describes a generic product.",
 "DECLARATION: NO LEASE OR RENTAL CONTRACT was entered into.",
 "SCHEDULING NOTE: NO APPOINTMENT is booked. Examples are informational.",
 "EXPLANATORY TEXT: this is NOT A BANK STATEMENT, only sample text.",
 "NOTICE: NO REGISTRATION was submitted. This notice is not an enrollment record.",
}

func TestNativeMultilingualRulesBenchmark(t *testing.T){
 endpoint:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_URL")
 token:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_TOKEN")
 if endpoint==""||token==""{t.Skip("disposable synthetic test only")}
 c,err:=NewClient(Config{BaseURL:endpoint,Token:token,TestOnlyLoopbackHTTP:true},nil)
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),17*time.Minute)
 defer cancel()
 ids:=make(map[string]int)
 for _,rule:=range multilingualRules {
   request:=map[string]any{"name":"Synthetic curated "+rule.Kind,"matching_algorithm":4,"match":rule.Pattern,"is_insensitive":true}
   body,err:=json.Marshal(request)
   if err!=nil{t.Fatal(err)}
   resp,err:=c.do(ctx,http.MethodPost,"/api/document_types/","application/json",bytes.NewReader(body))
   if err!=nil{t.Fatalf("creating synthetic rule %s: %v",rule.Kind,err)}
   var value struct{ID int `json:"id"`}
   decErr:=json.NewDecoder(resp.Body).Decode(&value)
   resp.Body.Close()
   if decErr!=nil||value.ID<=0{t.Fatalf("invalid API rule creation %s: %v",rule.Kind,decErr)}
   ids[rule.Kind]=value.ID
 }
 var observations []bench.Sample
 var assignedCorrect,assignedWrong,assignedEmpty int
 for i,item:=range nativeLabelCorpus {
   pdf:=synthPDF(fmt.Sprintf("FolioRelay synthetic classified document #%02d :: %s",i+1,item.Content))
   hash:=sha256.Sum256(pdf)
   doc:=Document{Content:pdf,SHA256:hex.EncodeToString(hash[:]),MediaType:"application/pdf"}
   t0:=time.Now()
   taskID,err:=c.Upload(ctx,doc)
   if err!=nil{t.Fatalf("upload sample %d: %v",i+1,err)}
   var task Task
   for {
      if err:=ctx.Err();err!=nil{t.Fatalf("benchmark deadline sample=%d last=%s/%s: %v",i+1,task.Status,task.Outcome,err)}
      task,err=c.Observe(ctx,taskID)
      if err!=nil{t.Fatalf("observe sample %d: %v",i+1,err)}
      if task.Outcome=="archived"||task.Outcome=="duplicate"||task.Outcome=="rejected"{break}
      time.Sleep(1*time.Second)
   }
   if task.Outcome!="archived"{t.Fatalf("synthetic document %d could not be archived: %s status=%s",i+1,task.Outcome,task.Status)}
   record,err:=c.ReadRecord(ctx,task.DocumentID)
   if err!=nil{t.Fatalf("native record %d: %v",i+1,err)}
   suggestions,err:=c.ReadClassicSuggestions(ctx,task.DocumentID)
   if err!=nil{t.Fatalf("native suggestions %d: %v",i+1,err)}
   expected:=ids[item.Kind]
   var actual int
   if record.DocumentType!=nil{actual=*record.DocumentType}
   switch {
   case actual==0:assignedEmpty++
   case actual==expected:assignedCorrect++
   default:assignedWrong++
   }
   observations=append(observations,bench.Sample{
     CaseID:fmt.Sprintf("%s-%s",item.Kind,item.Language),
     ExpectedTypeID:expected,
     SuggestedTypeIDs:suggestions.DocumentTypes,
     // No automatic FolioRelay action occurred. Paperless' native assigned
     // type is scored separately, never misreported as an advisor action.
     AppliedTypeID:0,
     HumanCorrections:0,
     LatencyMilliseconds:time.Since(t0).Milliseconds(),
   })
 }
 // Contradiction/negative controls: keyword presence is not necessarily
 // semantic membership. They do not belong to any of the six target types.
 var unsafeAssignments,unsafeSuggestions int
 for i,content:=range negativeSamples {
   pdf:=synthPDF(fmt.Sprintf("Synthetic negative control #%02d :: %s",i+1,content))
   sum:=sha256.Sum256(pdf)
   doc:=Document{Content:pdf,SHA256:hex.EncodeToString(sum[:]),MediaType:"application/pdf"}
   taskID,err:=c.Upload(ctx,doc)
   if err!=nil{t.Fatalf("negative upload %d: %v",i,err)}
   var task Task
   for {
      if err:=ctx.Err();err!=nil{t.Fatalf("negative fixture timeout %d: %v",i,err)}
      task,err=c.Observe(ctx,taskID)
      if err!=nil{t.Fatal(err)}
      if task.Outcome=="archived"||task.Outcome=="duplicate"||task.Outcome=="rejected"{break}
      time.Sleep(time.Second)
   }
   if task.Outcome!="archived"{t.Fatalf("negative fixture did not archive %d: %s",i,task.Outcome)}
   record,err:=c.ReadRecord(ctx,task.DocumentID)
   if err!=nil{t.Fatal(err)}
   suggested,err:=c.ReadClassicSuggestions(ctx,task.DocumentID)
   if err!=nil{t.Fatal(err)}
   if record.DocumentType!=nil&&*record.DocumentType>0{unsafeAssignments++}
   if len(suggested.DocumentTypes)>0{unsafeSuggestions++}
 }
 t.Logf("FOLIO_MULTILINGUAL_NEGATIVE_METRICS={\"cases\":%d,\"unexpected_native_assignments\":%d,\"unsafe_classic_suggestions\":%d}",len(negativeSamples),unsafeAssignments,unsafeSuggestions)
 scores,err:=bench.Evaluate(observations)
 if err!=nil{t.Fatal(err)}
 report:=struct {
   Version string `json:"version"`
   CorpusCount int `json:"corpus_count"`
   Languages []string `json:"languages"`
   NativeAssignedCorrect int `json:"native_assigned_correct"`
   NativeAssignedWrong int `json:"native_assigned_wrong"`
   NativeAssignedEmpty int `json:"native_assigned_empty"`
   ClassicSuggestions bench.Metrics `json:"classic_suggestions"`
 }{"v3.1.3",len(observations),[]string{"en","de","fr","es"},assignedCorrect,assignedWrong,assignedEmpty,scores}
 data,err:=json.Marshal(report)
 if err!=nil{t.Fatal(err)}
 t.Logf("FOLIO_MULTILINGUAL_RULES_METRICS=%s",data)
 if len(observations)!=24 {t.Fatalf("missing holdout cases: %d",len(observations))}
}
