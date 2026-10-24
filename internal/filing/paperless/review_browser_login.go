package paperless

import (
 "embed"
 "net/http"
 "net/url"
)

// These bundled, static, auditable assets have no third-party scripts,
// inline JavaScript, external fonts, analytics or model integrations.
// Language selection is independent of a document's OCR content.
//
//go:embed review_login_en.html review_login_de.html review_login_th.html review_login.js
var reviewLoginAssets embed.FS

// serveReviewLoginAsset is called only inside the existing direct-TLS,
// approved-Host browser session handler. It cannot mint a session or read a
// Paperless document. The token field stays in the browser and POSTs via
// same-origin HTTPS Authorization header, NEVER URL, form GET or storage.
func serveReviewLoginAsset(w http.ResponseWriter,r *http.Request)bool {
 switch r.URL.Path{
 case "/filing/review-login":
  if r.Method!=http.MethodGet {
   w.Header().Set("Allow","GET")
   http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
   return true
  }
  values,err:=url.ParseQuery(r.URL.RawQuery)
  if err!=nil||len(values)>2 {
    http.Error(w,"invalid review login query",http.StatusBadRequest)
    return true
  }
  for key,list:=range values{
    if (key!="lang"&&key!="effect")||len(list)!=1 {
      http.Error(w,"invalid review login query",http.StatusBadRequest)
      return true
    }
  }
  if effect,exists:=values["effect"];exists&&!shaRe.MatchString(effect[0]){
    http.Error(w,"invalid review record reference",http.StatusBadRequest)
    return true
  }
  language:=values.Get("lang")
  filename:="review_login_en.html"
  switch language{
  case "","en":filename="review_login_en.html"
  case "de":filename="review_login_de.html"
  case "th":filename="review_login_th.html"
  default:
   http.Error(w,"unsupported language",http.StatusBadRequest)
   return true
  }
  page,err:=reviewLoginAssets.ReadFile(filename)
  if err!=nil {
   http.Error(w,"review unavailable",http.StatusServiceUnavailable)
   return true
  }
  // The one trusted external JS file is only used on the login page. The
  // actual read-only review screen remains script-disabled.
  w.Header().Set("Content-Security-Policy",
   "default-src 'none'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
  w.Header().Set("Content-Type","text/html; charset=utf-8")
  w.WriteHeader(http.StatusOK)
  _,_=w.Write(page)
  return true
 case "/filing/review-login.js":
  if r.Method!=http.MethodGet {
   w.Header().Set("Allow","GET")
   http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
   return true
  }
  if r.URL.RawQuery!=""{
    http.NotFound(w,r)
    return true
  }
  script,err:=reviewLoginAssets.ReadFile("review_login.js")
  if err!=nil {
   http.Error(w,"review unavailable",http.StatusServiceUnavailable)
   return true
  }
  w.Header().Set("Content-Type","text/javascript; charset=utf-8")
  w.WriteHeader(http.StatusOK)
  _,_=w.Write(script)
  return true
 default:return false
 }
}
