package paperless

import (
 "context"
 "errors"
 "net/http"
 "strings"
 "testing"
)

func TestNativeReviewURLIsFromOperatorOriginOnly(t *testing.T){
 c,err:=NewClient(Config{
  BaseURL:"https://paperless.example.invalid",Token:"never-export-this-token",
 },nil)
 if err!=nil{t.Fatal(err)}
 ticket,record,hints:=acceptedReviewFixture()
 record.Title="ใบแจ้งหนี้ https://attacker.example.invalid/evil?token=1"
 packet,err:=BuildReviewEnvelope(ticket,record,hints,ReviewThai)
 if err!=nil{t.Fatal(err)}
 link,err:=NativeReviewURL(c,packet)
 if err!=nil{t.Fatal(err)}
 if link!="https://paperless.example.invalid/documents/48"{
   t.Fatalf("Native UI route unexpectedly derived from model content: %q",link)
 }
 for _,bad:=range []string{"never-export-this-token","attacker","evil","?","%3F","@"}{
  if strings.Contains(link,bad){t.Fatalf("native link leaked token or document-selected destination: %s",link)}
 }
 packet.AutoApplyAllowed=true
 if _,err:=NativeReviewURL(c,packet);!errors.Is(err,ErrReviewEndpoint) {
   t.Fatalf("unsafe envelope received Paperless UI URL: %v",err)
 }
}
func TestNativePaperlessLinkOnlyVisibleInAuthorizedHTML(t *testing.T){
 c,err:=NewClient(Config{
  BaseURL:"https://paperless.example.invalid",Token:"dummy-token-for-local-tests",
 },nil)
 if err!=nil{t.Fatal(err)}
 ticket,record,hints:=acceptedReviewFixture()
 record.Title="ไม่ใช่ใบแจ้งหนี้ <script>fetch('https://evil')</script>"
 packet,err:=BuildReviewEnvelope(ticket,record,hints,ReviewThai)
 if err!=nil{t.Fatal(err)}
 handler,err:=NewReviewHandler(
  func(r *http.Request,id string)(ReviewPrincipal,error){
   if r.Header.Get("Authorization")!="Bearer fixture-auth"||id!=packet.EffectID{
     return ReviewPrincipal{},ErrReview
   }
   return ReviewPrincipal{ActorID:"reviewer",CanRead:true},nil
  },
  func(_ context.Context,effect string)(ReviewEnvelope,error){
   if effect!=packet.EffectID{return ReviewEnvelope{},ErrReview}
   return packet,nil
  },c)
 if err!=nil{t.Fatal(err)}
 unauth:=getFixture(handler,"/filing/reviews/"+ticket.ID,"","")
 if unauth.Code!=http.StatusUnauthorized||strings.Contains(unauth.Body.String(),"paperless.example.invalid"){
  t.Fatal("native link exposed to unapproved actor")
 }
 view:=getFixture(handler,"/filing/reviews/"+ticket.ID,"Bearer fixture-auth","")
 if view.Code!=http.StatusOK{t.Fatalf("HTML not available: %d",view.Code)}
 for _,item:=range []string{
  "https://paperless.example.invalid/documents/48",
  "rel=\"noreferrer noopener\"",
  "เปิดเอกสารใน Paperless-ngx",
  "ต้องตรวจสอบ",
  "&lt;script&gt;",
 } {
  if !strings.Contains(view.Body.String(),item){t.Fatalf("missing safe review handoff: %q",item)}
 }
 if strings.Contains(view.Body.String(),"<script>")||
    strings.Contains(view.Body.String(),"dummy-token")||
    strings.Contains(view.Body.String(),"href=\"https://evil"){
    t.Fatal("untrusted source/credentials exposed as active link or script")
 }
 api:=getFixture(handler,"/api/filing/reviews/"+ticket.ID,"Bearer fixture-auth","")
 if api.Code!=http.StatusOK||strings.Contains(api.Body.String(),"paperless.example.invalid"){
   t.Fatal("typed read-only JSON output acquired unapproved navigation credential")
 }
}
func TestNativeReviewOriginOptInOnlyAndInvalidOriginFailsClosed(t *testing.T){
 ticket,record,hints:=acceptedReviewFixture()
 packet,err:=BuildReviewEnvelope(ticket,record,hints,ReviewEnglish)
 if err!=nil{t.Fatal(err)}
 authorize:=func(*http.Request,string)(ReviewPrincipal,error){
   return ReviewPrincipal{ActorID:"viewer",CanRead:true},nil
 }
 load:=func(context.Context,string)(ReviewEnvelope,error){return packet,nil}
 if _,err:=NewReviewHandler(authorize,load,(*Client)(nil));!errors.Is(err,ErrReviewEndpoint){
    t.Fatal("nil origin opt-in accepted")
 }
 if _,err:=NewReviewHandler(authorize,load,&Client{},&Client{});!errors.Is(err,ErrReviewEndpoint){
   t.Fatal("multiple conflicting origins accepted")
 }
 plain,err:=NewReviewHandler(authorize,load)
 if err!=nil{t.Fatal(err)}
 html:=getFixture(plain,"/filing/reviews/"+ticket.ID,"","")
 if html.Code!=http.StatusOK||strings.Contains(html.Body.String(),"href="){
    t.Fatal("unconfigured DMS link rendered")
 }
 // Internal package test constructs a malformed Client to prove that an
 // origin must be validated again even when a caller bypasses NewClient.
 badClient:=&Client{endpoint:"https://evil.example.invalid/subdir?token=secret"}
 if _,err:=NativeReviewURL(badClient,packet);!errors.Is(err,ErrReviewEndpoint){
   t.Fatalf("arbitrary URL admitted: %v",err)
 }
 handler,err:=NewReviewHandler(authorize,load,badClient)
 if err!=nil{t.Fatal(err)}
 failed:=getFixture(handler,"/filing/reviews/"+ticket.ID,"","")
 if failed.Code!=http.StatusServiceUnavailable||strings.Contains(failed.Body.String(),"evil"){
   t.Fatal("unsafe origin leaked into HTML response")
 }
}
