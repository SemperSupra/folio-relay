package inbox

import (
    "bytes"
    "context"
    "crypto/sha256"
    "encoding/hex"
    "io"
    "os"
    "path/filepath"
    "sync"
    "testing"
    "time"
)

// The global reservation covers separate server instances and independent
// callers sharing one Go process. A held verified snapshot owns its RAM until
// Close, rather than freeing capacity immediately after SHA-256 acceptance.
func TestSnapshotMemorySlotLivesThroughDownload(t *testing.T) {
    root,_,record,original:=integrityFixture(t)
    first,receipt:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
    if first==nil || receipt.State!=IntegrityAvailable{t.Fatalf("first snapshot: %+v",receipt)}
    defer first.Close()
    const contenders=24
    var wg sync.WaitGroup
    errCh:=make(chan string,contenders)
    for i:=0;i<contenders;i++ {
        wg.Add(1)
        go func(){
            defer wg.Done()
            candidate,denied:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
            if candidate!=nil {_=candidate.Close();errCh<-"over-admitted concurrent memory"}
            if denied.State!=IntegrityPending || denied.ReasonCode!="snapshot_memory_busy" {
                errCh<-"did not return stable busy reason"
            }
        }()
    }
    wg.Wait()
    close(errCh)
    for err:=range errCh{t.Error(err)}
    if got,err:=io.ReadAll(first);err!=nil || !bytes.Equal(got,original){
        t.Fatalf("first download did not preserve verified bytes: %v",err)
    }
    if err:=first.Close();err!=nil{t.Fatal(err)}
    if err:=first.Close();err!=nil{t.Fatal("idempotent close: ",err)}
    second,after:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
    if second==nil || after.State!=IntegrityAvailable{t.Fatalf("slot was not released: %+v",after)}
    if err:=second.Close();err!=nil{t.Fatal(err)}
}

func TestSnapshotMemorySlotReleasedAfterFailure(t *testing.T) {
    root,file,record,original:=integrityFixture(t)
    if err:=os.WriteFile(file,bytes.Repeat([]byte{'X'},len(original)),0o600);err!=nil{t.Fatal(err)}
    damaged,receipt:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
    if damaged!=nil || receipt.State!=IntegrityCorrupt {t.Fatalf("corrupt unexpectedly admitted: %+v",receipt)}
    if err:=os.WriteFile(file,original,0o600);err!=nil{t.Fatal(err)}
    next,ok:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
    if next==nil || ok.State!=IntegrityAvailable{t.Fatalf("failure leaked admission slot: %+v",ok)}
    defer next.Close()
}

func TestSnapshotConcurrentReadAndClose(t *testing.T) {
    root,_,record,_:=integrityFixture(t)
    for i:=0;i<24;i++ {
        snap,receipt:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
        if snap==nil || receipt.State!=IntegrityAvailable{t.Fatalf("snapshot %d: %+v",i,receipt)}
        start:=make(chan struct{})
        var wg sync.WaitGroup
        wg.Add(2)
        go func(){
            defer wg.Done()
            <-start
            p:=make([]byte,8)
            n,err:=snap.Read(p)
            if err!=nil && err!=os.ErrClosed && err!=io.EOF {t.Errorf("unexpected racing read: %d %v",n,err)}
        }()
        go func(){
            defer wg.Done()
            <-start
            if err:=snap.Close();err!=nil{t.Errorf("racing close: %v",err)}
        }()
        close(start)
        wg.Wait()
        if _,err:=snap.Read(make([]byte,1));err!=os.ErrClosed {t.Fatalf("iteration %d: reader accessible after close: %v",i,err)}
        if snap.holdsSlot {t.Fatal("closed snapshot retained process reservation")}
    }
}

// Size growth and source integrity must be checked before admitting a large
// in-memory buffer. This synthetic 8 MiB case is intentionally bounded so
// free-tier CI and cross-platform race runs are not forced into an OOM test.
func TestSnapshotEightMiBBoundedBytes(t *testing.T) {
    const length=8<<20
    original:=bytes.Repeat([]byte("A0B1C2D3"),length/8)
    sum:=sha256.Sum256(original)
    name:=hex.EncodeToString(sum[:])
    root:=t.TempDir()
    dir:=filepath.Join(root,"sha256",name[:2])
    if err:=os.MkdirAll(dir,0o700);err!=nil{t.Fatal(err)}
    p:=filepath.Join(dir,name)
    if err:=os.WriteFile(p,original,0o600);err!=nil{t.Fatal(err)}
    record:=Record{JobID:"job-large-synthetic",State:"accepted",ArtifactSHA256:name,ArtifactBytes:int64(length)}
    snap,receipt:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
    if snap==nil || receipt.State!=IntegrityAvailable {t.Fatalf("8MiB copy rejected: %+v",receipt)}
    defer snap.Close()
    if len(snap.buffer)!=length || snap.Size()!=length {t.Fatalf("unexpected live buffer size %d/%d",len(snap.buffer),snap.Size())}
    if err:=os.WriteFile(p,bytes.Repeat([]byte{'X'},length),0o600);err!=nil{t.Fatal(err)}
    result,err:=io.ReadAll(snap)
    if err!=nil || !bytes.Equal(result,original){t.Fatalf("post-verification source rewrite changed admitted content: %v",err)}
}
