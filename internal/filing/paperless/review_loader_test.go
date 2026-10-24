package paperless

import (
 "context"
 "crypto/sha256"
 "crypto/tls"
 "encoding/json"
 "errors"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func makeArchivedSyntheticReview(t *testing.T)(*Outbox,Ticket){
 t.Helper()
 outbox,err:=OpenOutbox(filepath.Join(t.TempDir(),"review.frj"))
 if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){outbox.Close()})
 intent:=testIntent()
 ticket,err:=outbox.Enqueue(intent)
 if err!=nil{t.Fatal(err)}
 if _,err:=outbox.Begin(ticket.ID);err!=nil{t.Fatal(err)}
 if _,err:=outbox.Accept(ticket.ID,sampleTaskID);err!=nil{t.Fatal(err)}
 archived,err:=outbox.MarkTask(ticket.ID,Task{TaskID:sampleTaskID,Outcome:"archived",DocumentID:48})
 if err!=nil||archived.State!="archived"{t.Fatalf("cannot archive test receipt: %#v %v",archived,err)}
 return outbox,archived
}
func TestArchivedLoaderUsesOnlyAuthorizedNativeGETs(t *testing.T){
 outbox,ticket:=makeArchivedSyntheticReview(t)
 calls:=0
 upstream:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  calls++
  if r.Method!=http.MethodGet {
    t.Errorf("loader unexpectedly made DMS mutation: %s",r.Method)
    w.WriteHeader(405);return
  }
  if r.Header.Get("Authorization")!="Token disposable-native-token"{
    t.Errorf("DMS token incorrect or missing");w.WriteHeader(401);return
  }
  w.Header().Set("Content-Type","application/json")
  switch r.URL.Path{
  case "/api/documents/48/":
   json.NewEncoder(w).Encode(map[string]any{
     "id":48,"title":"ใบแจ้งหนี้ Not an invoice <script>alert(1)</script>",
     "document_type":nil,"correspondent":nil,"storage_path":nil,
     "tags":[]int{},
   })
  case "/api/documents/48/suggestions/":
   json.NewEncoder(w).Encode(map[string]any{
     "document_types":[]int{7},"correspondents":[]int{},
     "storage_paths":[]int{},"tags":[]int{5},
   })
  default:w.WriteHeader(404)
  }
 }))
 defer upstream.Close()
 client,err:=NewClient(Config{BaseURL:upstream.URL,Token:"disposable-native-token",TestOnlyLoopbackHTTP:true},nil)
 if err!=nil{t.Fatal(err)}
 languageCalls:=0
 loader,err:=NewArchivedReviewLoader(outbox,client,"paperless-main",func(context.Context,string)(ReviewLanguage,error){
   languageCalls++
   return ReviewThai,nil
 })
 if err!=nil{t.Fatal(err)}
 _,hash:=capabilityFixture()
 wireToken,_:=capabilityFixture()
 at:=time.Date(2026,10,24,8,30,0,0,time.UTC)
 grant:=ReviewGrant{TokenHash:hash,ActorID:"thai-operator",
  EffectID:ticket.ID,ValidFrom:at.Add(-time.Hour),ExpiresAt:at.Add(time.Hour),CanRead:true}
 auth,err:=NewScopedReviewAuthorizer(func(received [32]byte)(ReviewGrant,error){
    if received!=hash{return ReviewGrant{},ErrReviewGrant}
    return grant,nil
 },func()time.Time{return at})
 if err!=nil{t.Fatal(err)}
 handler,err:=NewReviewHandler(auth,loader)
 if err!=nil{t.Fatal(err)}
 call:=func(path,token string)*httptest.ResponseRecorder{
  request:=httptest.NewRequest(http.MethodGet,path,nil)
  request.TLS=&tls.ConnectionState{}
  if token!=""{request.Header.Set("Authorization","Bearer "+token)}
  rec:=httptest.NewRecorder()
  handler.ServeHTTP(rec,request)
  return rec
 }
 if r:=call("/api/filing/reviews/"+ticket.ID,"");r.Code!=401||calls!=0||languageCalls!=0{
  t.Fatalf("unauthorized requester performed native read status=%d reads=%d",r.Code,calls)
 }
 if r:=call("/api/filing/reviews/"+strings.Repeat("f",64),wireToken);r.Code!=401||calls!=0||languageCalls!=0 {
  t.Fatalf("cross-record requester performed native read status=%d reads=%d",r.Code,calls)
 }
 success:=call("/api/filing/reviews/"+ticket.ID,wireToken)
 if success.Code!=200||calls!=2||languageCalls!=1 {
  t.Fatalf("review failed or unexpected DMS calls: %d calls=%d lang=%d",success.Code,calls,languageCalls)
 }
 var envelope ReviewEnvelope
 if err:=json.Unmarshal(success.Body.Bytes(),&envelope);err!=nil{t.Fatal(err)}
 if envelope.EffectID!=ticket.ID||envelope.PaperlessDocumentID!=48 ||
    envelope.Language!=ReviewThai||envelope.AutoApplyAllowed||!envelope.RequiresHumanReview {
    t.Fatalf("read-only review integrity failure: %#v",envelope)
 }
 if !strings.Contains(envelope.NativeTitle,"ใบแจ้งหนี้"){t.Fatal("Thai title lost")}
 grant.Revoked=true
 if r:=call("/api/filing/reviews/"+ticket.ID,wireToken);r.Code!=401||calls!=2 {
   t.Fatal("revoked capability still exposed Paperless metadata")
 }
}
func TestArchivedLoaderFailsClosedOnWrongTargetAndUncommittedEffect(t *testing.T){
 outbox,ticket:=makeArchivedSyntheticReview(t)
 requests:=0
 upstream:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
    requests++;w.WriteHeader(http.StatusInternalServerError)
 }))
 defer upstream.Close()
 c,err:=NewClient(Config{BaseURL:upstream.URL,Token:"test-token",TestOnlyLoopbackHTTP:true},nil)
 if err!=nil{t.Fatal(err)}
 lang:=func(context.Context,string)(ReviewLanguage,error){return ReviewUnknown,nil}
 for _,arg:=range []struct{outbox *Outbox;client *Client;target string;language ReviewLanguageLookup}{
  {nil,c,"paperless-main",lang},
  {outbox,nil,"paperless-main",lang},
  {outbox,c,"bad target!",lang},
  {outbox,c,"paperless-main",nil},
 }{
  if _,err:=NewArchivedReviewLoader(arg.outbox,arg.client,arg.target,arg.language);!errors.Is(err,ErrReviewUnavailable){
   t.Fatalf("incomplete loader dependency accepted: %v",err)
  }
 }
 wrong,err:=NewArchivedReviewLoader(outbox,c,"paperless-another",lang)
 if err!=nil{t.Fatal(err)}
 if _,err:=wrong(context.Background(),ticket.ID);!errors.Is(err,ErrReviewUnavailable){
   t.Fatal("another DMS target read user metadata")
 }
 if requests!=0{t.Fatal("target mismatch produced remote request")}
 right,err:=NewArchivedReviewLoader(outbox,c,"paperless-main",lang)
 if err!=nil{t.Fatal(err)}
 if _,err:=right(context.Background(),strings.Repeat("0",64));!errors.Is(err,ErrReviewUnavailable){
   t.Fatal("nonexistent effect obtained a DMS read")
 }
 if requests!=0{t.Fatal("unknown effect produced remote request")}
 if _,err:=right(context.Background(),ticket.ID);!errors.Is(err,ErrReviewUnavailable){
    t.Fatal("native DMS request failure incorrectly treated as successful review")
 }
 if requests!=1{t.Fatalf("missing exactly one failed native GET: %d",requests)}
}
func TestReviewLoaderDoesNotTrustForgedReceiptOrLanguage(t *testing.T){
 outbox,ticket:=makeArchivedSyntheticReview(t)
 requests:=0
 remote:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  requests++
  w.Header().Set("Content-Type","application/json")
  // An untrusted/misconfigured Paperless instance returns a DIFFERENT record.
  w.Write([]byte(`{"id":99,"title":"Another person's record"}`))
 }))
 defer remote.Close()
 c,err:=NewClient(Config{BaseURL:remote.URL,Token:"fixture-token",TestOnlyLoopbackHTTP:true},nil)
 if err!=nil{t.Fatal(err)}
 reader,err:=NewArchivedReviewLoader(outbox,c,"paperless-main",func(context.Context,string)(ReviewLanguage,error){
   return ReviewGerman,nil
 })
 if err!=nil{t.Fatal(err)}
 if _,err:=reader(context.Background(),ticket.ID);!errors.Is(err,ErrReviewUnavailable){
   t.Fatal("misbound DMS document ID became a review packet")
 }
 if requests!=1{t.Fatal("unexpected additional DMS reads after mismatch")}
 // Ensure annotations are mandatory trusted source inputs.
 annotationFails:=func(context.Context,string)(ReviewLanguage,error){
  return ReviewUnknown,errors.New("no reviewed language data")
 }
 reader,err=NewArchivedReviewLoader(outbox,c,"paperless-main",annotationFails)
 if err!=nil{t.Fatal(err)}
 if _,err:=reader(context.Background(),ticket.ID);!errors.Is(err,ErrReviewUnavailable)||requests!=1{
   t.Fatal("missing trusted language annotation caused DMS disclosure")
 }
 // A forged, unarchived effect can never cause a remote GET.
 extra,err:=outbox.Enqueue(Intent{CanonicalSHA256:strings.Repeat("b",64),
  TargetID:"paperless-main",PolicyVersion:"v1",ApprovalID:"approved-002"})
 if err!=nil{t.Fatal(err)}
 if _,err:=reader(context.Background(),extra.ID);!errors.Is(err,ErrReviewUnavailable)||requests!=1{
   t.Fatal("unarchived filing effect caused remote request")
 }
}
func TestScopedTokenHashUsesFixedLengthDigest(t *testing.T){
 raw,_:=capabilityFixture()
 secret,err:=base64.RawURLEncoding.DecodeString(raw)
 if err!=nil{t.Fatal(err)}
 actual:=sha256.Sum256(secret)
 _,hash:=capabilityFixture()
 if actual!=hash{t.Fatal("capability wire and grant hash disagree")}
}
