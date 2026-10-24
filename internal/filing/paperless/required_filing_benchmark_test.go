//go:build paperless_integration

package paperless

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "sort"
 "testing"
 "time"
)

// A deliberately small *source-only* positive + negative EN/DE/TH benchmark.
// We measure native Paperless rule assignment and OCR presence; these rules
// cannot be authorized to apply a filing path or retention policy by this test.
func TestRequiredTrilingualFilingQuality(t *testing.T) {
 base:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_URL")
 token:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_TOKEN")
 dir:=os.Getenv("FOLIO_TRILINGUAL_FIXTURE_DIR")
 if base==""||token==""||dir==""{t.Skip("explicit disposable EN DE TH run only")}
 c,err:=NewClient(Config{BaseURL:base,Token:token,TestOnlyLoopbackHTTP:true},nil)
 if err!=nil{t.Fatal(err)}
 data,err:=os.ReadFile(filepath.Join(dir,"classification-manifest.json"))
 if err!=nil{t.Fatal(err)}
 type sample struct{
  ID string `json:"id"`
  Language string `json:"language"`
  Kind string `json:"kind"`
  Negative bool `json:"negative"`
  File string `json:"file"`
 }
 var samples []sample
 if err:=json.Unmarshal(data,&samples);err!=nil{t.Fatal(err)}
 if len(samples)!=24{t.Fatalf("expected 18 positive plus six negatives: %d",len(samples))}
 ctx,cancel:=context.WithTimeout(context.Background(),21*time.Minute)
 defer cancel()
 patterns:=[]struct{kind,pattern string}{
  {"invoice","INVOICE|RECHNUNG|ใบแจ้งหนี้"},
  {"insurance","INSURANCE POLICY|VERSICHERUNG|กรมธรรม์ประกันภัย"},
  {"lease","LEASE AGREEMENT|MIETVERTRAG|สัญญาเช่า"},
  {"appointment","APPOINTMENT|TERMINBESTÄTIGUNG|ใบนัดหมาย"},
  {"statement","BANK STATEMENT|KONTOAUSZUG|รายการเดินบัญชี"},
  {"registration","REGISTRATION|ANMELDUNG|ใบลงทะเบียน"},
 }
 ids:=map[string]int{}
 for _,rule:=range patterns {
  body,err:=json.Marshal(map[string]any{
    "name":"synthetic tri-language "+rule.kind,
    "matching_algorithm":4,
    "match":rule.pattern,
    "is_insensitive":true,
  })
  if err!=nil{t.Fatal(err)}
  response,err:=c.do(ctx,"POST","/api/document_types/","application/json",bytes.NewReader(body))
  if err!=nil{t.Fatalf("fixture taxonomy create %s: %v",rule.kind,err)}
  var obj struct{ID int `json:"id"`}
  err=json.NewDecoder(response.Body).Decode(&obj)
  response.Body.Close()
  if err!=nil||obj.ID<=0{t.Fatalf("fixture taxonomy %s: %v",rule.kind,err)}
  ids[rule.kind]=obj.ID
 }
 type results struct {
  Language string `json:"language"`
  Positive int `json:"positive"`
  Correct int `json:"correct"`
  Wrong int `json:"wrong"`
  Abstained int `json:"abstained"`
  Negatives int `json:"negatives"`
  UnsafeNegatives int `json:"unsafe_negatives"`
  ThaiOCRMissing int `json:"thai_ocr_missing"`
  DurationsMS []int64 `json:"durations_ms"`
 }
 byLang:=map[string]*results{
   "en":{Language:"en"},"de":{Language:"de"},"th":{Language:"th"},
 }
 seen:=map[string]bool{}
 for _,item:=range samples {
  stats:=byLang[item.Language]
  if stats==nil||ids[item.Kind]==0||seen[item.ID]{t.Fatalf("invalid sample metadata %#v",item)}
  seen[item.ID]=true
  png,err:=os.ReadFile(filepath.Join(dir,item.File))
  if err!=nil{t.Fatal(err)}
  sha:=sha256.Sum256(png)
  start:=time.Now()
  taskID,err:=c.Upload(ctx,Document{
   Content:png,SHA256:hex.EncodeToString(sha[:]),MediaType:"image/png",
  })
  if err!=nil{t.Fatalf("upload %s: %v",item.ID,err)}
  var task Task
  for {
   if ctx.Err()!=nil{t.Fatalf("EN/DE/TH deadline sample %s state=%q outcome=%q",item.ID,task.Status,task.Outcome)}
   task,err=c.Observe(ctx,taskID)
   if err!=nil{t.Fatalf("task %s: %v",item.ID,err)}
   if task.Outcome=="archived"||task.Outcome=="duplicate"||task.Outcome=="rejected"{break}
   time.Sleep(time.Second)
  }
  if task.Outcome!="archived"{t.Fatalf("OCR/ingest %s: %q",item.ID,task.Outcome)}
  response,err:=c.do(ctx,"GET",fmt.Sprintf("/api/documents/%d/",task.DocumentID),"",nil)
  if err!=nil{t.Fatal(err)}
  var record struct {
    ID int `json:"id"`
    DocumentType *int `json:"document_type"`
    Content string `json:"content"`
  }
  decErr:=json.NewDecoder(response.Body).Decode(&record)
  response.Body.Close()
  if decErr!=nil||record.ID!=task.DocumentID{t.Fatalf("readback %s: %v",item.ID,decErr)}
  stats.DurationsMS=append(stats.DurationsMS,time.Since(start).Milliseconds())
  var actual int
  if record.DocumentType!=nil{actual=*record.DocumentType}
  if item.Language=="th"&&countThai(record.Content)<4 {stats.ThaiOCRMissing++}
  if item.Negative {
   stats.Negatives++
   if actual!=0{stats.UnsafeNegatives++}
   continue
  }
  stats.Positive++
  switch{
  case actual==0:stats.Abstained++
  case actual==ids[item.Kind]:stats.Correct++
  default:stats.Wrong++
  }
 }
 keys:=[]string{"de","en","th"}
 sort.Strings(keys)
 resultsOut:=make([]results,0,len(keys))
 for _,lang:=range keys{
  s:=byLang[lang]
  if s.Positive!=6||s.Negatives!=2{t.Fatalf("missing required-language coverage %+v",s)}
  resultsOut=append(resultsOut,*s)
 }
 encoded,err:=json.Marshal(struct{
   Corpus string `json:"corpus"`
   PerLanguage []results `json:"per_language"`
   AutoApply bool `json:"auto_apply"`
 }{"18-positive-6-negative-raster-en-de-th",resultsOut,false})
 if err!=nil{t.Fatal(err)}
 t.Logf("FOLIO_REQUIRED_LANGUAGE_FILING_METRICS=%s",encoded)
 // The experiment deliberately does not assert a classification accuracy
 // threshold. Actual false routes, OCR misses and abstentions are reported
 // faithfully; independent review decides whether the baseline earns its keep.
}
