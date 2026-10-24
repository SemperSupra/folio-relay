//go:build paperless_integration

package paperless

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

// Advisory-only semantic classification in the REQUIRED languages, using
// actual pixels scanned through Paperless's own OCR and native LLM endpoint.
// The tiny CPU model is a feasibility baseline, not a product-quality model.
// No model-written title, type, tag, path or retention is applied.
func TestRequiredEnglishGermanThaiOllama(t *testing.T){
 dir:=os.Getenv("FOLIO_TRILINGUAL_FIXTURE_DIR")
 base:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_URL")
 token:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_TOKEN")
 if dir==""||base==""||token==""{t.Skip("only approved disposable EN DE TH inference fixture")}
 c,err:=NewClient(Config{BaseURL:base,Token:token,TestOnlyLoopbackHTTP:true},nil)
 if err!=nil{t.Fatal(err)}
 payload,err:=os.ReadFile(filepath.Join(dir,"classification-manifest.json"))
 if err!=nil{t.Fatal(err)}
 var cases []struct{
  ID string `json:"id"`
  Language string `json:"language"`
  Kind string `json:"kind"`
  Negative bool `json:"negative"`
  File string `json:"file"`
 }
 if err:=json.Unmarshal(payload,&cases);err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),28*time.Minute)
 defer cancel()
 aliases:=map[string][]string{
  "invoice":{"invoice","rechnung","ใบแจ้งหนี้"},
  "appointment":{"appointment","termin","นัดหมาย","ใบนัดหมาย"},
 }
 type langReport struct{
  Language string `json:"language"`
  Cases int `json:"cases"`
  Nonblank int `json:"nonblank"`
  TypeMatched int `json:"type_matched"`
  TypeMissingOrOther int `json:"type_missing_or_other"`
  AdversarialCases int `json:"adversarial_cases"`
  AdversarialSuggestedType int `json:"adversarial_suggested_type"`
  LatencyMS []int64 `json:"latency_ms"`
 }
 reports:=map[string]*langReport{
  "en":{Language:"en"},"de":{Language:"de"},"th":{Language:"th"},
 }
 errorsCount:=0
 for _,item:=range cases {
  // Balance two categories × three supported languages, plus one negative
  // per language. Discard all other generated corpus cases in this lane.
  if item.Kind!="invoice"&&item.Kind!="appointment"{continue}
  if item.Negative && item.Kind!="invoice"{continue}
  stats:=reports[item.Language]
  if stats==nil{t.Fatalf("unexpected language: %s",item.Language)}
  raster,err:=os.ReadFile(filepath.Join(dir,item.File))
  if err!=nil{t.Fatal(err)}
  sha:=sha256.Sum256(raster)
  tid,err:=c.Upload(ctx,Document{Content:raster,SHA256:hex.EncodeToString(sha[:]),MediaType:"image/png"})
  if err!=nil{t.Fatalf("upload %s: %v",item.ID,err)}
  var task Task
  for {
   if ctx.Err()!=nil{t.Fatalf("inference input deadline %s task=%s/%s",item.ID,task.Status,task.Outcome)}
   task,err=c.Observe(ctx,tid)
   if err!=nil{t.Fatal(err)}
   if task.Outcome=="archived"||task.Outcome=="duplicate"||task.Outcome=="rejected"{break}
   time.Sleep(time.Second)
  }
  if task.Outcome!="archived"{t.Fatalf("input %s: %s",item.ID,task.Outcome)}
  t0:=time.Now()
  resp,err:=c.do(ctx,"GET",fmt.Sprintf("/api/documents/%d/ai_suggestions/",task.DocumentID),"",nil)
  if err!=nil{t.Logf("native Ollama API error for synthetic %s: %v",item.ID,err);errorsCount++;continue}
  var suggestions AISuggestions
  parseErr:=json.NewDecoder(resp.Body).Decode(&suggestions)
  resp.Body.Close()
  if parseErr!=nil{t.Logf("native model structured response failed for %s",item.ID);errorsCount++;continue}
  stats.Cases++
  stats.LatencyMS=append(stats.LatencyMS,time.Since(t0).Milliseconds())
  names:=suggestions.SuggestedDocumentTypes
  nonblank:=len(names)>0||len(suggestions.DocumentTypes)>0||(suggestions.Title!=nil&&*suggestions.Title!="")
  if nonblank {stats.Nonblank++}
  if item.Negative{
   stats.AdversarialCases++
   // An unsafe recommendation is any explicit type proposal which includes
   // the negated category. This is not a claim about other possible errors.
   for _,name:=range names {
    n:=strings.ToLower(name)
    for _,v:=range aliases[item.Kind] {
     if strings.Contains(n,strings.ToLower(v)){stats.AdversarialSuggestedType++;break}
    }
   }
   continue
  }
  match:=false
  for _,name:=range names{
   n:=strings.ToLower(name)
   for _,v:=range aliases[item.Kind]{
    if strings.Contains(n,strings.ToLower(v)){match=true;break}
   }
   if match{break}
  }
  if match{stats.TypeMatched++}else{stats.TypeMissingOrOther++}
 }
 ordered:=[]langReport{*reports["en"],*reports["de"],*reports["th"]}
 for _,s:=range ordered {
  if s.Cases!=3||s.AdversarialCases!=1 {
    t.Fatalf("incomplete native Ollama EN/DE/TH fixture: %+v",s)
  }
 }
 b,err:=json.Marshal(struct{
   Model string `json:"model"`
   OCRProfile string `json:"ocr_profile"`
   InferenceFailures int `json:"inference_failures"`
   PerLanguage []langReport `json:"per_language"`
   AutoApply bool `json:"auto_apply"`
 }{"qwen2.5:0.5b-instruct","eng+deu+tha",errorsCount,ordered,false})
 if err!=nil{t.Fatal(err)}
 t.Logf("FOLIO_REQUIRED_LANGUAGE_OLLAMA_METRICS=%s",b)
 if errorsCount>0{t.Fatalf("native local LLM failed %d times",errorsCount)}
}
