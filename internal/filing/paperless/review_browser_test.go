package paperless

import (
 "context"
 "crypto/tls"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
 "time"
)

const testBrowserOrigin="https://reviews.example.test"
func browserReviewFixture(t *testing.T)(http.Handler,*ReviewGrant,*time.Time,*int,string,string,[]byte){
 t.Helper()
 token,hash:=capabilityFixture()
 ticket,record,hints:=acceptedReviewFixture()
 record.Title="ใบแจ้งหนี้ / Rechnung <script>steal()</script>"
 packet,err:=BuildReviewEnvelope(ticket,record,hints,ReviewThai)
 if err!=nil{t.Fatal(err)}
 now:=time.Date(2026,10,24,11,0,0,0,time.UTC)
 grant:=&ReviewGrant{TokenHash:hash,ActorID:"human-reviewer-1",
  EffectID:ticket.ID,ValidFrom:now.Add(-time.Minute),
  ExpiresAt:now.Add(time.Hour),CanRead:true}
 getCalls:=0
 key:=[]byte("separate-browser-review-key-32b!!")
 if len(key)!=32 {t.Fatalf("bad fixture AES key size: %d",len(key))}
 handler,err:=NewReviewBrowserHandler(func(got [32]byte)(ReviewGrant,error){
   if got!=hash{return ReviewGrant{},ErrReviewGrant}
   return *grant,nil
 },func(_ context.Context,effect string)(ReviewEnvelope,error){
   getCalls++
   if effect!=ticket.ID{return ReviewEnvelope{},ErrReviewUnavailable}
   return packet,nil
 },testBrowserOrigin,key,func()time.Time{return now})
 if err!=nil{t.Fatal(err)}
 return handler,grant,&now,&getCalls,ticket.ID,token,key
}
func browserRequest(h http.Handler,method,path,body string,headers map[string]string,cookies ...*http.Cookie)*httptest.ResponseRecorder{
 request:=httptest.NewRequest(method,testBrowserOrigin+path,strings.NewReader(body))
 request.TLS=&tls.ConnectionState{}
 for k,v:=range headers{request.Header.Set(k,v)}
 for _,cookie:=range cookies{request.AddCookie(cookie)}
 result:=httptest.NewRecorder()
 h.ServeHTTP(result,request)
 return result
}
func submitBrowserGrant(h http.Handler,id,token string)*httptest.ResponseRecorder{
 return browserRequest(h,http.MethodPost,"/filing/review-sessions",
   `{"effect_id":"`+id+`"}`,map[string]string{
    "Authorization":"Bearer "+token,
    "Origin":testBrowserOrigin,
    "Sec-Fetch-Site":"same-origin",
    "X-FolioRelay-Review-Session":"1",
    "Content-Type":"application/json",
 })
}
func TestBrowserReviewSessionExchangeAndHumanThaiRead(t *testing.T){
 h,_,_,reads,id,token,_:=browserReviewFixture(t)
 unauth:=browserRequest(h,"GET","/filing/reviews/"+id,"",nil)
 if unauth.Code!=401||*reads!=0{t.Fatal("document metadata leaked before login")}
 exchange:=submitBrowserGrant(h,id,token)
 if exchange.Code!=201{t.Fatalf("session exchange rejected: %d %s",exchange.Code,exchange.Body.String())}
 if *reads!=0{t.Fatal("session mint accessed Paperless before actual GET")}
 var response map[string]string
 if err:=json.Unmarshal(exchange.Body.Bytes(),&response);err!=nil{t.Fatal(err)}
 if response["review_path"]!="/filing/reviews/"+id{t.Fatalf("bad review navigation: %#v",response)}
 if strings.Contains(exchange.Body.String(),token)||strings.Contains(exchange.Body.String(),"approved-001"){
  t.Fatal("session endpoint disclosed credential or approval evidence")
 }
 cookies:=exchange.Result().Cookies()
 if len(cookies)!=1{t.Fatalf("expected scoped browser cookie, got %d",len(cookies))}
 cookie:=cookies[0]
 if !cookie.Secure||!cookie.HttpOnly||cookie.SameSite!=http.SameSiteStrictMode||
  cookie.Path!="/filing/reviews/"||cookie.Name!=reviewBrowserCookie{
  t.Fatalf("insecure browser cookie attributes: %+v",cookie)
 }
 if strings.Contains(cookie.Value,token)||len(cookie.Value)<60{
  t.Fatal("session cookie contained raw bearer or was unsealed")
 }
 for _,path:=range []string{"/filing/reviews/"+id}{
  reviewed:=browserRequest(h,"GET",path,"",nil,cookie)
  if reviewed.Code!=200{t.Fatalf("authorized browser GET denied: %d",reviewed.Code)}
  if !strings.Contains(reviewed.Body.String(),"ใบแจ้งหนี้")||
   !strings.Contains(reviewed.Body.String(),"&lt;script&gt;")||
   strings.Contains(reviewed.Body.String(),"<script>") {
   t.Fatal("unsafe or unreadable Thai browser review HTML")
  }
  if strings.Contains(reviewed.Body.String(),token){t.Fatal("raw token leaked on reviewer HTML")}
 }
 if *reads!=1{t.Fatalf("unexpected Paperless loader reads: %d",*reads)}
 // No cookie-based access to machine API / management endpoints.
 api:=browserRequest(h,"GET","/api/filing/reviews/"+id,"",nil,cookie)
 if api.Code!=404||*reads!=1{t.Fatal("HTML session escaped into machine API")}
}
func TestBrowserSessionRevocationExpiryAndCrossDocumentDeny(t *testing.T){
 h,grant,now,reads,id,token,_:=browserReviewFixture(t)
 exchange:=submitBrowserGrant(h,id,token)
 if exchange.Code!=201{t.Fatal("fixture exchange failed")}
 cookie:=exchange.Result().Cookies()[0]
 invalidID:=strings.Repeat("e",64)
 cross:=browserRequest(h,"GET","/filing/reviews/"+invalidID,"",nil,cookie)
 if cross.Code!=401||*reads!=0{t.Fatal("cookie granted other user's document")}
 wrong:=*cookie
 wrong.Value=wrong.Value[:len(wrong.Value)-2]+"xx"
 tampered:=browserRequest(h,"GET","/filing/reviews/"+id,"",nil,&wrong)
 if tampered.Code!=401||*reads!=0{t.Fatal("tampered session decrypted")}
 duplicate:=browserRequest(h,"GET","/filing/reviews/"+id,"",nil,cookie,cookie)
 if duplicate.Code!=401||*reads!=0{t.Fatal("ambiguous duplicate cookies allowed")}
 grant.Revoked=true
 revoked:=browserRequest(h,"GET","/filing/reviews/"+id,"",nil,cookie)
 if revoked.Code!=401||*reads!=0{t.Fatal("revoked grant retained browser access")}
 grant.Revoked=false
 *now=now.Add(reviewBrowserTTL)
 expired:=browserRequest(h,"GET","/filing/reviews/"+id,"",nil,cookie)
 if expired.Code!=401||*reads!=0{t.Fatal("past cookie expiration retained access")}
}
func TestBrowserSessionRejectsOriginDowngradeCSRFAndManagementCredentials(t *testing.T){
 h,_,_,reads,id,token,_:=browserReviewFixture(t)
 validHeaders:=map[string]string{
  "Authorization":"Bearer "+token,
  "Origin":testBrowserOrigin,
  "X-FolioRelay-Review-Session":"1",
  "Content-Type":"application/json",
 }
 payload:=`{"effect_id":"`+id+`"}`
 scenarios:=[]struct{name string; mutate func(map[string]string)}{
  {"cross-origin",func(h map[string]string){h["Origin"]="https://evil.example.test"}},
  {"missing origin",func(h map[string]string){delete(h,"Origin")}},
  {"cross-site fetch",func(h map[string]string){h["Sec-Fetch-Site"]="cross-site"}},
  {"same-site-other-subdomain",func(h map[string]string){h["Sec-Fetch-Site"]="same-site"}},
  {"missing CSRF header",func(h map[string]string){delete(h,"X-FolioRelay-Review-Session")}},
  {"wrong MIME",func(h map[string]string){h["Content-Type"]="application/x-www-form-urlencoded"}},
  {"management token",func(h map[string]string){h["Authorization"]="Bearer management-admin-token"}},
  {"query token",func(h map[string]string){delete(h,"Authorization")}},
 }
 for _,scenario:=range scenarios{
  t.Run(scenario.name,func(t *testing.T){
   headers:=map[string]string{}
   for k,v:=range validHeaders{headers[k]=v}
   scenario.mutate(headers)
   res:=browserRequest(h,http.MethodPost,"/filing/review-sessions",payload,headers)
   if res.Code==201||len(res.Result().Cookies())!=0 {
     t.Fatalf("untrusted browser session admitted: %s status %d",scenario.name,res.Code)
   }
  })
 }
 malformed:=browserRequest(h,http.MethodPost,"/filing/review-sessions",
  `{"effect_id":"`+id+`","admin":true}`,validHeaders)
 if malformed.Code==201{t.Fatal("unknown JSON property admitted")}
 wrongMethod:=browserRequest(h,"GET","/filing/review-sessions","",nil)
 if wrongMethod.Code!=405{t.Fatal("session minted from GET")}
 noTLS:=httptest.NewRequest("POST",testBrowserOrigin+"/filing/review-sessions",strings.NewReader(payload))
 noTLS.Host="reviews.example.test"
 noTLS.Header.Set("Origin",testBrowserOrigin)
 noTLS.Header.Set("X-Forwarded-Proto","https")
 noTLS.Header.Set("Authorization","Bearer "+token)
 noTLS.Header.Set("Content-Type","application/json")
 noTLS.Header.Set("X-FolioRelay-Review-Session","1")
 w:=httptest.NewRecorder()
 h.ServeHTTP(w,noTLS)
 if w.Code==201{t.Fatal("spoofed forwarded TLS granted session")}
 if *reads!=0{t.Fatal("unauthorized exchange caused DMS read")}
}
func TestBrowserSessionIndependentKeysAndHostBinding(t *testing.T){
 h,grant,_,_,id,token,key:=browserReviewFixture(t)
 exchange:=submitBrowserGrant(h,id,token)
 if exchange.Code!=201{t.Fatal("missing source session")}
 cookie:=exchange.Result().Cookies()[0]
 key2:=append([]byte(nil),key...)
 key2[0]^=0xff
 // Reconstruct a new instance with the same test grant but a different
 // key; a cookie minted by the other instance must not be decrypted.
 auth:=func(hash [32]byte)(ReviewGrant,error){
  if hash!=grant.TokenHash{return ReviewGrant{},ErrReviewGrant}
  return *grant,nil
 }
 next,err:=NewReviewBrowserHandler(auth,func(context.Context,string)(ReviewEnvelope,error){
  return ReviewEnvelope{},ErrReviewUnavailable
 },testBrowserOrigin,key2,func()time.Time{return time.Date(2026,10,24,11,0,0,0,time.UTC)})
 if err!=nil{t.Fatal(err)}
 response:=browserRequest(next,"GET","/filing/reviews/"+id,"",nil,cookie)
 if response.Code!=401{t.Fatal("cookie replayed across independently keyed service")}
 spoofed:=httptest.NewRequest("GET","https://other.example.test/filing/reviews/"+id,nil)
 spoofed.TLS=&tls.ConnectionState{}
 spoofed.AddCookie(cookie)
 w:=httptest.NewRecorder()
 h.ServeHTTP(w,spoofed)
 if w.Code!=401{t.Fatal("review cookie accepted at wrong public origin")}
}
func TestBrowserSessionRequiresTrustedOriginSecretAndGrantStore(t *testing.T){
 grant:=func([32]byte)(ReviewGrant,error){return ReviewGrant{},ErrReviewGrant}
 load:=func(context.Context,string)(ReviewEnvelope,error){return ReviewEnvelope{},ErrReviewUnavailable}
 valid:=[]byte("separate-browser-review-key-32b!!")
 for _,failure:=range []struct{origin string;key []byte;lookup ReviewGrantLookup;loader ReviewLoader}{
  {"http://reviews.example.test",valid,grant,load},
  {"https://reviews.example.test/path",valid,grant,load},
  {"https://evil@reviews.example.test",valid,grant,load},
  {"https://reviews.example.test?secret=1",valid,grant,load},
  {testBrowserOrigin,nil,grant,load},
  {testBrowserOrigin,[]byte("short"),grant,load},
  {testBrowserOrigin,valid,nil,load},
  {testBrowserOrigin,valid,grant,nil},
 }{
  if _,err:=NewReviewBrowserHandler(failure.lookup,failure.loader,
    failure.origin,failure.key,nil);err==nil{
   t.Fatalf("untrusted origin/key/provider accepted: %s",failure.origin)
  }
 }
}
