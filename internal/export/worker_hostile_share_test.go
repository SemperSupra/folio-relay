package export

import (
    "bytes"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "os"
    "path/filepath"
    "runtime"
    "strings"
    "sync"
    "testing"
    "time"
)

// A named SMB account is *modeled* as an untrusted concurrent actor which
// can modify only paths under shared/. Same-UID CI cannot establish a real
// POSIX user, SMB ACL, or TrueNAS site-authority boundary.
func TestHostileShareWriterCannotPublishOutsideOrTouchPrivateStage(t *testing.T) {
    if runtime.GOOS == "windows" {
        t.Skip("mode 0660 requires real Windows ACL qualification, not a Unix-mode fake")
    }
    root:=t.TempDir()
    canonical:=filepath.Join(root,"canonical")
    exportRoot:=filepath.Join(root,"export-parent")
    shared:=filepath.Join(exportRoot,"shared")
    outside:=filepath.Join(root,"outside")
    for _,p:=range []string{canonical,shared,outside}{
        if err:=os.MkdirAll(p,0o700);err!=nil{t.Fatal(err)}
    }
    if err:=os.Chmod(shared,0o770);err!=nil{t.Fatal(err)}
    immutable:=append([]byte("%PDF-1.7\n"),bytes.Repeat([]byte("source-only fixture\n"),4096)...)
    sum:=sha256.Sum256(immutable)
    digest:=hex.EncodeToString(sum[:])
    canonicalPath:=filepath.Join(canonical,"sha256",digest[:2],digest)
    if err:=os.MkdirAll(filepath.Dir(canonicalPath),0o700);err!=nil{t.Fatal(err)}
    if err:=os.WriteFile(canonicalPath,immutable,0o600);err!=nil{t.Fatal(err)}
    worker,err:=NewWorkerWithMode(canonical,exportRoot,0o660)
    if err!=nil{t.Fatal(err)}
    defer worker.Close()
    job:=Job{
        JobID:"job-"+strings.Repeat("a",64),
        AcceptedAt:time.Date(2026,10,24,20,0,0,0,time.UTC),
        SourceTitle:"Synthetic Share Race", MediaType:"application/pdf",
    }
    plan,err:=MakePlan(Defaults{Folder:"shared"},Overrides{},job)
    if err!=nil{t.Fatal(err)}
    artifact:=Artifact{SHA256:digest,Bytes:int64(len(immutable))}
    target:=filepath.Join(exportRoot,filepath.FromSlash(plan.RelativePath))
    sentinel:=filepath.Join(outside,"do-not-touch.pdf")
    sentinelBytes:=[]byte("outside sentinel, NOT an export")
    if err:=os.WriteFile(sentinel,sentinelBytes,0o600);err!=nil{t.Fatal(err)}

    // Deterministic negative: a preexisting SMB-controlled symlink at the
    // destination is never followed, deleted or overwritten for publication.
    if err:=os.Symlink(sentinel,target);err!=nil{t.Skipf("symlink fixture unavailable: %v",err)}
    if _,err:=worker.Publish(plan,artifact);!errors.Is(err,ErrCollision){
        t.Fatalf("share-controlled symlink was replaced or followed: %v",err)
    }
    if contents,err:=os.ReadFile(sentinel);err!=nil||!bytes.Equal(contents,sentinelBytes){
        t.Fatalf("source-only worker wrote outside protected export root: %v",err)
    }
    if err:=os.Remove(target);err!=nil{t.Fatal(err)}

    // Concurrent adversary can freely inject/delete output entries while
    // Publish is writing. No assumption about the FINAL user-visible share
    // entry is safe under that race; only canonical/stage/outside invariants.
    start:=make(chan struct{})
    var group sync.WaitGroup
    attackerErrors:=make(chan error,1)
    group.Add(1)
    go func(){
        defer group.Done()
        <-start
        for i:=0;i<120;i++{
            err:=os.Symlink(sentinel,target)
            if err!=nil&&!errors.Is(err,os.ErrExist){
                attackerErrors<-err;return
            }
            if err:=os.Remove(target);err!=nil&&!errors.Is(err,os.ErrNotExist){
                attackerErrors<-err;return
            }
        }
    }()
    close(start)
    _,publishedErr:=worker.Publish(plan,artifact)
    group.Wait()
    close(attackerErrors)
    for e:=range attackerErrors{t.Fatalf("attacker fixture failed: %v",e)}
    // There may be a successful publication, a collision, or an observed
    // unlinked destination. Those are the expected adversarial outcomes.
    // Never infer durable delivery without the separately owned export effect.
    if publishedErr!=nil && !errors.Is(publishedErr,ErrCollision) &&
       !errors.Is(publishedErr,os.ErrNotExist) {
        t.Fatalf("unexpected publication failure under share writer: %v",publishedErr)
    }
    current,err:=os.ReadFile(canonicalPath)
    if err!=nil||!bytes.Equal(current,immutable){
        t.Fatalf("canonical source changed during SMB attack: %v",err)
    }
    unchanged,err:=os.ReadFile(sentinel)
    if err!=nil||!bytes.Equal(unchanged,sentinelBytes){
        t.Fatalf("outside sentinel modified during SMB attack: %v",err)
    }
    private:=filepath.Join(exportRoot,".foliorelay-private-stage")
    stageInfo,err:=os.Lstat(private)
    if err!=nil||!stageInfo.IsDir()||stageInfo.Mode().Perm()!=0o700{
        t.Fatalf("protected staging root permissions changed: %#v %v",stageInfo,err)
    }
    staged,err:=os.ReadDir(private)
    if err!=nil||len(staged)!=0{
        t.Fatalf("temporary source staged outside cleanup or never unlinked: count=%d err=%v",len(staged),err)
    }
}
