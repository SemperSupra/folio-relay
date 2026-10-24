package paperless

import (
 "context"
 "encoding/json"
 "errors"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func reviewerTransportFixture(t *testing.T)(http.Handler,*int,*int,string) {
 t.Helper()
 ticket,record,suggestions:=acceptedReviewFixture()
 // Untrusted document and model text should never become executable HTML.
 record.Title="ใบแจ้งหนี้ Rechnung <script>alert('pwn')</script> & Invoice"
 packet,err:=BuildReviewEnvelope(ticket,record,suggestions,ReviewThai)
 if err!=nil{t.Fatal(err)}
 authCalls:=0
 reads:=0
 handler,err:=NewReviewHandler(
   func(r *http.Request,id string)(ReviewPrincipal,error){
      authCalls++
      if r.Header.Get("Authorization")!="Bearer disposable-local-fixture" {
        return ReviewPrincipal{},errors.New("not authenticated")
      }
      if id!=packet.EffectID{return ReviewPrincipal{ActorID:"tester"},nil}
      if r.Header.Get("X-Test-Scope")!="document-read" {
        return ReviewPrincipal{ActorID:"tester",CanRead:false},nil
      }
      return ReviewPrincipal{ActorID:"tester",CanRead:true},nil
   },
   func(_ context.Context,id string)(ReviewEnvelope,error){
      reads++
      if id!=packet.EffectID{return ReviewEnvelope{},errors.New("unknown")}
      return packet,nil
   },
 )
 if err!=nil{t.Fatal(err)}
 return handler,&authCalls,&reads,packet.EffectID
}
func getFixture(h http.Handler,path,token,scope string)*httptest.ResponseRecorder{
 req:=httptest.NewRequest(http.MethodGet,path,nil)
 if token!=""{req.Header.Set("Authorization",token)}
 if scope!=""{req.Header.Set("X-Test-Scope",scope)}
 rec:=httptest.NewRecorder()
 h.ServeHTTP(rec,req)
 return rec
}
func TestReviewTransportDefaultDenyAndEffectScopedAuthorization(t *testing.T){
 if _,err:=NewReviewHandler(nil,nil);!errors.Is(err,ErrReviewEndpoint){t.Fatal(err)}
 h,a,l,id:=reviewerTransportFixture(t)
 path:="/api/filing/reviews/"+id
 missing:=getFixture(h,path,"","")
 if missing.Code!=http.StatusUnauthorized||*l!=0||*a!=1{t.Fatalf("missing auth granted: %d read %d",missing.Code,*l)}
 noScope:=getFixture(h,path,"Bearer disposable-local-fixture","")
 if noScope.Code!=http.StatusForbidden||*l!=0||*a!=2 {t.Fatalf("no record scope granted: %d",noScope.Code)}
 forbidden:=getFixture(h,"/api/filing/reviews/"+strings.Repeat("a",64),"Bearer disposable-local-fixture","document-read")
 if forbidden.Code!=http.StatusForbidden||*l!=0 {t.Fatalf("unscoped record granted: %d",forbidden.Code)}
 malformed:=getFixture(h,"/api/filing/reviews/../../env","Bearer disposable-local-fixture","document-read")
 if malformed.Code!=http.StatusNotFound||*l!=0{t.Fatalf("invalid ID granted: %d",malformed.Code)}
 badMethod:=httptest.NewRequest(http.MethodPost,path,strings.NewReader("{\"approved\":true}"))
 badMethod.Header.Set("Authorization","Bearer disposable-local-fixture")
 rec:=httptest.NewRecorder()
 h.ServeHTTP(rec,badMethod)
 if rec.Code!=http.StatusMethodNotAllowed||*l!=0 {t.Fatalf("POST side effect: %d",rec.Code)}
 if rec.Header().Get("Allow")!="GET"{t.Fatal("missing method contract")}
}
func TestReviewTransportJSONReadableButInert(t *testing.T){
 h,_,_,id:=reviewerTransportFixture(t)
 rec:=getFixture(h,"/api/filing/reviews/"+id,"Bearer disposable-local-fixture","document-read")
 if rec.Code!=http.StatusOK{t.Fatalf("GET status=%d body=%q",rec.Code,rec.Body.String())}
 if got:=rec.Header().Get("Content-Type");!strings.HasPrefix(got,"application/json") {t.Fatalf("bad JSON type %s",got)}
 if !strings.Contains(rec.Header().Get("Cache-Control"),"no-store"){t.Fatal("document review cached")}
 if rec.Header().Get("Content-Security-Policy")==""||rec.Header().Get("X-Frame-Options")!="DENY" {t.Fatal("UI policy headers missing")}
 var packet ReviewEnvelope
 if err:=json.Unmarshal(rec.Body.Bytes(),&packet);err!=nil{t.Fatal(err)}
 if !strings.Contains(packet.NativeTitle,"ใบแจ้งหนี้")||!strings.Contains(packet.NativeTitle,"<script>") {t.Fatal("lost original untrusted text")}
 if packet.AutoApplyAllowed||packet.ClassificationQualified||!packet.RequiresHumanReview {
   t.Fatal("JSON claimed native recommendation was authorized")
 }
 if packet.EffectID!=id||packet.Language!=ReviewThai{t.Fatal("wrong record")}
 if strings.Contains(rec.Body.String(),"approved-001"){t.Fatal("disclosed approval identity")}
}
func TestReviewerHTMLAccessibleLocalizationsAndEscapes(t *testing.T){
 ticket,record,hints:=acceptedReviewFixture()
 record.Title="ใบแจ้งหนี้ <script>alert('pwn')</script> & Rechnung"
 for _,lang:=range []struct{
  choice ReviewLanguage
  htmlLang string
  title string
 }{
  {ReviewEnglish,"en","Document filing review"},
  {ReviewGerman,"de","Dokumentenablage prüfen"},
  {ReviewThai,"th","ตรวจสอบการจัดเก็บเอกสาร"},
  {ReviewMixed,"en","Document filing review"},
  {ReviewUnknown,"en","Document filing review"},
 }{
  packet,err:=BuildReviewEnvelope(ticket,record,hints,lang.choice)
  if err!=nil{t.Fatal(err)}
  handler,err:=NewReviewHandler(func(*http.Request,string)(ReviewPrincipal,error){
   return ReviewPrincipal{ActorID:"fixture",CanRead:true},nil
  },func(context.Context,string)(ReviewEnvelope,error){return packet,nil})
  if err!=nil{t.Fatal(err)}
  r:=getFixture(handler,"/filing/reviews/"+ticket.ID,"","")
  if r.Code!=200{t.Fatalf("%s: status %d",lang.choice,r.Code)}
  body:=r.Body.String()
  for _,must:=range []string{
    "<html lang=\""+lang.htmlLang+"\">",
    lang.title,
    "dir=\"auto\"",
    "<main>",
    "aria-labelledby=",
    "&lt;script&gt;alert",
    "ใบแจ้งหนี้",
    "Rechnung",
  }{if !strings.Contains(body,must){t.Errorf("%s: missing %q",lang.choice,must)}}
  if strings.Contains(body,"<script>")||strings.Contains(body,"<form")||strings.Contains(body,"<button"){
   t.Fatal("untrusted metadata became active UI or editable control")
  }
  if !strings.Contains(r.Header().Get("Cache-Control"),"no-store"){t.Fatal("private data cached")}
 }
}
func TestReviewTransportRefusesUnqualifiedLoaderPackets(t *testing.T){
 _,_,_,id:=reviewerTransportFixture(t)
 originalTicket,record,hints:=acceptedReviewFixture()
 valid,err:=BuildReviewEnvelope(originalTicket,record,hints,ReviewThai)
 if err!=nil{t.Fatal(err)}
 for _,bad:=range []func(*ReviewEnvelope){
  func(p *ReviewEnvelope){p.AutoApplyAllowed=true},
  func(p *ReviewEnvelope){p.RequiresHumanReview=false},
  func(p *ReviewEnvelope){p.ClassificationQualified=true},
  func(p *ReviewEnvelope){p.PaperlessDocumentID=0},
  func(p *ReviewEnvelope){p.Language="fr"},
  func(p *ReviewEnvelope){p.EffectID=strings.Repeat("0",64)},
  func(p *ReviewEnvelope){p.Risks=nil},
 }{
  candidate:=valid
  bad(&candidate)
  handler,err:=NewReviewHandler(
   func(*http.Request,string)(ReviewPrincipal,error){return ReviewPrincipal{ActorID:"fixture",CanRead:true},nil},
   func(context.Context,string)(ReviewEnvelope,error){return candidate,nil})
  if err!=nil{t.Fatal(err)}
  rec:=getFixture(handler,"/api/filing/reviews/"+id,"","")
  if rec.Code!=http.StatusServiceUnavailable {
   t.Errorf("unsafe loader packet exposed status=%d",rec.Code)
  }
 }
}
func TestReviewTransportUnavailableAndNoErrorLeak(t *testing.T){
 _,_,_,id:=reviewerTransportFixture(t)
 handler,err:=NewReviewHandler(
  func(*http.Request,string)(ReviewPrincipal,error){return ReviewPrincipal{ActorID:"fixture",CanRead:true},nil},
  func(context.Context,string)(ReviewEnvelope,error){
   return ReviewEnvelope{},errors.New("internal secret paperless token should never leak")
  })
 if err!=nil{t.Fatal(err)}
 rec:=getFixture(handler,"/filing/reviews/"+id,"","")
 if rec.Code!=http.StatusServiceUnavailable||strings.Contains(rec.Body.String(),"secret"){
  t.Fatal("review service exposed internal credentials")
 }
}
