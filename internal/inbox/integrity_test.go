package inbox

import (
    "bytes"
    "crypto/sha256"
    "encoding/hex"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"
)

func integrityFixture(t *testing.T) (string,string,Record,[]byte) {
    t.Helper()
    root:=t.TempDir()
    payload:=[]byte("%PDF-1.7\nSYNTHETIC DOCUMENT NO PERSON DATA\n")
    sum:=sha256.Sum256(payload)
    digest:=hex.EncodeToString(sum[:])
    full:=filepath.Join(root,"sha256",digest[:2],digest)
    if err:=os.MkdirAll(filepath.Dir(full),0o700);err!=nil{t.Fatal(err)}
    if err:=os.WriteFile(full,payload,0o600);err!=nil{t.Fatal(err)}
    r:=Record{
        JobID:"job-"+strings.Repeat("a",64),
        State:"accepted",ArtifactSHA256:digest,ArtifactBytes:int64(len(payload)),
        MediaType:"application/pdf",
    }
    return root,full,r,payload
}

func TestCanonicalIntegrityPresentMissingAndCorrupt(t *testing.T) {
    root,file,record,payload:=integrityFixture(t)
    now:=time.Date(2026,10,25,3,0,0,0,time.UTC)
    good:=VerifyCanonicalArtifact(root,record,now)
    if good.State!=IntegrityAvailable || good.ReasonCode!="canonical_verified" ||
       good.JobID!=record.JobID || !good.CheckedAt.Equal(now) {
        t.Fatalf("present canonical artifact not verified: %#v",good)
    }
    if err:=os.Remove(file);err!=nil{t.Fatal(err)}
    missing:=VerifyCanonicalArtifact(root,record,now)
    if missing.State!=IntegrityMissing||missing.ReasonCode!="canonical_artifact_missing" {
        t.Fatalf("missing canonical source is not preserved as a recorded concern: %#v",missing)
    }
    altered:=append([]byte(nil),payload...)
    altered[len(altered)-2]^=1
    if err:=os.WriteFile(file,altered,0o600);err!=nil{t.Fatal(err)}
    wrongHash:=VerifyCanonicalArtifact(root,record,now)
    if wrongHash.State!=IntegrityCorrupt||wrongHash.ReasonCode!="canonical_hash_mismatch"{
        t.Fatalf("same-size corrupt source accepted: %#v",wrongHash)
    }
    if err:=os.WriteFile(file,payload[:len(payload)-1],0o600);err!=nil{t.Fatal(err)}
    wrongSize:=VerifyCanonicalArtifact(root,record,now)
    if wrongSize.State!=IntegrityCorrupt||wrongSize.ReasonCode!="canonical_size_mismatch"{
        t.Fatalf("truncated source accepted: %#v",wrongSize)
    }
    // No repair, purge or implicit recreation. The immutable historical job
    // remains retained independently of its current file availability.
    after,err:=os.ReadFile(file)
    if err!=nil||!bytes.Equal(after,payload[:len(payload)-1]){
        t.Fatal("reconciliation must not modify corrupted bytes")
    }
}

func TestCanonicalIntegrityRejectsLinksAndUntrustedReferences(t *testing.T) {
    root,file,record,_:=integrityFixture(t)
    now:=time.Now()
    outside:=filepath.Join(t.TempDir(),"outside")
    if err:=os.WriteFile(outside,[]byte("outside file"),0o600);err!=nil{t.Fatal(err)}
    if err:=os.Remove(file);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(outside,file);err==nil{
        result:=VerifyCanonicalArtifact(root,record,now)
        if result.State!=IntegrityCorrupt||result.ReasonCode!="canonical_not_regular"{
            t.Fatalf("symbolic link was accepted as a canonical source: %#v",result)
        }
    }else{
        t.Logf("symlink test unavailable on host: %v",err)
    }
    for _,c:=range []struct{
        name string
        state Integrity
    }{
        {"../../etc/passwd",IntegrityCorrupt},
        {strings.Repeat("A",64),IntegrityCorrupt},
        {strings.Repeat("a",63)+"z",IntegrityCorrupt},
        {"",IntegrityCorrupt},
    }{
        bad:=record
        bad.ArtifactSHA256=c.name
        result:=VerifyCanonicalArtifact(root,bad,now)
        if result.State!=c.state||result.ReasonCode!="invalid_canonical_reference"{
            t.Fatalf("invalid source hash %q accepted: %#v",c.name,result)
        }
    }
}

func TestCanonicalIntegrityNoFilesystemImportOrImplicitStatusChange(t *testing.T) {
    root,_,record,payload:=integrityFixture(t)
    now:=time.Now()
    // Files with no accepted durable record must not become a job simply
    // because they exist in the canonical or export subtree.
    orphan:=filepath.Join(root,"stray-document.pdf")
    if err:=os.WriteFile(orphan,payload,0o600);err!=nil{t.Fatal(err)}
    result:=VerifyCanonicalArtifact(root,record,now)
    if result.State!=IntegrityAvailable{t.Fatal("unrelated file changed recorded artifact availability")}
    if record.State!="accepted" {t.Fatal("verifier modified durable job state")}
    if _,err:=os.Stat(orphan);err!=nil{t.Fatal("verifier changed orphan file")}
    record.State="unknown"
    result=VerifyCanonicalArtifact(root,record,now)
    if result.State!=IntegrityCorrupt||result.ReasonCode!="invalid_canonical_reference"{
        t.Fatalf("nonaccepted record admitted: %#v",result)
    }
}

func TestCanonicalIntegrityBoundsWorkAndDoesNotMislabelIOFailure(t *testing.T){
    root,_,record,_:=integrityFixture(t)
    now:=time.Now()
    record.ArtifactBytes=maxCanonicalVerifyBytes+1
    tooLarge:=VerifyCanonicalArtifact(root,record,now)
    if tooLarge.State!=IntegrityCorrupt||tooLarge.ReasonCode!="invalid_canonical_reference"{
        t.Fatalf("unbounded input admitted: %#v",tooLarge)
    }
    _,_,record,_=integrityFixture(t)
    missingRoot:=VerifyCanonicalArtifact(filepath.Join(root,"does-not-exist"),record,now)
    if missingRoot.State!=IntegrityPending||missingRoot.ReasonCode!="canonical_root_unavailable"{
        t.Fatalf("I/O absence incorrectly reported as corrupt content: %#v",missingRoot)
    }
    invalidClock:=VerifyCanonicalArtifact(root,record,time.Time{})
    if invalidClock.State!=IntegrityPending||invalidClock.ReasonCode!="invalid_verification_context"{
        t.Fatalf("invalid context incorrectly verified: %#v",invalidClock)
    }
}
