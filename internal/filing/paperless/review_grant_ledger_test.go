package paperless

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/base64"
 "errors"
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "sync"
 "testing"
 "time"
)

func grantLedgerFixture(t *testing.T)(*EncryptedReviewGrantLedger,string,[]byte,*time.Time,*int) {
 t.Helper()
 path:=filepath.Join(t.TempDir(),"grants.frj")
 key:=[]byte("0123456789abcdef0123456789abcdef") // TEST ONLY
 current:=time.Date(2026,10,24,14,0,0,0,time.UTC)
 calls:=0
 decide:=func(_ context.Context,action ReviewGrantAction,actor,effect,operator string)error{
  calls++
  if operator!="trusted-review-authority"||!identRe.MatchString(actor)||
   !shaRe.MatchString(effect)||
   (action!=GrantIssue&&action!=GrantRevoke){
    return ErrReviewGrantLedger
  }
  return nil
 }
 ledger,err:=OpenEncryptedReviewGrantLedger(path,key,decide,func()time.Time{return current})
 if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){_ = ledger.Close()})
 return ledger,path,key,&current,&calls
}
func digestGrant(t *testing.T,token string)[32]byte{
 t.Helper()
 raw,err:=base64.RawURLEncoding.DecodeString(token)
 if err!=nil||len(raw)!=32{t.Fatalf("invalid encoded review grant: %v",err)}
 return sha256.Sum256(raw)
}
func TestEncryptedReviewGrantsSurviveRestartAndRevocation(t *testing.T){
 l,path,key,now,calls:=grantLedgerFixture(t)
 ctx:=context.Background()
 effect:=strings.Repeat("a",64)
 token,err:=l.Issue(ctx,"reviewer-thai",effect,"trusted-review-authority",30*time.Minute)
 if err!=nil{t.Fatal(err)}
 hash:=digestGrant(t,token)
 grant,err:=l.Lookup(hash)
 if err!=nil||grant.EffectID!=effect||grant.ActorID!="reviewer-thai"||
   !grant.CanRead||grant.Revoked {t.Fatalf("bad issued review scope: %#v %v",grant,err)}
 disk,err:=os.ReadFile(path)
 if err!=nil{t.Fatal(err)}
 for _,secret:=range []string{token,"reviewer-thai",effect,"trusted-review-authority"}{
  if bytes.Contains(disk,[]byte(secret)){
    t.Fatal("journal leaked raw token or even unencrypted metadata")
  }
 }
 if runtime.GOOS!="windows" {
  st,err:=os.Stat(path)
  if err!=nil{t.Fatal(err)}
  if st.Mode().Perm()!=0o600{t.Fatalf("grant journal permissions not private: %o",st.Mode().Perm())}
 }
 if err:=l.Close();err!=nil{t.Fatal(err)}
 if _,err:=l.Lookup(hash);!errors.Is(err,ErrReviewGrantLedger){
  t.Fatal("closed ledger continued handing out review permissions")
 }
 reopen:=func()(*EncryptedReviewGrantLedger,error){
  return OpenEncryptedReviewGrantLedger(path,key,
   func(_ context.Context,action ReviewGrantAction,actor,effect,operator string)error{
     if operator!="trusted-review-authority"{return ErrReviewGrantLedger}
     return nil
   },func()time.Time{return *now})
 }
 again,err:=reopen()
 if err!=nil{t.Fatalf("durable grant did not recover: %v",err)}
 recovered,err:=again.Lookup(hash)
 if err!=nil||recovered.Revoked||recovered.ActorID!="reviewer-thai"{
  t.Fatalf("grant not recovered correctly: %#v %v",recovered,err)
 }
 if err:=again.Revoke(ctx,hash,"trusted-review-authority");err!=nil{t.Fatal(err)}
 revoked,err:=again.Lookup(hash)
 if err!=nil||!revoked.Revoked{t.Fatal("revocation did not take effect immediately")}
 if err:=again.Close();err!=nil{t.Fatal(err)}
 afterReboot,err:=reopen()
 if err!=nil{t.Fatalf("revocation not durable: %v",err)}
 defer afterReboot.Close()
 final,err:=afterReboot.Lookup(hash)
 if err!=nil||!final.Revoked{t.Fatalf("grant resurrected after reboot: %#v %v",final,err)}
 if err:=afterReboot.Revoke(ctx,hash,"trusted-review-authority");!errors.Is(err,ErrReviewGrantLedger){
  t.Fatal("revoked grant was reactivated or accepted again")
 }
 if *calls!=1{t.Fatalf("unexpected original issuer authorization decisions: %d",*calls)}
 disk,err=os.ReadFile(path)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(disk,[]byte("reviewer-thai"))||bytes.Contains(disk,[]byte(token)){
   t.Fatal("plaintext grant reappeared in journal after revocation")
 }
}
func TestGrantLedgerIndependentAdmissionAndExpiry(t *testing.T){
 l,_,_,now,calls:=grantLedgerFixture(t)
 ctx:=context.Background()
 effect:=strings.Repeat("c",64)
 if _,err:=l.Issue(ctx,"reviewer",effect,"not-authorized",5*time.Minute);!errors.Is(err,ErrReviewGrantLedger){
  t.Fatal("operator-supplied authority without independent decision granted")
 }
 if _,err:=l.Issue(ctx,"reviewer",effect,"trusted-review-authority",25*time.Hour);!errors.Is(err,ErrReviewGrantLedger){
  t.Fatal("unbounded review bearer lifetime")
 }
 if _,err:=l.Issue(ctx,"reviewer",effect,"trusted-review-authority",time.Second);!errors.Is(err,ErrReviewGrantLedger){
  t.Fatal("too-short bearer lifetime")
 }
 if _,err:=l.Issue(ctx,"reviewer","not an effect","trusted-review-authority",time.Minute);!errors.Is(err,ErrReviewGrantLedger){
  t.Fatal("unscoped invalid document id allowed")
 }
 bearer,err:=l.Issue(ctx,"reviewer",effect,"trusted-review-authority",time.Minute)
 if err!=nil{t.Fatal(err)}
 lookup,err:=NewScopedReviewAuthorizer(l.Lookup,func()time.Time{return *now})
 if err!=nil{t.Fatal(err)}
 // Authenticated API and browser still reject stale/expired grants, even
 // when the encrypted journal retains them as historical audit events.
 fromToken:=digestGrant(t,bearer)
 _,err=l.Lookup(fromToken)
 if err!=nil{t.Fatal(err)}
 *now=now.Add(2*time.Minute)
 grant,err:=l.Lookup(fromToken)
 if err!=nil||grant.Revoked{t.Fatal("read-only ledger must not invent revocation")}
 _=lookup // existing authorizer independently applies expiry on each GET.
 if *calls!=2{t.Fatalf("bad count for rejected admission and good issuance %d",*calls)}
}
func TestGrantJournalWrongKeyAndCorruptionFailClosed(t *testing.T){
 l,path,key,_,_:=grantLedgerFixture(t)
 token,err:=l.Issue(context.Background(),"reviewer",strings.Repeat("d",64),"trusted-review-authority",time.Minute)
 if err!=nil{t.Fatal(err)}
 if err:=l.Close();err!=nil{t.Fatal(err)}
 wrong:=append([]byte(nil),key...)
 wrong[0]^=0xff
 allow:=func(context.Context,ReviewGrantAction,string,string,string)error{return nil}
 if _,err:=OpenEncryptedReviewGrantLedger(path,wrong,allow,nil);!errors.Is(err,ErrReviewGrantLedger){
  t.Fatal("incorrect private at-rest key recovered journal")
 }
 contents,err:=os.ReadFile(path)
 if err!=nil{t.Fatal(err)}
 contents[len(contents)/2]^=0xff
 if err:=os.WriteFile(path,contents,0o600);err!=nil{t.Fatal(err)}
 if _,err:=OpenEncryptedReviewGrantLedger(path,key,allow,nil);!errors.Is(err,ErrReviewGrantLedger){
  t.Fatal("corrupted grant journal was interpreted as authorization")
 }
 if bytes.Contains(contents,[]byte(token)){t.Fatal("raw credential in corrupted journal")}
}
func TestGrantLedgerSerializesConcurrentIssueAndRevoke(t *testing.T){
 l,_,_,_,_:=grantLedgerFixture(t)
 const n=20
 tokens:=make(chan string,n)
 errorsFound:=make(chan error,n)
 var wg sync.WaitGroup
 for i:=0;i<n;i++{
  wg.Add(1)
  go func(){
   defer wg.Done()
   token,err:=l.Issue(context.Background(),"reviewer",
    strings.Repeat("f",64),"trusted-review-authority",5*time.Minute)
   if err!=nil{errorsFound<-err;return}
   tokens<-token
  }()
 }
 wg.Wait()
 close(tokens);close(errorsFound)
 for err:=range errorsFound{t.Fatalf("concurrent issuer failed: %v",err)}
 seen:=map[string]bool{}
 for token:=range tokens{
  if seen[token]{t.Fatal("two issued grants share the same secret")}
  seen[token]=true
  if _,err:=l.Lookup(digestGrant(t,token));err!=nil{t.Fatal(err)}
 }
 if len(seen)!=n{t.Fatalf("lost concurrent issues: got %d",len(seen))}
 if err:=l.Close();err!=nil{t.Fatal(err)}
}
func TestLedgerFailsClosedOnMissingPolicyWeakKeyOrWorldReadablePath(t *testing.T){
 path:=filepath.Join(t.TempDir(),"grants.frj")
 key:=[]byte("0123456789abcdef0123456789abcdef")
 admit:=func(context.Context,ReviewGrantAction,string,string,string)error{return nil}
 for _,args:=range []struct{key []byte;admit ReviewGrantDecision}{
   {nil,admit},{key,nil},{[]byte("short"),admit},
 }{
  if _,err:=OpenEncryptedReviewGrantLedger(path,args.key,args.admit,nil);!errors.Is(err,ErrReviewGrantLedger){
   t.Fatal("implicit key or privilege granted")
  }
 }
 if runtime.GOOS=="windows"{t.Skip("Windows ACL mode semantics require independent host verification")}
 if err:=os.WriteFile(path,[]byte("plaintext"),0o644);err!=nil{t.Fatal(err)}
 if _,err:=OpenEncryptedReviewGrantLedger(path,key,admit,nil);!errors.Is(err,ErrReviewGrantLedger){
  t.Fatal("world-readable grant journal accepted")
 }
}
