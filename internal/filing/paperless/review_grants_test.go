package paperless

import (
 "context"
 "crypto/sha256"
 "encoding/base64"
 "errors"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
)

func capabilityFixture()(string,[32]byte) {
 // Deterministic and synthetic! This is a unit-test credential only, not
 // cryptographically random and NEVER permitted for deployed provisioning.
 raw:=[]byte("0123456789abcdef0123456789abcdef")
 if len(raw)!=32 {panic("test-only credential fixture is not 32 bytes")}
 return base64.RawURLEncoding.EncodeToString(raw),sha256.Sum256(raw)
}
func TestScopedReviewerRequiresExactEffectAndUnexpiredGrant(t *testing.T){
 token,hash:=capabilityFixture()
 now:=time.Date(2026,10,24,8,30,0,0,time.UTC)
 effect1,effect2:=strings.Repeat("a",64),strings.Repeat("b",64)
 grant:=ReviewGrant{TokenHash:hash,ActorID:"reviewer-a",
   EffectID:effect1,ValidFrom:now.Add(-time.Hour),
   ExpiresAt:now.Add(time.Hour),CanRead:true}
 resolverCalls:=0
 lookup:=func(got [32]byte)(ReviewGrant,error){
   resolverCalls++
   if got!=hash{return ReviewGrant{},ErrReviewGrant}
   return grant,nil
 }
 authorize,err:=NewScopedReviewAuthorizer(lookup,func()time.Time{return now})
 if err!=nil{t.Fatal(err)}
 makeRequest:=func(effect,credential string)*http.Request {
    req:=httptest.NewRequest(http.MethodGet,"/filing/reviews/"+effect,nil)
    if credential!=""{req.Header.Set("Authorization",credential)}
    return req
 }
 principal,err:=authorize(makeRequest(effect1,"Bearer "+token),effect1)
 if err!=nil||!principal.CanRead||principal.ActorID!="reviewer-a"{
   t.Fatalf("genuine scoped actor refused: %#v %v",principal,err)
 }
 if _,err:=authorize(makeRequest(effect2,"Bearer "+token),effect2);!errors.Is(err,ErrReviewGrant){
   t.Fatal("one effect credential crossed into a different document")
 }
 if _,err:=authorize(makeRequest(effect1,"Bearer management-admin-token"),effect1);!errors.Is(err,ErrReviewGrant){
   t.Fatal("management/admin token granted document review")
 }
 if _,err:=authorize(makeRequest(effect1,""),effect1);!errors.Is(err,ErrReviewGrant){
   t.Fatal("anonymous reviewer permitted")
 }
 if resolverCalls!=2{t.Fatalf("malformed credentials reached lookup: %d",resolverCalls)}
 grant.CanRead=false
 if _,err:=authorize(makeRequest(effect1,"Bearer "+token),effect1);!errors.Is(err,ErrReviewGrant){t.Fatal("write-only grant became readable")}
 grant.CanRead=true
 grant.Revoked=true
 if _,err:=authorize(makeRequest(effect1,"Bearer "+token),effect1);!errors.Is(err,ErrReviewGrant){t.Fatal("revoked grant still admitted")}
 grant.Revoked=false
 grant.ExpiresAt=now
 if _,err:=authorize(makeRequest(effect1,"Bearer "+token),effect1);!errors.Is(err,ErrReviewGrant){t.Fatal("expired grant still admitted")}
 grant.ExpiresAt=now.Add(time.Hour)
 grant.ValidFrom=now.Add(time.Minute)
 if _,err:=authorize(makeRequest(effect1,"Bearer "+token),effect1);!errors.Is(err,ErrReviewGrant){t.Fatal("future-dated grant admitted")}
}
func TestReviewTokensCannotBeSpoofedViaProxyCookieOrQuery(t *testing.T){
 token,hash:=capabilityFixture()
 now:=time.Date(2026,10,24,8,30,0,0,time.UTC)
 effect:=strings.Repeat("a",64)
 resolver:=func(got [32]byte)(ReviewGrant,error){
  if got!=hash{return ReviewGrant{},ErrReviewGrant}
  return ReviewGrant{TokenHash:hash,ActorID:"viewer",
    EffectID:effect,ValidFrom:now.Add(-time.Second),
    ExpiresAt:now.Add(time.Second),CanRead:true},nil
 }
 auth,err:=NewScopedReviewAuthorizer(resolver,func()time.Time{return now})
 if err!=nil{t.Fatal(err)}
 req:=httptest.NewRequest("GET","/filing/reviews/"+effect+"?token="+token,nil)
 req.Header.Set("X-Forwarded-User","administrator")
 req.Header.Set("X-Actor","trusted-actor")
 req.Header.Set("X-User","reviewer-a")
 req.AddCookie(&http.Cookie{Name:"foliorelay_session",Value:token})
 if _,err:=auth(req,effect);!errors.Is(err,ErrReviewGrant){
   t.Fatal("proxy-supplied principal, cookie or URL token bypassed scoped bearer")
 }
 for _,bad:=range []string{
  "Bearer "+strings.Repeat("A",43),
  "Bearer "+token+"=",
  "Basic "+token,
  "bearer "+token,
  "Bearer "+token+" extra",
  "Bearer "+token[:42]+"!",
  "Bearer "+strings.Repeat("a",64),
 }{
  req.Header.Set("Authorization",bad)
  if _,err:=auth(req,effect);!errors.Is(err,ErrReviewGrant){
   t.Fatalf("invalid wire bearer admitted: %q",bad[:10])
  }
 }
 req.Header.Set("Authorization","Bearer "+token)
 if p,err:=auth(req,effect);err!=nil||p.ActorID!="viewer"{
   t.Fatalf("genuine token failed with forged proxy headers: %#v %v",p,err)
 }
}
func TestScopedReviewHandlerDeniesCrossDocumentAndDisclosesNoSecrets(t *testing.T){
 token,hash:=capabilityFixture()
 ticket,record,hints:=acceptedReviewFixture()
 packet,err:=BuildReviewEnvelope(ticket,record,hints,ReviewThai)
 if err!=nil{t.Fatal(err)}
 effectOther:=strings.Repeat("0",64)
 now:=time.Date(2026,10,24,8,30,0,0,time.UTC)
 readCalls:=0
 grant:=ReviewGrant{
  TokenHash:hash,ActorID:"operator-thai",EffectID:packet.EffectID,
  ValidFrom:now.Add(-time.Hour),ExpiresAt:now.Add(time.Hour),CanRead:true,
 }
 lookup:=func(got [32]byte)(ReviewGrant,error){
  if got!=hash{return ReviewGrant{},ErrReviewGrant}
  return grant,nil
 }
 auth,err:=NewScopedReviewAuthorizer(lookup,func()time.Time{return now})
 if err!=nil{t.Fatal(err)}
 handler,err:=NewReviewHandler(auth,func(_ context.Context,got string)(ReviewEnvelope,error){
  readCalls++
  if got!=packet.EffectID{return ReviewEnvelope{},ErrReview}
  return packet,nil
 })
 if err!=nil{t.Fatal(err)}
 call:=func(uri,credential string)*httptest.ResponseRecorder{
  req:=httptest.NewRequest(http.MethodGet,uri,nil)
  if credential!=""{req.Header.Set("Authorization",credential)}
  recorder:=httptest.NewRecorder()
  handler.ServeHTTP(recorder,req)
  return recorder
 }
 for _,uri:=range []string{
  "/api/filing/reviews/"+effectOther,
  "/filing/reviews/"+effectOther,
 }{
  response:=call(uri,"Bearer "+token)
  if response.Code!=http.StatusUnauthorized||readCalls!=0 {
   t.Fatalf("cross-record access resulted in %d with %d document loads",response.Code,readCalls)
  }
 }
 for _,uri:=range []string{
  "/api/filing/reviews/"+packet.EffectID,
  "/filing/reviews/"+packet.EffectID,
 }{
  response:=call(uri,"Bearer "+token)
  if response.Code!=http.StatusOK{t.Fatalf("valid scoped review rejected: %d",response.Code)}
  if strings.Contains(response.Body.String(),token)||
    strings.Contains(response.Body.String(),"approved-001")||
    strings.Contains(response.Body.String(),"Bearer "){
    t.Fatal("raw credential or approval ID leaked")
  }
  if !strings.Contains(response.Body.String(),"ใบแจ้งหนี้"){
    t.Fatal("Thai UTF-8 view lost")
  }
 }
 if readCalls!=2 {t.Fatalf("only authorized records may be loaded; got %d",readCalls)}
 grant.Revoked=true
 again:=call("/api/filing/reviews/"+packet.EffectID,"Bearer "+token)
 if again.Code!=http.StatusUnauthorized||readCalls!=2{
  t.Fatal("revocation did not take effect on next request")
 }
}
func TestScopedReviewConstructorDeniesAbsentGrantResolver(t *testing.T){
 if _,err:=NewScopedReviewAuthorizer(nil,nil);!errors.Is(err,ErrReviewGrant){
  t.Fatal("missing trusted grant source accepted")
 }
}
