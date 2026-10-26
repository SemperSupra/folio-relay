package inbox

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "io"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"
)

// A zero-byte accepted artifact still needs the extra-byte EOF check and
// a valid SHA-256; it must never create a scratch file on disk.
func TestVerifiedSnapshotZeroByteAndNoFilesystemScratch(t *testing.T) {
    root:=t.TempDir()
    sum:=sha256.Sum256(nil)
    hash:=hex.EncodeToString(sum[:])
    dir:=filepath.Join(root,"sha256",hash[:2])
    if err:=os.MkdirAll(dir,0o700);err!=nil{t.Fatal(err)}
    file:=filepath.Join(dir,hash)
    if err:=os.WriteFile(file,nil,0o600);err!=nil{t.Fatal(err)}
    record:=Record{JobID:"job-empty",State:"accepted",ArtifactSHA256:hash,ArtifactBytes:0}
    snapshot,receipt:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
    if snapshot==nil || receipt.State!=IntegrityAvailable || snapshot.Size()!=0 {
        t.Fatalf("empty canonical artifact rejected: %+v",receipt)
    }
    var b [1]byte
    if n,err:=snapshot.Read(b[:]);n!=0||err!=io.EOF {t.Fatalf("empty artifact read: %d %v",n,err)}
    if err:=snapshot.Close();err!=nil{t.Fatal(err)}
    if n,err:=snapshot.Read(b[:]);n!=0||err!=os.ErrClosed {t.Fatalf("closed artifact read: %d %v",n,err)}
    entries,err:=os.ReadDir(root)
    if err!=nil{t.Fatal(err)}
    for _,entry:=range entries{
        if strings.Contains(entry.Name(),"snapshot") {
            t.Fatalf("scratch directory persisted: %q",entry.Name())
        }
    }

    // A one-byte source under the hash for an empty file must be rejected
    // before a single byte is exposed; the extra-byte check detects growth.
    if err:=os.WriteFile(file,[]byte{42},0o600);err!=nil{t.Fatal(err)}
    rejected,fault:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
    if rejected!=nil || fault.State!=IntegrityCorrupt || fault.ReasonCode!="canonical_size_mismatch" {
        t.Fatalf("extra byte bypassed length check: %+v",fault)
    }
}
