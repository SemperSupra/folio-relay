package paperless

import (
 "crypto/sha256"
 "crypto/subtle"
 "encoding/base64"
 "errors"
 "net/http"
 "strings"
 "time"
)

// ReviewGrant is a record-specific read-only capability. TokenHash is the
// SHA-256 digest of an independently generated, high-entropy 32-byte secret;
// raw credentials must NEVER be placed in Git, DLE, logs or review packets.
// No grant authorizes metadata corrections, DMS credentials or a file path.
type ReviewGrant struct {
 TokenHash [32]byte
 ActorID string
 EffectID string
 ValidFrom time.Time
 ExpiresAt time.Time
 CanRead bool
 Revoked bool
}

// ReviewGrantLookup is an application-provided, request-time resolver.
// It MUST read authoritative, revocable grants from a trusted source;
// no permissive default implementation or persistent store is shipped.
// A resolver failure is treated as a denial, not an eventual grant.
type ReviewGrantLookup func(tokenHash [32]byte)(ReviewGrant,error)

var ErrReviewGrant = errors.New("filing review credential not authorized")

// NewScopedReviewAuthorizer enforces record-level identity for browser/API
// callers who supply a dedicated opaque Bearer capability. This is separate
// from FolioRelay's management/admin token and session cookie. It does NOT
// mint grants or map management sessions into broader filing permissions.
//
// Raw token wire format: base64.RawURLEncoding of exactly 32 random bytes.
// This 256-bit token MUST be generated and delivered by a separately
// admitted authentication workflow. It is never returned by the authorizer.
//
// No headers such as X-User, X-Actor, X-Forwarded-User, query tokens, cookies,
// or Paperless API tokens can substitute for that capability. This enforces
// an exact effect binding and expiration; it does not grant DMS write access.
func NewScopedReviewAuthorizer(lookup ReviewGrantLookup,now func()time.Time)(ReviewAuthorizer,error){
 if lookup==nil{return nil,ErrReviewGrant}
 if now==nil {now=time.Now}
 return func(r *http.Request,effectID string)(ReviewPrincipal,error){
  if r==nil||!shaRe.MatchString(effectID){return ReviewPrincipal{},ErrReviewGrant}
  raw:=r.Header.Get("Authorization")
  if len(raw)!=len("Bearer ")+43||!strings.HasPrefix(raw,"Bearer ") {
   return ReviewPrincipal{},ErrReviewGrant
  }
  encoded:=strings.TrimPrefix(raw,"Bearer ")
  secret,err:=base64.RawURLEncoding.DecodeString(encoded)
  if err!=nil||len(secret)!=32||base64.RawURLEncoding.EncodeToString(secret)!=encoded {
   return ReviewPrincipal{},ErrReviewGrant
  }
  hash:=sha256.Sum256(secret)
  grant,err:=lookup(hash)
  if err!=nil||subtle.ConstantTimeCompare(hash[:],grant.TokenHash[:])!=1 ||
   !identRe.MatchString(grant.ActorID)||!shaRe.MatchString(grant.EffectID) ||
   grant.EffectID!=effectID || !grant.CanRead || grant.Revoked ||
   grant.ValidFrom.IsZero() || grant.ExpiresAt.IsZero() ||
   !grant.ExpiresAt.After(grant.ValidFrom) {
    return ReviewPrincipal{},ErrReviewGrant
  }
  at:=now()
  if at.Before(grant.ValidFrom)||!at.Before(grant.ExpiresAt){
   return ReviewPrincipal{},ErrReviewGrant
  }
  return ReviewPrincipal{ActorID:grant.ActorID,CanRead:true},nil
 },nil
}
