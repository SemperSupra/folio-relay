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

// All documents are generated exclusively for this experiment. Rules are
// seeded with only the English training lexemes; the held-out documents use
// independently selected English, German, French and Spanish alternatives.
// Standard PDF Helvetica is an ASCII-only fixture here; image/Thai coverage
// is a separate qualification, not silently claimed by this run.
type labelCase struct {Language, Kind, Content string}
var nativeLabelCorpus=[]labelCase{
 {"en","invoice","INVOICE account billing and amount due reference AA-01"},
 {"de","invoice","RECHNUNG zahlungskonto betrag fall AB-02"},
 {"fr","invoice","FACTURE facture date et montant reference AC-03"},
 {"es","invoice","FACTURA importe pendiente y fecha AD-04"},
 {"en","insurance","POLICY renewal premium coverage reference BA-11"},
 {"de","insurance","VERSICHERUNG policennummer und zahlung BB-12"},
 {"fr","insurance","POLICE assurance couverture renouvellement BC-13"},
 {"es","insurance","SEGURO cobertura prima vencimiento BD-14"},
 {"en","lease","LEASE rental premises terms agreement CA-21"},
 {"de","lease","MIETVERTRAG miete und wohnung CB-22"},
 {"fr","lease","BAIL location et conditions CC-23"},
 {"es","lease","ARRENDAMIENTO alquiler vivienda CD-24"},
 {"en","appointment","APPOINTMENT visit schedule location DA-31"},
 {"de","appointment","TERMIN besuch ort und zeit DB-32"},
 {"fr","appointment","RENDEZVOUS date et lieu DC-33"},
 {"es","appointment","CITA hora lugar DD-34"},
 {"en","statement","STATEMENT period balance account EA-41"},
 {"de","statement","KONTOAUSZUG kontostand zeitraum EB-42"},
 {"fr","statement","RELEVE compte solde EC-43"},
 {"es","statement","EXTRACTO saldo periodo ED-44"},
 {"en","registration","REGISTRATION school enrollment term FA-51"},
 {"de","registration","ANMELDUNG kurs semester FB-52"},
 {"fr","registration","INSCRIPTION cours trimestre FC-53"},
 {"es","registration","MATRICULA escuela semestre FD-54"},
}
var rules=[]struct{Kind,Seed string}{
 {"invoice","INVOICE"},
 {"insurance","POLICY"},
 {"lease","LEASE"},
 {"appointment","APPOINTMENT"},
 {"statement","STATEMENT"},
 {"registration","REGISTRATION"},
}

func TestNativeClassifierBenchmark(t *testing.T){
 endpoint:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_URL")
 token:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_TOKEN")
 if endpoint==""||token==""{t.Skip("disposable synthetic test only")}
 c,err:=NewClient(Config{BaseURL:endpoint,Token:token,TestOnlyLoopbackHTTP:true},nil)
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),17*time.Minute)
 defer cancel()
 ids:=make(map[string]int)
 for _,rule:=range rules {
   request:=map[string]any{"name":"Synthetic "+rule.Kind,"matching_algorithm":3,"match":rule.Seed,"is_insensitive":true}
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
 t.Logf("FOLIO_NATIVE_BASELINE_METRICS=%s",data)
 if len(observations)!=24 {t.Fatalf("missing holdout cases: %d",len(observations))}
}
