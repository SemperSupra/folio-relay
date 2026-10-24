package paperless

import (
 "context"
 "crypto/aes"
 "crypto/cipher"
 "crypto/rand"
 "crypto/sha256"
 "crypto/subtle"
 "encoding/base64"
 "encoding/json"
 "errors"
 "io"
 "mime"
 "net/http"
 "net/url"
 "strings"
 "time"
)

// BrowserReviewSession does not reuse FolioRelay management/admin cookies,
// trust proxy-authenticated actor headers or introduce DMS write authority.
// A separately issued, 32-byte per-effect bearer can be exchanged ONCE over
// direct TLS for an AEAD-sealed, host-only Secure/HttpOnly/SameSite=Strict cookie.
// GET access always rechecks live grant state for expiry/revocation and scope.
//
// The grant issuer, authoritative revocation store, TLS listener and browser
// credential-entry UX are intentionally external. Nothing is registered on the
// running FolioRelay daemon by this source-only helper.
const reviewBrowserCookie = "foliorelay_review_scoped"
const reviewBrowserCookiePath = "/filing/reviews/"
const reviewBrowserTTL = 15*time.Minute
var ErrBrowserReviewSession = errors.New("filing browser session not admitted")

type browserClaims struct {
 EffectID string
 ActorID string
 GrantHash [32]byte
 ExpiresUnix int64
}

