package inbox

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "path/filepath"
 "runtime"
 "io"
 "os"
 "path"
 "time"
)

// VerifiedSnapshot is a private, bounded byte snapshot that was completely
// copied and checked against an immutable accepted-job digest BEFORE any
// caller can obtain bytes. It has no external effect or durable job mutation.
// Call Close, including after transport failure or request cancellation.
type VerifiedSnapshot struct {
 file *os.File
 privateDir string
 bytes int64
}

func (s *VerifiedSnapshot) Read(p []byte) (int,error) {
 if s==nil || s.file==nil {return 0,os.ErrClosed}
 return s.file.Read(p)
}
func (s *VerifiedSnapshot) Size() int64 {
 if s==nil{return 0}
 return s.bytes
}
func (s *VerifiedSnapshot) Close() error {
 if s==nil {return nil}
 var err error
 if s.file!=nil {err=s.file.Close();s.file=nil}
 if s.privateDir!="" {
  if cleanupErr:=os.RemoveAll(s.privateDir);err==nil {err=cleanupErr}
  s.privateDir=""
 }
 return err
}

// snapshotReader enforces bounded request cancellation even when a regular
// filesystem Read does not itself consult HTTP context.
type snapshotReader struct {
 ctx context.Context
 src io.Reader
}
func (r snapshotReader) Read(p []byte)(int,error) {
 if err:=r.ctx.Err();err!=nil{return 0,err}
 return r.src.Read(p)
}

// SnapshotVerifiedArtifact never returns a readable snapshot until ALL bytes
// have been checked against the accepted durable SHA-256 and size. The
// snapshot is copied into a fresh 0700 private temporary directory with a
// 0600 file, isolated from the attacker-writable shared export root.
// It is not sufficient to precheck a mutable source then stream that source.
// The temp root must be capacity/ownership qualified on the actual runtime.
func SnapshotVerifiedArtifact(ctx context.Context,rootDir string,record Record,checkedAt time.Time)(*VerifiedSnapshot,IntegrityReceipt){
 receipt:=IntegrityReceipt{JobID:record.JobID,State:IntegrityPending,CheckedAt:checkedAt.UTC(),ReasonCode:"not_verified"}
 if ctx==nil || checkedAt.IsZero() || rootDir=="" || record.JobID=="" {
  receipt.ReasonCode="invalid_verification_context"
  return nil,receipt
 }
 if record.State!="accepted" || !validCanonicalHash(record.ArtifactSHA256) ||
  record.ArtifactBytes<0 || record.ArtifactBytes>maxCanonicalVerifyBytes {
  receipt.State=IntegrityCorrupt;receipt.ReasonCode="invalid_canonical_reference"
  return nil,receipt
 }
 if ctx.Err()!=nil {receipt.ReasonCode="verification_cancelled";return nil,receipt}
 root,err:=os.OpenRoot(rootDir)
 if err!=nil{receipt.ReasonCode="canonical_root_unavailable";return nil,receipt}
 defer root.Close()
 name:=path.Join("sha256",record.ArtifactSHA256[:2],record.ArtifactSHA256)
 info,err:=root.Lstat(name)
 if errors.Is(err,os.ErrNotExist) {
  receipt.State=IntegrityMissing;receipt.ReasonCode="canonical_artifact_missing"
  return nil,receipt
 }
 if err!=nil{receipt.ReasonCode="canonical_artifact_unreadable";return nil,receipt}
 if !info.Mode().IsRegular(){
  receipt.State=IntegrityCorrupt;receipt.ReasonCode="canonical_not_regular"
  return nil,receipt
 }
 if info.Size()!=record.ArtifactBytes {
  receipt.State=IntegrityCorrupt;receipt.ReasonCode="canonical_size_mismatch"
  return nil,receipt
 }
 source,err:=root.Open(name)
 if errors.Is(err,os.ErrNotExist){
  receipt.State=IntegrityMissing;receipt.ReasonCode="canonical_artifact_missing"
  return nil,receipt
 }
 if err!=nil{receipt.ReasonCode="canonical_artifact_unreadable";return nil,receipt}
 defer source.Close()
 opened,err:=source.Stat()
 if err!=nil {receipt.ReasonCode="canonical_artifact_unreadable";return nil,receipt}
 if !opened.Mode().IsRegular() || !os.SameFile(info,opened) {
  receipt.State=IntegrityCorrupt;receipt.ReasonCode="canonical_file_changed_during_open"
  return nil,receipt
 }
 dir,err:=os.MkdirTemp("","foliorelay-private-snapshot-")
 if err!=nil {receipt.ReasonCode="snapshot_storage_unavailable";return nil,receipt}
 snap:=&VerifiedSnapshot{privateDir:dir,bytes:record.ArtifactBytes}
 committed:=false
 defer func(){if !committed {_=snap.Close()}}()
 stat,err:=os.Stat(dir)
 if err!=nil || !stat.IsDir() || (runtime.GOOS!="windows" && stat.Mode().Perm()!=0o700) {
  receipt.ReasonCode="snapshot_storage_untrusted"
  return nil,receipt
 }
 snap.file,err=os.OpenFile(filepath.Join(dir,"verified"),os.O_CREATE|os.O_EXCL|os.O_RDWR,0o600)
 if err!=nil {receipt.ReasonCode="snapshot_storage_unavailable";return nil,receipt}
 h:=sha256.New()
 // Hash exactly the bytes copied to the isolated private snapshot, not a
 // second independent read of the live mutable canonical source.
 n,err:=io.Copy(io.MultiWriter(snap.file,h),
  io.LimitReader(snapshotReader{ctx:ctx,src:source},record.ArtifactBytes+1))
 if err!=nil {
  if ctx.Err()!=nil {receipt.ReasonCode="verification_cancelled"} else {receipt.ReasonCode="canonical_read_failed"}
  return nil,receipt
 }
 if n!=record.ArtifactBytes {
  receipt.State=IntegrityCorrupt;receipt.ReasonCode="canonical_size_mismatch"
  return nil,receipt
 }
 if hex.EncodeToString(h.Sum(nil))!=record.ArtifactSHA256 {
  receipt.State=IntegrityCorrupt;receipt.ReasonCode="canonical_hash_mismatch"
  return nil,receipt
 }
 if err=snap.file.Sync();err!=nil {
  receipt.ReasonCode="snapshot_storage_unavailable";return nil,receipt
 }
 if _,err=snap.file.Seek(0,io.SeekStart);err!=nil {
  receipt.ReasonCode="snapshot_storage_unavailable";return nil,receipt
 }
 if err=ctx.Err();err!=nil {receipt.ReasonCode="verification_cancelled";return nil,receipt}
 receipt.State=IntegrityAvailable;receipt.ReasonCode="canonical_verified"
 committed=true
 return snap,receipt
}

