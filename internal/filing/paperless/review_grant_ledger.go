package paperless

import (
 "context"
 "crypto/aes"
 "crypto/cipher"
 "crypto/rand"
 "crypto/sha256"
 "encoding/base64"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "runtime"
 "sync"
 "time"

 "github.com/SemperSupra/folio-relay/internal/state"
)

// No raw grant or Paperless API credential is ever written to the journal.
// This component is NOT mounted in the running FolioRelay daemon.
var ErrReviewGrantLedger = errors.New("review grant journal unavailable or admission denied")
const grantEventAAD = "foliorelay-paperless-grant-journal/v1"
const maxGrantLifetime = time.Hour

type ReviewGrantAction string
const (
 GrantIssue ReviewGrantAction = "issue"
 GrantRevoke ReviewGrantAction = "revoke"
)

// ReviewGrantDecision is a mandatory, independently implemented and
// authenticated policy decision at the time of issuance OR revocation.
// The operator identifier is untrusted input until checked by this policy.
// The issuer must never reuse the global management session as a blanket
// entitlement for access to every record.
type ReviewGrantDecision func(context.Context,ReviewGrantAction,string,string,string)error

type grantEvent struct {
 Version int
 Sequence uint64
 Kind ReviewGrantAction
 Grant ReviewGrant
 Operator string
 When time.Time
}

// EncryptedReviewGrantLedger reuses FolioRelay's checksummed, fsynced,
// single-writer journal. Each event payload is AES-GCM encrypted under
// externally provisioned independent key material. On recovery, events are
// validated in order; a wrong key or altered frame fails closed.
//
// Threat boundary: a privileged filesystem administrator can delete a
// complete final journal frame (e.g. one revocation). A separately anchored
// tamper-resistant audit/checkpoint is still REQUIRED before deployment.
type EncryptedReviewGrantLedger struct {
 mu sync.Mutex
 journal *state.Journal
 aead cipher.AEAD
 grants map[[32]byte]ReviewGrant
 sequence uint64
 decide ReviewGrantDecision
 clock func()time.Time
 closed bool
}

// OpenEncryptedReviewGrantLedger requires an independently provisioned
// 256-bit storage key and an explicit review-grant admission policy. It
// intentionally has NO auto-generated filesystem key, default operator,
// listener, grant endpoint or production registration.
func OpenEncryptedReviewGrantLedger(path string,key []byte,decide ReviewGrantDecision,clock func()time.Time)(*EncryptedReviewGrantLedger,error){
 if path==""||len(key)!=32||decide==nil{
  return nil,ErrReviewGrantLedger
 }
 if clock==nil{clock=time.Now}
 // Fail closed on the most dangerous local configuration mistakes, rather
 // than following a symlink into another actor's state. Windows filesystem
 // ACL qualification remains a separate admission boundary.
 dir:=filepath.Dir(path)
 if info,err:=os.Lstat(path);err==nil {
  if !info.Mode().IsRegular()||runtime.GOOS!="windows"&&info.Mode().Perm()&0o077!=0{
    return nil,ErrReviewGrantLedger
  }
 }else if !errors.Is(err,os.ErrNotExist){return nil,ErrReviewGrantLedger}
 if info,err:=os.Lstat(dir);err==nil{
  if !info.IsDir()||info.Mode()&os.ModeSymlink!=0||
    runtime.GOOS!="windows"&&info.Mode().Perm()&0o077!=0 {
   return nil,fmt.Errorf("%w: parent directory unsafe: %v",ErrReviewGrantLedger,info.Mode())
  }
 }else if !errors.Is(err,os.ErrNotExist){return nil,ErrReviewGrantLedger}
 block,err:=aes.NewCipher(key)
 if err!=nil{return nil,ErrReviewGrantLedger}
 aead,err:=cipher.NewGCM(block)
 if err!=nil{return nil,ErrReviewGrantLedger}
 journal,err:=state.OpenJournal(path)
 if err!=nil{return nil,fmt.Errorf("%w: journal open/lock failed",ErrReviewGrantLedger)}
 ledger:=&EncryptedReviewGrantLedger{journal:journal,aead:aead,
   grants:map[[32]byte]ReviewGrant{},decide:decide,clock:clock}
 records,err:=journal.Recover()
 if err==nil {
  for _,record:=range records{
   if err=ledger.replay(record);err!=nil{break}
  }
 }
 if err!=nil{
  _=journal.Close()
  return nil,fmt.Errorf("%w: invalid or untrusted encrypted event",ErrReviewGrantLedger)
 }
 return ledger,nil
}
func (l *EncryptedReviewGrantLedger) replay(ciphertext []byte)error{
 if len(ciphertext)<=l.aead.NonceSize()+l.aead.Overhead(){return ErrReviewGrantLedger}
 nonce:=ciphertext[:l.aead.NonceSize()]
 encoded:=ciphertext[l.aead.NonceSize():]
 plaintext,err:=l.aead.Open(nil,nonce,encoded,[]byte(grantEventAAD))
 if err!=nil{return ErrReviewGrantLedger}
 var event grantEvent
 if err:=json.Unmarshal(plaintext,&event);err!=nil{return ErrReviewGrantLedger}
 if event.Version!=1||event.Sequence!=l.sequence+1||event.When.IsZero()||
   !identRe.MatchString(event.Operator)||!identRe.MatchString(event.Grant.ActorID)||
   !shaRe.MatchString(event.Grant.EffectID)||!event.Grant.CanRead||
   event.Grant.ValidFrom.IsZero()||!event.Grant.ExpiresAt.After(event.Grant.ValidFrom)||
   event.Grant.ExpiresAt.Sub(event.Grant.ValidFrom)>maxGrantLifetime||
   event.Grant.TokenHash==([32]byte{}) {return ErrReviewGrantLedger}
 previous,seen:=l.grants[event.Grant.TokenHash]
 switch event.Kind{
 case GrantIssue:
  if seen||event.Grant.Revoked {return ErrReviewGrantLedger}
  l.grants[event.Grant.TokenHash]=event.Grant
 case GrantRevoke:
  if !seen||previous.Revoked||!event.Grant.Revoked||
    previous.ActorID!=event.Grant.ActorID||
    previous.EffectID!=event.Grant.EffectID||
    previous.ValidFrom!=event.Grant.ValidFrom||
    previous.ExpiresAt!=event.Grant.ExpiresAt {return ErrReviewGrantLedger}
  l.grants[event.Grant.TokenHash]=event.Grant
 default:return ErrReviewGrantLedger
 }
 l.sequence=event.Sequence
 return nil
}
func (l *EncryptedReviewGrantLedger) append(event grantEvent)error{
 data,err:=json.Marshal(event)
 if err!=nil{return ErrReviewGrantLedger}
 nonce:=make([]byte,l.aead.NonceSize())
 if _,err=rand.Read(nonce);err!=nil{return ErrReviewGrantLedger}
 encrypted:=l.aead.Seal(nil,nonce,data,[]byte(grantEventAAD))
 if err=l.journal.Append(append(nonce,encrypted...));err!=nil{
  return fmt.Errorf("%w: commit failed",ErrReviewGrantLedger)
 }
 // All mutations to the in-memory projection happen only after fsync.
 return l.replay(append(nonce,encrypted...))
}

