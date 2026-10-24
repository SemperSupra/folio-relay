package paperless

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "io"
 "mime"
 "mime/multipart"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

const sampleTaskID="123e4567-e89b-12d3-a456-426614174000"
func fixturePDF() Document {
 b:=[]byte("%PDF-1.7\n% synthetic fixture only\n")
 h:=sha256.Sum256(b)
 return Document{Content:b,SHA256:hex.EncodeToString(h[:]),MediaType:"application/pdf"}
}
func mockClient(t *testing.T,handler http.HandlerFunc)(*Client,*httptest.Server) {
 t.Helper()
 server:=httptest.NewServer(handler)
 t.Cleanup(server.Close)
 c,err:=NewClient(Config{BaseURL:server.URL,Token:"fixture-test-token",TestOnlyLoopbackHTTP:true},nil)
 if err!=nil {t.Fatal(err)}
 return c,server
}
func TestUploadPassesUnmodifiedDocumentAndNoAIOverwrites(t *testing.T){
 d:=fixturePDF()
 hits:=0
 c,_:=mockClient(t,func(w http.ResponseWriter,r *http.Request){
   hits++
   if r.URL.Path!="/api/documents/post_document/" || r.Method!="POST" {t.Errorf("unwanted endpoint %s %s",r.Method,r.URL);w.WriteHeader(400);return}
   if r.Header.Get("Authorization")!="Token fixture-test-token" || r.Header.Get("Accept")!="application/json; version=10" {t.Error("invalid headers")}
   _,params,err:=mime.ParseMediaType(r.Header.Get("Content-Type"))
   if err!=nil {t.Error(err)}
   reader:=multipart.NewReader(r.Body,params["boundary"])
   part,err:=reader.NextPart()
   if err!=nil {t.Error(err);return}
   if part.FormName()!="document" || !strings.HasSuffix(part.FileName(),".pdf") || strings.Contains(part.FileName(),"..") {t.Errorf("wrong part %q",part.FileName())}
   got,_:=io.ReadAll(part)
   if !bytes.Equal(got,d.Content) {t.Error("document mutated")}
   if _,err:=reader.NextPart();err!=io.EOF {t.Errorf("unexpected tags/paths or title injected: %v",err)}
   w.Header().Set("Content-Type","application/json")
   fmt.Fprintf(w,"%q",sampleTaskID)
 })
 uuid,err:=c.Upload(context.Background(),d)
 if err!=nil || uuid!=sampleTaskID || hits!=1 {t.Fatalf("uuid=%q hits=%d err=%v",uuid,hits,err)}
}
func TestObserveAsynchronousStatesAndNativeClassificationReadback(t *testing.T){
 cases:=[]struct{label,status,body,want string}{
  {"pending","started","","pending"},
  {"success","success","\"document_id\":47","archived"},
  {"duplicate","success","\"duplicate_of\":12","duplicate"},
  {"failure","failure","","rejected"},
  {"no-linked-document","success","","unknown"},
  {"unexpected","FUTURE_STATUS","","unknown"},
  {"legacy-v9-uppercase","SUCCESS","\"document_id\":47","unknown"},
 }
 for _,tc:=range cases{
  t.Run(tc.label,func(t *testing.T){
   c,_:=mockClient(t,func(w http.ResponseWriter,r *http.Request){
    if r.Method=="GET" && r.URL.Path=="/api/tasks/" {
      if r.URL.Query().Get("task_id")!=sampleTaskID {t.Error("task query missing")}
      fmt.Fprintf(w,"{\"count\":1,\"results\":[{\"task_id\":%q,\"status\":%q,\"result_data\":{%s}}]}",sampleTaskID,tc.status,tc.body)
      return
    }
    http.NotFound(w,r)
   })
   task,err:=c.Observe(context.Background(),sampleTaskID)
   if err!=nil || task.Outcome!=tc.want {t.Fatalf("got %#v err %v",task,err)}
  })
 }
 c,_:=mockClient(t,func(w http.ResponseWriter,r *http.Request){
  if r.URL.Path=="/api/documents/47/" {
   w.Write([]byte("{\"id\":47,\"title\":\"Energy Invoice\",\"correspondent\":4,\"document_type\":2,\"storage_path\":5,\"tags\":[8,10],\"created\":\"2026-10-24\"}"));return
  }
  http.NotFound(w,r)
 })
 record,err:=c.ReadRecord(context.Background(),47)
 if err!=nil || record.Title!="Energy Invoice" || len(record.Tags)!=2 || *record.DocumentType!=2 {t.Fatalf("native metadata %#v %v",record,err)}
}
func TestTaskNotObservedDoesNotImplySafeRetry(t *testing.T){
 c,_:=mockClient(t,func(w http.ResponseWriter,r *http.Request){w.Write([]byte("{\"count\":0,\"results\":[]}"))})
 task,err:=c.Observe(context.Background(),sampleTaskID)
 if err!=nil||task.Outcome!="unknown" {t.Fatalf("unobserved task %#v %v",task,err)}
}
func TestOriginAndTokenCannotBeRewrittenByRedirect(t *testing.T){
 redirected:=0
 attacker:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
   redirected++
   if r.Header.Get("Authorization")!="" {t.Error("token leaked across redirect")}
 }))
 defer attacker.Close()
 c,_:=mockClient(t,func(w http.ResponseWriter,r *http.Request){
   http.Redirect(w,r,attacker.URL,http.StatusTemporaryRedirect)
 })
 _,err:=c.Upload(context.Background(),fixturePDF())
 if err==nil || redirected!=0 {t.Fatalf("followed redirect, err=%v hits=%d",err,redirected)}
}
func TestMalformedAndUnexpectedTaskResponsesFailClosed(t *testing.T){
 cases:=[]struct{json string}{
  {"{\"count\":2,\"results\":[]}"},
  {"{\"count\":1,\"results\":[{\"task_id\":\"00000000-0000-0000-0000-000000000000\",\"status\":\"SUCCESS\",\"result_data\":{\"document_id\":4}}]}"},
  {"{\"count\":1,\"results\":[{\"task_id\":\""+sampleTaskID+"\",\"status\":\"SUCCESS\",\"result_data\":{\"document_id\":4,\"duplicate_of\":5}}]}"},
 }
 for _,tc:=range cases {
  c,_:=mockClient(t,func(w http.ResponseWriter,r *http.Request){w.Write([]byte(tc.json))})
  task,err:=c.Observe(context.Background(),sampleTaskID)
  if err==nil && task.Outcome!="unknown" {t.Fatalf("unsafe task %#v",task)}
 }
 c,_:=mockClient(t,func(w http.ResponseWriter,r *http.Request){w.Write([]byte("{\"unexpected\":\"shape\"}"))})
 if _,err:=c.Upload(context.Background(),fixturePDF());!errors.Is(err,ErrUncertain) {t.Fatalf("malformed upload acceptance must hold: %v",err)}
}
func TestRejectUntrustedConfigAndBytes(t *testing.T){
 cases:=[]Config{
  {BaseURL:"http://example.com",Token:"verysecret",TestOnlyLoopbackHTTP:true},
  {BaseURL:"http://localhost",Token:"verysecret"},
  {BaseURL:"https://operator:secret@example.com",Token:"verysecret"},
  {BaseURL:"https://example.com/malicious",Token:"verysecret"},
  {BaseURL:"https://example.com?evil=yes",Token:"verysecret"},
  {BaseURL:"ftp://example.com",Token:"verysecret"},
  {BaseURL:"https://example.com",Token:"a\nToken: leaked"},
 }
 for _,cfg:=range cases {if _,err:=NewClient(cfg,nil);err==nil {t.Errorf("accepted config %q",cfg.BaseURL)}}
 c,_:=mockClient(t,func(w http.ResponseWriter,r *http.Request){t.Fatal("invalid data reached network")})
 bad:=fixturePDF();bad.SHA256=strings.Repeat("0",64)
 if _,err:=c.Upload(context.Background(),bad);!errors.Is(err,ErrInvalid) {t.Fatal(err)}
 bad=fixturePDF();bad.MediaType="text/html"
 if _,err:=c.Upload(context.Background(),bad);!errors.Is(err,ErrInvalid) {t.Fatal(err)}
 if _,err:=c.Observe(context.Background(),"../../../evil");!errors.Is(err,ErrInvalid) {t.Fatal(err)}
 if _,err:=c.ReadRecord(context.Background(),-1);!errors.Is(err,ErrInvalid) {t.Fatal(err)}
}
func TestNoTokenInErrors(t *testing.T){
 c,_:=mockClient(t,func(w http.ResponseWriter,r *http.Request){w.WriteHeader(403);w.Write([]byte("secret fixture-test-token"))})
 _,err:=c.Upload(context.Background(),fixturePDF())
 if !errors.Is(err,ErrRemote)||strings.Contains(err.Error(),"fixture-test-token"){t.Fatalf("exposed secret %v",err)}
}
