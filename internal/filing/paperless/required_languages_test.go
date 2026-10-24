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
 "sort"
 "strings"
 "testing"
 "time"
)

// This qualifies OCR of actual pixels, not embedded/searchable PDF text.
// This test must NOT be used to claim that handwriting, all Thai diacritics,
// Thai word segmentation, or production scanning conditions are qualified.
func TestRequiredEnglishGermanThaiRasterOCR(t *testing.T) {
 root:=os.Getenv("FOLIO_TRILINGUAL_FIXTURE_DIR")
 base:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_URL")
 token:=os.Getenv("FOLIO_FIXTURE_PAPERLESS_TOKEN")
 if root==""||base==""||token=="" {t.Skip("explicit disposable trilingual fixture only")}
 type sample struct {
  ID string `json:"id"`
  Language string `json:"language"`
  File string `json:"file"`
  Lines []string `json:"lines"`
 }
 raw,err:=os.ReadFile(filepath.Join(root,"manifest.json"))
 if err!=nil{t.Fatal(err)}
 var manifest []sample
 if err:=json.Unmarshal(raw,&manifest);err!=nil{t.Fatal(err)}
 if len(manifest)!=4 {t.Fatalf("expected English/German/Thai/mixed raster cases: %d",len(manifest))}
 client,err:=NewClient(Config{BaseURL:base,Token:token,TestOnlyLoopbackHTTP:true},nil)
 if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),14*time.Minute)
 defer cancel()
 type measure struct {
  ID string `json:"id"`
  Language string `json:"language"`
  OCRChars int `json:"ocr_chars"`
  ThaiChars int `json:"thai_chars"`
  ThaiCharOverlap float64 `json:"thai_char_overlap"`
  EnglishAnchor bool `json:"english_anchor"`
  GermanAnchor bool `json:"german_anchor"`
  LatencyMS int64 `json:"latency_ms"`
 }
 var measurements []measure
 seen:=map[string]bool{}
 for _,item:=range manifest {
  if seen[item.ID]{t.Fatalf("duplicate fixture %q",item.ID)}
  seen[item.ID]=true
  b,err:=os.ReadFile(filepath.Join(root,item.File))
  if err!=nil{t.Fatal(err)}
  hash:=sha256.Sum256(b)
  t0:=time.Now()
  taskID,err:=client.Upload(ctx,Document{Content:b,SHA256:hex.EncodeToString(hash[:]),MediaType:"image/png"})
  if err!=nil{t.Fatalf("upload %s: %v",item.ID,err)}
  var task Task
  for {
   if err:=ctx.Err();err!=nil {t.Fatalf("OCR deadline %s: last status=%s outcome=%s",item.ID,task.Status,task.Outcome)}
   task,err=client.Observe(ctx,taskID)
   if err!=nil{t.Fatalf("observe %s: %v",item.ID,err)}
   if task.Outcome=="archived"||task.Outcome=="duplicate"||task.Outcome=="rejected" {break}
   time.Sleep(2*time.Second)
  }
  if task.Outcome!="archived"||task.DocumentID<=0{t.Fatalf("OCR archive %s: %s status=%s",item.ID,task.Outcome,task.Status)}
  response,err:=client.do(ctx,"GET",fmt.Sprintf("/api/documents/%d/",task.DocumentID),"",nil)
  if err!=nil{t.Fatalf("document OCR text readback %s: %v",item.ID,err)}
  var record struct {
    ID int `json:"id"`
    Content string `json:"content"`
  }
  decErr:=json.NewDecoder(response.Body).Decode(&record)
  response.Body.Close()
  if decErr!=nil||record.ID!=task.DocumentID{t.Fatalf("bad OCR readback %s: %v",item.ID,decErr)}
  reference:=strings.Join(item.Lines," ")
  actual:=record.Content
  m:=measure{
    ID:item.ID,Language:item.Language,OCRChars:len([]rune(actual)),
    ThaiChars:countThai(actual),
    ThaiCharOverlap:thaiOverlap(reference,actual),
    EnglishAnchor:strings.Contains(strings.ToUpper(actual),"INVOICE"),
    GermanAnchor:strings.Contains(strings.ToUpper(actual),"RECHNUNG"),
    LatencyMS:time.Since(t0).Milliseconds(),
  }
  measurements=append(measurements,m)
  // Hard smoke gates distinguish missing language packs or skipped raster
  // OCR from imperfect but actual language decoding.
  switch item.ID {
  case "english":
    if !m.EnglishAnchor{t.Errorf("English OCR failed to extract anchor")}
  case "german":
    if !m.GermanAnchor{t.Errorf("German OCR failed to extract anchor")}
  case "thai":
    if m.ThaiChars<6{t.Errorf("Thai raster OCR failed: only %d Thai codepoints",m.ThaiChars)}
  case "mixed":
    if !m.EnglishAnchor||!m.GermanAnchor||m.ThaiChars<4{
      t.Errorf("mixed-language raster OCR insufficient (en=%v de=%v th=%d)",m.EnglishAnchor,m.GermanAnchor,m.ThaiChars)
    }
  default: t.Errorf("unsupported fixture %s",item.ID)
  }
 }
 if len(seen)!=4 {t.Fatal("incomplete test")}
 sort.Slice(measurements,func(i,j int)bool{return measurements[i].ID<measurements[j].ID})
 report,err:=json.Marshal(struct {
  Profile string `json:"profile"`
  Cases []measure `json:"cases"`
 }{"eng+deu+tha",measurements})
 if err!=nil{t.Fatal(err)}
 t.Logf("FOLIO_REQUIRED_LANGUAGE_OCR_METRICS=%s",report)
}

// Multiset character-overlap is a *descriptive* script-recognition metric.
// It cannot establish correct Thai word order or semantic interpretation.
func thaiOverlap(reference,observed string)float64{
 ref:=map[rune]int{}
 total:=0
 for _,r:=range reference{if r>=0x0e00&&r<=0x0e7f {ref[r]++;total++}}
 if total==0{return 0}
 overlap:=0
 for _,r:=range observed{
  if ref[r]>0{ref[r]--;overlap++}
 }
 return float64(overlap)/float64(total)
}
func countThai(s string)int{
 count:=0
 for _,r:=range s{if r>=0x0e00&&r<=0x0e7f{count++}}
 return count
}