// Issue performs no DMS writes; admission is delegated to a trusted,
// independently authenticated actor-scoped decision. The 256-bit bearer
// is returned ONCE to the direct caller; no database, audit or DLE content
// contains it. The caller owns secure delivery to the intended actor.
func (l *EncryptedReviewGrantLedger) Issue(ctx context.Context,actor,effect,operator string,ttl time.Duration)(string,error){
 if l==nil||ctx==nil||ctx.Err()!=nil||!identRe.MatchString(actor)||
   !shaRe.MatchString(effect)||!identRe.MatchString(operator)||
   ttl<time.Minute||ttl>maxGrantLifetime{return "",ErrReviewGrantLedger}
 l.mu.Lock()
 defer l.mu.Unlock()
 if l.closed{return "",ErrReviewGrantLedger}
 if err:=l.decide(ctx,GrantIssue,actor,effect,operator);err!=nil{return "",ErrReviewGrantLedger}
 if ctx.Err()!=nil{return "",ErrReviewGrantLedger}
 raw:=make([]byte,32)
 if _,err:=rand.Read(raw);err!=nil{return "",ErrReviewGrantLedger}
 hash:=sha256.Sum256(raw)
 if _,exists:=l.grants[hash];exists{return "",ErrReviewGrantLedger}
 now:=l.clock().UTC()
 event:=grantEvent{Version:1,Sequence:l.sequence+1,Kind:GrantIssue,
  Grant:ReviewGrant{TokenHash:hash,ActorID:actor,EffectID:effect,
    ValidFrom:now,ExpiresAt:now.Add(ttl),CanRead:true},
  Operator:operator,When:now}
 if err:=l.append(event);err!=nil{return "",err}
 return base64.RawURLEncoding.EncodeToString(raw),nil
}

// Revoke is an independently admitted action. It is fsynced before the
// lookup can ever return a revoked grant; the same token cannot be
// reactivated by a later issuance or replay.
func (l *EncryptedReviewGrantLedger) Revoke(ctx context.Context,hash [32]byte,operator string)error{
 if l==nil||ctx==nil||ctx.Err()!=nil||!identRe.MatchString(operator){return ErrReviewGrantLedger}
 l.mu.Lock()
 defer l.mu.Unlock()
 if l.closed{return ErrReviewGrantLedger}
 grant,ok:=l.grants[hash]
 if !ok||grant.Revoked{return ErrReviewGrantLedger}
 if err:=l.decide(ctx,GrantRevoke,grant.ActorID,grant.EffectID,operator);err!=nil{return ErrReviewGrantLedger}
 if ctx.Err()!=nil{return ErrReviewGrantLedger}
 grant.Revoked=true
 now:=l.clock().UTC()
 return l.append(grantEvent{Version:1,Sequence:l.sequence+1,
   Kind:GrantRevoke,Grant:grant,Operator:operator,When:now})
}

// Lookup satisfies ReviewGrantLookup, for the existing API/browsers.
// It returns only grant metadata and NEVER the raw secret. The existing
// authorizers independently check read scope, revocation and expiry.
func (l *EncryptedReviewGrantLedger) Lookup(hash [32]byte)(ReviewGrant,error){
 if l==nil{return ReviewGrant{},ErrReviewGrantLedger}
 l.mu.Lock()
 defer l.mu.Unlock()
 if l.closed{return ReviewGrant{},ErrReviewGrantLedger}
 grant,ok:=l.grants[hash]
 if !ok{return ReviewGrant{},ErrReviewGrantLedger}
 return grant,nil
}
func (l *EncryptedReviewGrantLedger) Close()error{
 if l==nil{return nil}
 l.mu.Lock()
 defer l.mu.Unlock()
 if l.closed{return nil}
 l.closed=true
 return l.journal.Close()
}