// NewReviewBrowserHandler combines a TLS-only credential exchange endpoint
// POST /filing/review-sessions with read-only HTML
// GET /filing/reviews/<effect>, never /api/... or management routes.
// publicOrigin is an operator-approved exact HTTPS origin, not derived
// from request Host, proxy forwarding headers, DMS, model or OCR content.
// The separate random 32-byte AES-GCM key MUST NOT be the management token.
func NewReviewBrowserHandler(lookup ReviewGrantLookup,load ReviewLoader,
 publicOrigin string,sessionKey []byte,now func()time.Time,
 nativeClient ...*Client)(http.Handler,error){
 if lookup==nil||load==nil||len(sessionKey)!=32||len(nativeClient)>1 {
  return nil,ErrBrowserReviewSession
 }
 parsed,err:=url.Parse(publicOrigin)
 if err!=nil||parsed.Scheme!="https"||parsed.Host==""||parsed.User!=nil||
  parsed.Path!=""||parsed.RawQuery!=""||parsed.Fragment!=""||
  strings.Contains(parsed.Host,"%")||parsed.Hostname()==""{
  return nil,ErrBrowserReviewSession
 }
 if now==nil{now=time.Now}
 block,err:=aes.NewCipher(sessionKey)
 if err!=nil{return nil,ErrBrowserReviewSession}
 aead,err:=cipher.NewGCM(block)
 if err!=nil{return nil,ErrBrowserReviewSession}
 grantAuth,err:=NewScopedReviewAuthorizer(lookup,now)
 if err!=nil{return nil,ErrBrowserReviewSession}

 validGrant:=func(g ReviewGrant,hash [32]byte,effect,actor string,at time.Time)bool{
  return subtle.ConstantTimeCompare(g.TokenHash[:],hash[:])==1 &&
   identRe.MatchString(g.ActorID)&&g.ActorID==actor&&
   shaRe.MatchString(g.EffectID)&&g.EffectID==effect&&
   g.CanRead&&!g.Revoked&&
   !g.ValidFrom.IsZero()&&!g.ExpiresAt.IsZero()&&
   !at.Before(g.ValidFrom)&&at.Before(g.ExpiresAt)&&
   g.ExpiresAt.After(g.ValidFrom)
 }
 cookieAuth:=func(r *http.Request,effect string)(ReviewPrincipal,error){
  if r==nil||r.TLS==nil||r.Host!=parsed.Host||!shaRe.MatchString(effect) {
   return ReviewPrincipal{},ErrBrowserReviewSession
  }
  var raw string
  count:=0
  for _,c:=range r.Cookies(){
    if c.Name==reviewBrowserCookie{raw=c.Value;count++}
  }
  if count!=1||raw==""||len(raw)>2048{return ReviewPrincipal{},ErrBrowserReviewSession}
  encrypted,err:=base64.RawURLEncoding.DecodeString(raw)
  if err!=nil||len(encrypted)<=aead.NonceSize()+aead.Overhead(){
   return ReviewPrincipal{},ErrBrowserReviewSession
  }
  nonce:=encrypted[:aead.NonceSize()]
  sealed:=encrypted[aead.NonceSize():]
  plain,err:=aead.Open(nil,nonce,sealed,[]byte("foliorelay-paperless-browser-review/v1"))
  if err!=nil{return ReviewPrincipal{},ErrBrowserReviewSession}
  var claims browserClaims
  if err:=json.Unmarshal(plain,&claims);err!=nil||
   claims.EffectID!=effect||!identRe.MatchString(claims.ActorID)||
   now().Unix()>=claims.ExpiresUnix {
   return ReviewPrincipal{},ErrBrowserReviewSession
  }
  grant,err:=lookup(claims.GrantHash)
  if err!=nil||!validGrant(grant,claims.GrantHash,effect,claims.ActorID,now()) {
   return ReviewPrincipal{},ErrBrowserReviewSession
  }
  return ReviewPrincipal{ActorID:claims.ActorID,CanRead:true},nil
 }
 review,err:=NewReviewHandler(cookieAuth,load,nativeClient...)
 if err!=nil{return nil,ErrBrowserReviewSession}
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Cache-Control","private, no-store")
  w.Header().Set("X-Content-Type-Options","nosniff")
  w.Header().Set("Referrer-Policy","no-referrer")
  w.Header().Set("X-Frame-Options","DENY")
  w.Header().Set("Content-Security-Policy","default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
  w.Header().Set("Vary","Origin")
  if r.TLS==nil||r.Host!=parsed.Host {
    http.Error(w,"browser review unavailable",http.StatusUnauthorized);return
  }
  if r.URL.Path=="/filing/review-sessions" {
   if r.Method!=http.MethodPost{
    w.Header().Set("Allow","POST")
    http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
   }
   // Exact same-origin, JSON-only exchange with non-simple custom header.
   // Forged X-Forwarded-* headers are never consulted.
   origins:=r.Header.Values("Origin")
   if len(origins)!=1||origins[0]!=publicOrigin||
     r.Header.Get("X-FolioRelay-Review-Session")!="1"{
    http.Error(w,"review exchange denied",http.StatusForbidden);return
   }
   if site:=r.Header.Get("Sec-Fetch-Site");site!=""&&site!="same-origin"{
    http.Error(w,"review exchange denied",http.StatusForbidden);return
   }
   mediatype,_,err:=mime.ParseMediaType(r.Header.Get("Content-Type"))
   if err!=nil||mediatype!="application/json"{
    http.Error(w,"unsupported body",http.StatusUnsupportedMediaType);return
   }
   body:=http.MaxBytesReader(w,r.Body,1024)
   defer body.Close()
   var payload map[string]string
   decoder:=json.NewDecoder(body)
   if err:=decoder.Decode(&payload);err!=nil||len(payload)!=1||
      !shaRe.MatchString(payload["effect_id"]) {
     http.Error(w,"invalid session request",http.StatusBadRequest);return
   }
   var extra any
   if err:=decoder.Decode(&extra);err!=io.EOF{
     http.Error(w,"invalid session request",http.StatusBadRequest);return
   }
   effect:=payload["effect_id"]
   actor,err:=grantAuth(r,effect)
   if err!=nil||!actor.CanRead||actor.ActorID==""{
     http.Error(w,"review exchange denied",http.StatusUnauthorized);return
   }
   token:=strings.TrimPrefix(r.Header.Get("Authorization"),"Bearer ")
   secret,err:=base64.RawURLEncoding.DecodeString(token)
   if err!=nil||len(secret)!=32{
     http.Error(w,"review exchange denied",http.StatusUnauthorized);return
   }
   digest:=sha256.Sum256(secret)
   current:=now()
   grant,err:=lookup(digest)
   if err!=nil||!validGrant(grant,digest,effect,actor.ActorID,current) {
     http.Error(w,"review exchange denied",http.StatusUnauthorized);return
   }
   expires:=current.Add(reviewBrowserTTL)
   if grant.ExpiresAt.Before(expires){expires=grant.ExpiresAt}
   if !expires.After(current.Add(time.Second)){
     http.Error(w,"review exchange denied",http.StatusUnauthorized);return
   }
   claims:=browserClaims{EffectID:effect,ActorID:actor.ActorID,
     GrantHash:digest,ExpiresUnix:expires.Unix()}
   plain,err:=json.Marshal(claims)
   if err!=nil{
     http.Error(w,"review exchange unavailable",http.StatusServiceUnavailable);return
   }
   nonce:=make([]byte,aead.NonceSize())
   if _,err:=rand.Read(nonce);err!=nil{
     http.Error(w,"review exchange unavailable",http.StatusServiceUnavailable);return
   }
   ciphertext:=aead.Seal(nil,nonce,plain,[]byte("foliorelay-paperless-browser-review/v1"))
   value:=base64.RawURLEncoding.EncodeToString(append(nonce,ciphertext...))
   http.SetCookie(w,&http.Cookie{
     Name:reviewBrowserCookie,Value:value,
     Path:reviewBrowserCookiePath,Secure:true,HttpOnly:true,
     SameSite:http.SameSiteStrictMode,
     Expires:expires,MaxAge:int(expires.Sub(current).Seconds()),
   })
   w.Header().Set("Content-Type","application/json; charset=utf-8")
   w.WriteHeader(http.StatusCreated)
   _=json.NewEncoder(w).Encode(map[string]string{
     "review_path":reviewBrowserCookiePath+effect,
   })
   return
  }
  if strings.HasPrefix(r.URL.Path,reviewBrowserCookiePath) {
   if r.Method!=http.MethodGet{
    w.Header().Set("Allow","GET")
    http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return
   }
   // The existing renderer reauthorizes the cookie by effect ID BEFORE
   // consulting the native Paperless loader. No API auth inheritance.
   review.ServeHTTP(w,r)
   return
  }
  http.NotFound(w,r)
 }),nil
}

// ReviewBrowserContextProbe is a deliberate compile-time reminder that
// the handler and its injected loader use request context, not a background
// task or a scheduler that could outlive the user's scope.
var _ context.Context
