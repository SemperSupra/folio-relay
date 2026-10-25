package inbox

import (
 "bytes"
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

func TestVerifiedSnapshotPinsOriginalBytesAfterSourceMutation(t *testing.T) {
 root,file,record,original:=integrityFixture(t)
 snapshot,receipt:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
 if receipt.State!=IntegrityAvailable || snapshot==nil || snapshot.Size()!=int64(len(original)){
  t.Fatalf("snapshot not admitted: %+v",receipt)
 }
 privateDir:=snapshot.privateDir
 st,err:=os.Stat(filepath.Join(privateDir,"verified"))
 if err!=nil || !st.Mode().IsRegular(){t.Fatalf("private snapshot missing: %v",err)}
 if err:=os.WriteFile(file,bytes.Repeat([]byte{'X'},len(original)),0o600);err!=nil{t.Fatal(err)}
 if err:=os.Remove(file);err!=nil{t.Fatal(err)}
 result,err:=io.ReadAll(snapshot)
 if err!=nil || !bytes.Equal(result,original){t.Fatalf("source mutation changed verified snapshot: %q / %v",result,err)}
 if err:=snapshot.Close();err!=nil{t.Fatal(err)}
 if _,err:=os.Stat(privateDir);!os.IsNotExist(err){t.Fatalf("private scratch survives close: %v",err)}
 if _,err:=snapshot.Read(make([]byte,1));!os.IsNotExist(err)&&err!=os.ErrClosed{
  t.Fatalf("closed snapshot unexpectedly read: %v",err)
 }
}

func TestVerifiedSnapshotFailClosedBeforeAnyBodyBytes(t *testing.T) {
 root,file,record,payload:=integrityFixture(t)
 now:=time.Now()
 corrupt:=append([]byte(nil),payload...)
 corrupt[3]^=1
 if err:=os.WriteFile(file,corrupt,0o600);err!=nil{t.Fatal(err)}
 snapshot,receipt:=SnapshotVerifiedArtifact(context.Background(),root,record,now)
 if snapshot!=nil || receipt.State!=IntegrityCorrupt || receipt.ReasonCode!="canonical_hash_mismatch" {
  t.Fatalf("corrupt payload produced readable snapshot: snapshot=%v receipt=%+v",snapshot,receipt)
 }
 if err:=os.WriteFile(file,payload[:len(payload)-1],0o600);err!=nil{t.Fatal(err)}
 snapshot,receipt=SnapshotVerifiedArtifact(context.Background(),root,record,now)
 if snapshot!=nil || receipt.State!=IntegrityCorrupt || receipt.ReasonCode!="canonical_size_mismatch" {
  t.Fatalf("truncated bytes exposed: %+v",receipt)
 }
 if err:=os.Remove(file);err!=nil{t.Fatal(err)}
 snapshot,receipt=SnapshotVerifiedArtifact(context.Background(),root,record,now)
 if snapshot!=nil || receipt.State!=IntegrityMissing || receipt.ReasonCode!="canonical_artifact_missing"{
  t.Fatalf("deleted artifact exposed: %+v",receipt)
 }
 if strings.Contains(receipt.ReasonCode,root){t.Fatal("receipt leaked host path")}
}

func TestVerifiedSnapshotDeniedTraversalSymlinkAndUnboundedReference(t *testing.T){
 root,file,record,_:=integrityFixture(t)
 if err:=os.Remove(file);err!=nil{t.Fatal(err)}
 outside:=filepath.Join(t.TempDir(),"external.pdf")
 if err:=os.WriteFile(outside,[]byte("outside"),0o600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(outside,file);err==nil{
  s,r:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
  if s!=nil || r.State!=IntegrityCorrupt || r.ReasonCode!="canonical_not_regular"{
   t.Fatalf("symlink followed out of root: %+v",r)
  }
 }else{t.Logf("symlink unavailable on host: %v",err)}
 for _,bad:=range []string{"../../etc/passwd",strings.Repeat("A",64),strings.Repeat("a",63)+"z",""} {
  copy:=record
  copy.ArtifactSHA256=bad
  s,r:=SnapshotVerifiedArtifact(context.Background(),root,copy,time.Now())
  if s!=nil||r.ReasonCode!="invalid_canonical_reference"{t.Fatalf("invalid digest %q admitted: %+v",bad,r)}
 }
 record.ArtifactBytes=100<<20+1
 s,r:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
 if s!=nil||r.State!=IntegrityCorrupt{t.Fatalf("oversized record admitted: %+v",r)}
}

func TestVerifiedSnapshotCancellationAndUnavailableStore(t *testing.T){
 root,_,record,_:=integrityFixture(t)
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 s,r:=SnapshotVerifiedArtifact(ctx,root,record,time.Now())
 if s!=nil||r.State!=IntegrityPending||r.ReasonCode!="verification_cancelled"{
  t.Fatalf("cancelled snapshot incorrectly admitted: %+v",r)
 }
 s,r=SnapshotVerifiedArtifact(context.Background(),filepath.Join(root,"offline"),record,time.Now())
 if s!=nil||r.State!=IntegrityPending||r.ReasonCode!="canonical_root_unavailable"{
  t.Fatalf("unavailable root reported as corrupt/published: %+v",r)
 }
}

func TestVerifiedSnapshotNeverUsesOrphanContents(t *testing.T){
 root,file,record,original:=integrityFixture(t)
 // Equal-length substitute with different bytes must not be accepted merely
 // because a path named by the accepted artifact hash exists.
 fake:=bytes.Repeat([]byte("Q"),len(original))
 digest:=sha256.Sum256(fake)
 if hex.EncodeToString(digest[:])==record.ArtifactSHA256{t.Fatal("invalid fixture")}
 if err:=os.WriteFile(file,fake,0o600);err!=nil{t.Fatal(err)}
 s,r:=SnapshotVerifiedArtifact(context.Background(),root,record,time.Now())
 if s!=nil||r.State!=IntegrityCorrupt||r.ReasonCode!="canonical_hash_mismatch" {
  t.Fatalf("wrong same-size artifact was published: %+v",r)
 }
 if _,err:=os.Stat(file);err!=nil{t.Fatal("verification modified source")}
}
