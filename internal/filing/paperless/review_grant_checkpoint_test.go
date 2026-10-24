package paperless

import (
    "context"
    "crypto/sha256"
    "encoding/binary"
    "encoding/base64"
    "errors"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "testing"
    "time"
)

// This fake exists ONLY in a disposable source test. It is deliberately
// not a production anchor: it shares the test process's trust boundary.
type reviewAnchorFake struct {
    mu sync.Mutex
    current ReviewGrantCheckpoint
    unavailable bool
    refuseAdvance bool
}
func (a *reviewAnchorFake) Read(ctx context.Context) (ReviewGrantCheckpoint, error) {
    a.mu.Lock()
    defer a.mu.Unlock()
    if ctx.Err()!=nil || a.unavailable {return ReviewGrantCheckpoint{},ErrReviewGrantAnchor}
    return a.current,nil
}
func (a *reviewAnchorFake) CompareAndAdvance(ctx context.Context, before, after ReviewGrantCheckpoint) error {
    a.mu.Lock()
    defer a.mu.Unlock()
    if ctx.Err()!=nil||a.unavailable||a.refuseAdvance||
       a.current!=before||after.Sequence!=before.Sequence+1||after.Digest==before.Digest {
        return ErrReviewGrantAnchor
    }
    a.current=after
    return nil
}
func anchoredReviewFixture(t *testing.T,anchor *reviewAnchorFake)(string,[]byte,ReviewGrantDecision,func()time.Time){
    t.Helper()
    dir:=filepath.Join(t.TempDir(),"private-anchor")
    if err:=os.Mkdir(dir,0o700);err!=nil{t.Fatal(err)}
    key:=[]byte("0123456789abcdef0123456789abcdef") // test fixture, never live
    decide:=func(_ context.Context,action ReviewGrantAction,actor,effect,operator string)error{
        if operator!="trusted-operator"||actor!="reviewer"||!shaRe.MatchString(effect)||
           (action!=GrantIssue&&action!=GrantRevoke){return ErrReviewGrantLedger}
        return nil
    }
    now:=func()time.Time{return time.Date(2026,10,24,14,0,0,0,time.UTC)}
    return filepath.Join(dir,"grant.frj"),key,decide,now
}
func TestAnchoredGrantIssueRestartAndRevoke(t *testing.T){
    anchor:=&reviewAnchorFake{}
    path,key,decide,now:=anchoredReviewFixture(t,anchor)
    open:=func()(*EncryptedReviewGrantLedger,error){
        return OpenAnchoredEncryptedReviewGrantLedger(path,key,decide,now,anchor)
    }
    l,err:=open()
    if err!=nil{t.Fatal(err)}
    effect:=strings.Repeat("a",64)
    token,err:=l.Issue(context.Background(),"reviewer",effect,"trusted-operator",time.Minute*30)
    if err!=nil{t.Fatal(err)}
    digest:=sha256.Sum256(mustDecodeGrantToken(t,token))
    if cp:=anchor.current;cp.Sequence!=1||cp.Digest==([32]byte{}){
        t.Fatalf("independent source-only checkpoint not committed: %#v",cp)
    }
    if err:=l.Close();err!=nil{t.Fatal(err)}
    again,err:=open()
    if err!=nil{t.Fatal(err)}
    if err:=again.Revoke(context.Background(),digest,"trusted-operator");err!=nil{t.Fatal(err)}
    if cp:=anchor.current;cp.Sequence!=2{t.Fatalf("expected revocation checkpoint 2, got %d",cp.Sequence)}
    got,err:=again.Lookup(digest)
    if err!=nil||!got.Revoked{t.Fatal("revoked grant was not denied",err)}
    if err:=again.Close();err!=nil{t.Fatal(err)}
    final,err:=open()
    if err!=nil{t.Fatal(err)}
    defer final.Close()
    got,err=final.Lookup(digest)
    if err!=nil||!got.Revoked{t.Fatal("durable revocation was not preserved",err)}
}
func mustDecodeGrantToken(t *testing.T,token string)[]byte{
    t.Helper()
    // Base64URL decoding itself is exercised by existing review_grant tests.
    raw,err:=base64.RawURLEncoding.DecodeString(token)
    if err!=nil||len(raw)!=32{t.Fatalf("malformed test token: %v",err)}
    return raw
}
func TestAnchoredCheckpointRejectsRemovedCompleteRevokeFrame(t *testing.T){
    anchor:=&reviewAnchorFake{}
    path,key,decide,now:=anchoredReviewFixture(t,anchor)
    l,err:=OpenAnchoredEncryptedReviewGrantLedger(path,key,decide,now,anchor)
    if err!=nil{t.Fatal(err)}
    token,err:=l.Issue(context.Background(),"reviewer",strings.Repeat("b",64),"trusted-operator",10*time.Minute)
    if err!=nil{t.Fatal(err)}
    hash:=sha256.Sum256(mustDecodeGrantToken(t,token))
    if err:=l.Revoke(context.Background(),hash,"trusted-operator");err!=nil{t.Fatal(err)}
    if err:=l.Close();err!=nil{t.Fatal(err)}
    contents,err:=os.ReadFile(path)
    if err!=nil{t.Fatal(err)}
    // Fully remove the final well-formed frame, not merely corrupt bytes.
    // Existing journal checksum/AES-GCM alone would accept the first frame.
    if len(contents)<8||string(contents[:4])!="FRJ1"{t.Fatal("unexpected journal framing")}
    firstFrame:=8+int(binary.BigEndian.Uint32(contents[4:8]))+sha256.Size
    if firstFrame>=len(contents){t.Fatal("test did not retain an independent revoke frame")}
    if err:=os.WriteFile(path,contents[:firstFrame],0o600);err!=nil{t.Fatal(err)}
    if _,err:=OpenAnchoredEncryptedReviewGrantLedger(path,key,decide,now,anchor);!errors.Is(err,ErrReviewGrantAnchor){
        t.Fatalf("deleted complete revoke was not detected: %v",err)
    }
}
func TestAnchoredGrantQuarantinesOnUnknownCommit(t *testing.T){
    anchor:=&reviewAnchorFake{refuseAdvance:true}
    path,key,decide,now:=anchoredReviewFixture(t,anchor)
    l,err:=OpenAnchoredEncryptedReviewGrantLedger(path,key,decide,now,anchor)
    if err!=nil{t.Fatal(err)}
    token,err:=l.Issue(context.Background(),"reviewer",strings.Repeat("c",64),"trusted-operator",10*time.Minute)
    if token!=""||!errors.Is(err,ErrReviewGrantAnchor){t.Fatalf("unanchored bearer returned: %q %v",token,err)}
    if _,err=l.Lookup([32]byte{});!errors.Is(err,ErrReviewGrantLedger){t.Fatalf("quarantined ledger served a lookup: %v",err)}
    if err:=l.Close();err!=nil{t.Fatal(err)}
    anchor.refuseAdvance=false
    if _,err:=OpenAnchoredEncryptedReviewGrantLedger(path,key,decide,now,anchor);!errors.Is(err,ErrReviewGrantAnchor){
        t.Fatalf("orphaned fsynced event silently adopted on restart: %v",err)
    }
}
func TestAnchoredGrantDeniesWhenAnchorOfflineOrRewritten(t *testing.T){
    anchor:=&reviewAnchorFake{}
    path,key,decide,now:=anchoredReviewFixture(t,anchor)
    l,err:=OpenAnchoredEncryptedReviewGrantLedger(path,key,decide,now,anchor)
    if err!=nil{t.Fatal(err)}
    token,err:=l.Issue(context.Background(),"reviewer",strings.Repeat("d",64),"trusted-operator",time.Minute)
    if err!=nil{t.Fatal(err)}
    hash:=sha256.Sum256(mustDecodeGrantToken(t,token))
    anchor.unavailable=true
    if _,err:=l.Lookup(hash);!errors.Is(err,ErrReviewGrantAnchor){t.Fatalf("offline anchor still authorized read: %v",err)}
    anchor.unavailable=false
    if _,err:=l.Lookup(hash);!errors.Is(err,ErrReviewGrantLedger){t.Fatalf("quarantine silently reset: %v",err)}
    _=l.Close()
    anchor.current.Digest[0]^=0x01
    if _,err:=OpenAnchoredEncryptedReviewGrantLedger(path,key,decide,now,anchor);!errors.Is(err,ErrReviewGrantAnchor){
        t.Fatalf("changed checkpoint still opened: %v",err)
    }
}
func TestAnchorCannotBeOmittedOrBlindlyBootstrapHistory(t *testing.T){
    path,key,decide,now:=anchoredReviewFixture(t,nil)
    if _,err:=OpenAnchoredEncryptedReviewGrantLedger(path,key,decide,now,nil);!errors.Is(err,ErrReviewGrantAnchor){
        t.Fatal("missing independent checkpoint source admitted")
    }
    a:=&reviewAnchorFake{current:ReviewGrantCheckpoint{Sequence:2,Digest:sha256.Sum256([]byte("unrelated"))}}
    if _,err:=OpenAnchoredEncryptedReviewGrantLedger(path,key,decide,now,a);!errors.Is(err,ErrReviewGrantAnchor){
        t.Fatalf("nonmatching provenance adopted as genesis: %v",err)
    }
}
