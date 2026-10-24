package paperless

import (
 "crypto/tls"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestBrowserLoginPagesAreLocalizedAndRequireHTTPS(t *testing.T){
 h,_,_,reads,_,_,_:=browserReviewFixture(t)
 for _,sample:=range []struct{lang,htmlTag,heading string}{
  {"en",`<html lang="en">`,"Document review sign-in"},
  {"de",`<html lang="de">`,"Anmeldung zur Dokumentenprüfung"},
  {"th",`<html lang="th">`,"เข้าสู่ระบบเพื่อตรวจสอบเอกสาร"},
 }{
  rec:=browserRequest(h,"GET","/filing/review-login?lang="+sample.lang,"",nil)
  if rec.Code!=200{t.Fatalf("%s login failed: %d",sample.lang,rec.Code)}
  body:=rec.Body.String()
  for _,needle:=range []string{sample.htmlTag,sample.heading,
   `autocomplete="off"`,`type="password"`,
   `aria-live="polite"`,`src="/filing/review-login.js"`}{
   if !strings.Contains(body,needle){t.Errorf("%s missing accessible form element %q",sample.lang,needle)}
  }
  if strings.Contains(body,"<script>")||strings.Contains(body,"http://")||
   strings.Contains(body,"localStorage")||strings.Contains(body,"foliorelay_session"){
    t.Fatal("login form includes inline code or management credentials")
  }
  policy:=rec.Header().Get("Content-Security-Policy")
  if !strings.Contains(policy,"script-src 'self'")||
    !strings.Contains(policy,"connect-src 'self'")||
    !strings.Contains(policy,"form-action 'none'"){
   t.Fatalf("login page CSP allowed external code or form submission: %s",policy)
  }
 }
 if *reads!=0{t.Fatal("unauthenticated login page loaded real record content")}
 bad:=browserRequest(h,"GET","/filing/review-login?lang=../../../secrets","",nil)
 if bad.Code!=400{t.Fatal("unsupported locale was accepted")}
 noTLS:=httptest.NewRequest("GET",testBrowserOrigin+"/filing/review-login",nil)
 noTLS.TLS=nil
 noTLS.Header.Set("X-Forwarded-Proto","https")
 w:=httptest.NewRecorder()
 h.ServeHTTP(w,noTLS)
 if w.Code!=401{t.Fatal("proxy-forged plaintext login page accepted")}
 wrongHost:=httptest.NewRequest("GET","https://untrusted.example.test/filing/review-login",nil)
 wrongHost.TLS=&tls.ConnectionState{}
 w=httptest.NewRecorder()
 h.ServeHTTP(w,wrongHost)
 if w.Code!=401{t.Fatal("unexpected public origin allowed")}
}
func TestBrowserLoginScriptKeepsGrantOffURLsAndBrowserStorage(t *testing.T){
 h,_,_,reads,_,_,_:=browserReviewFixture(t)
 rec:=browserRequest(h,"GET","/filing/review-login.js","",nil)
 if rec.Code!=200 {t.Fatalf("static login script status %d",rec.Code)}
 if rec.Header().Get("Cache-Control")!="private, no-store"{
  t.Fatal("credential-entry script should not be cached")
 }
 if !strings.HasPrefix(rec.Header().Get("Content-Type"),"text/javascript") {
  t.Fatal("script MIME type missing")
 }
 js:=rec.Body.String()
 for _,necessary:=range []string{
  `fetch("/filing/review-sessions"`,
  `"Authorization": "Bearer " + credential`,
  `tokenInput.value = "";`,
  `credentials: "same-origin"`,
  `redirect: "error"`,
  `window.location.assign(target)`,
  `status.textContent`,
 }{
  if !strings.Contains(js,necessary){t.Fatalf("review login missing secure client control: %q",necessary)}
 }
 for _,forbidden:=range []string{"localStorage","sessionStorage","console.log",
  "document.cookie","innerHTML","sendBeacon","XMLHttpRequest","?token=",
  "window.open","fetch('http"}{
  if strings.Contains(js,forbidden){t.Fatalf("login JS includes prohibited secret/URL path: %s",forbidden)}
 }
 query:=browserRequest(h,"GET","/filing/review-login.js?token=secret","",nil)
 if query.Code!=404{t.Fatal("script endpoint ignored credential-like URL query")}
 if *reads!=0{t.Fatal("login JS asset triggered native Paperless read")}
}
func TestBrowserLoginDoesNotAuthorizeManagementOrNativeDMSWrites(t *testing.T){
 h,_,_,reads,id,token,_:=browserReviewFixture(t)
 request:=browserRequest(h,http.MethodPost,"/filing/review-sessions",
  `{"effect_id":"`+id+`"}`,map[string]string{
   "Authorization":"Bearer "+token,
   "Origin":testBrowserOrigin,
   "X-FolioRelay-Review-Session":"1",
   "Content-Type":"application/json",
 })
 if request.Code!=201{t.Fatal("fixture cannot open scoped session")}
 cookie:=request.Result().Cookies()[0]
 for _,path:=range []string{"/api/v1/jobs","/api/filing/reviews/"+id,
    "/admin","/filing/review-sessions"} {
  rec:=browserRequest(h,"GET",path,"",nil,cookie)
  if rec.Code==200{t.Fatalf("browser cookie escaped into unrelated API: %s",path)}
 }
 post:=browserRequest(h,http.MethodPost,"/filing/reviews/"+id,"{}",map[string]string{
  "Content-Type":"application/json",
 },cookie)
 if post.Code!=405||*reads!=0{t.Fatal("browser route allowed filing mutations")}
}
