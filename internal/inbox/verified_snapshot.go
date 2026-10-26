package inbox

import (
    "bytes"
    "context"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "io"
    "os"
    "path"
    "sync"
    "time"
)

// VerifiedSnapshot owns a bounded in-process copy of an accepted artifact.
// There is no scratch pathname to swap or rewrite after the digest passes.
// The caller must Close the snapshot after HTTP delivery or cancellation.
// snapshotMemorySlot reserves at most one 100 MiB-capable buffer per process,
// across ALL independent control Server instances and direct Go callers.
// A separate cgroup/host limit is still needed across multiple processes.
var snapshotMemorySlot = make(chan struct{}, 1)

type VerifiedSnapshot struct {
    mu sync.Mutex
    reader *bytes.Reader
    buffer []byte
    bytes int64
    holdsSlot bool
}

func (s *VerifiedSnapshot) Read(p []byte) (int,error) {
    if s==nil { return 0,os.ErrClosed }
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.reader==nil {return 0,os.ErrClosed}
    return s.reader.Read(p)
}
func (s *VerifiedSnapshot) Size() int64 {
    if s==nil {return 0}
    return s.bytes // immutable after allocation
}
func (s *VerifiedSnapshot) Close() error {
    if s==nil {return nil}
    s.mu.Lock()
    defer s.mu.Unlock()
    // Serialize with Read, scrub bytes, and return the process-wide capacity.
    // Idempotent Close must never release a second caller's reservation.
    clear(s.buffer)
    s.buffer=nil
    s.reader=nil
    if s.holdsSlot {
        s.holdsSlot=false
        <-snapshotMemorySlot
    }
    return nil
}

// snapshotReader checks cancellation between bounded source reads. No client
// receives a byte until the full privately owned buffer is validated.
type snapshotReader struct {
    ctx context.Context
    src io.Reader
}
func (r snapshotReader) Read(p []byte)(int,error) {
    if err:=r.ctx.Err();err!=nil{return 0,err}
    return r.src.Read(p)
}

// SnapshotVerifiedArtifact returns no readable bytes until the complete
// bounded in-memory copy equals the durable accepted SHA-256 and length.
// A full-privilege same-UID filesystem actor cannot mutate the verified copy
// by editing or replacing a scratch pathname: no scratch file is created.
// The HTTP handler bounds one verifier per Server; this primitive also
// reserves one live snapshot per PROCESS (including independent Server objects).
// Cross-process cgroup memory limits remain a separate target gate.
func SnapshotVerifiedArtifact(ctx context.Context,rootDir string,record Record,checkedAt time.Time)(*VerifiedSnapshot,IntegrityReceipt){
    receipt:=IntegrityReceipt{
        JobID:record.JobID,State:IntegrityPending,
        CheckedAt:checkedAt.UTC(),ReasonCode:"not_verified",
    }
    if ctx==nil || checkedAt.IsZero() || rootDir=="" || record.JobID=="" {
        receipt.ReasonCode="invalid_verification_context"
        return nil,receipt
    }
    if record.State!="accepted" || !validCanonicalHash(record.ArtifactSHA256) ||
        record.ArtifactBytes<0 || record.ArtifactBytes>maxCanonicalVerifyBytes {
        receipt.State=IntegrityCorrupt
        receipt.ReasonCode="invalid_canonical_reference"
        return nil,receipt
    }
    if ctx.Err()!=nil {receipt.ReasonCode="verification_cancelled";return nil,receipt}
    root,err:=os.OpenRoot(rootDir)
    if err!=nil {receipt.ReasonCode="canonical_root_unavailable";return nil,receipt}
    defer root.Close()
    name:=path.Join("sha256",record.ArtifactSHA256[:2],record.ArtifactSHA256)
    info,err:=root.Lstat(name)
    if errors.Is(err,os.ErrNotExist) {
        receipt.State=IntegrityMissing
        receipt.ReasonCode="canonical_artifact_missing"
        return nil,receipt
    }
    if err!=nil {receipt.ReasonCode="canonical_artifact_unreadable";return nil,receipt}
    if !info.Mode().IsRegular() {
        receipt.State=IntegrityCorrupt
        receipt.ReasonCode="canonical_not_regular"
        return nil,receipt
    }
    if info.Size()!=record.ArtifactBytes {
        receipt.State=IntegrityCorrupt
        receipt.ReasonCode="canonical_size_mismatch"
        return nil,receipt
    }
    source,err:=root.Open(name)
    if errors.Is(err,os.ErrNotExist) {
        receipt.State=IntegrityMissing
        receipt.ReasonCode="canonical_artifact_missing"
        return nil,receipt
    }
    if err!=nil {receipt.ReasonCode="canonical_artifact_unreadable";return nil,receipt}
    defer source.Close()
    opened,err:=source.Stat()
    if err!=nil {receipt.ReasonCode="canonical_artifact_unreadable";return nil,receipt}
    if !opened.Mode().IsRegular() || !os.SameFile(info,opened) {
        receipt.State=IntegrityCorrupt
        receipt.ReasonCode="canonical_file_changed_during_open"
        return nil,receipt
    }

    // Reserve before allocating. The slot is held until Close, not merely
    // through SHA verification, because the HTTP response still owns RAM.
    // Default-deny when another source/client already holds a snapshot.
    select {
    case snapshotMemorySlot<-struct{}{}:
    default:
        receipt.ReasonCode="snapshot_memory_busy"
        return nil,receipt
    }
    // The accepted length has already passed the 100 MiB bound. Using one
    // exact-size allocation avoids unbounded buffer growth or disk spill.
    data:=make([]byte,int(record.ArtifactBytes))
    committed:=false
    defer func(){if !committed {clear(data);<-snapshotMemorySlot}}()
    reader:=snapshotReader{ctx:ctx,src:source}
    n,err:=io.ReadFull(reader,data)
    if ctx.Err()!=nil {receipt.ReasonCode="verification_cancelled";return nil,receipt}
    if err==io.EOF || err==io.ErrUnexpectedEOF || int64(n)!=record.ArtifactBytes {
        receipt.State=IntegrityCorrupt
        receipt.ReasonCode="canonical_size_mismatch"
        return nil,receipt
    }
    if err!=nil {receipt.ReasonCode="canonical_read_failed";return nil,receipt}
    // Explicit extra-byte read is required even when expected length is 0.
    var extra [1]byte
    extraN,extraErr:=reader.Read(extra[:])
    if ctx.Err()!=nil {receipt.ReasonCode="verification_cancelled";return nil,receipt}
    if extraN!=0 {
        receipt.State=IntegrityCorrupt
        receipt.ReasonCode="canonical_size_mismatch"
        return nil,receipt
    }
    if extraErr!=io.EOF {receipt.ReasonCode="canonical_read_failed";return nil,receipt}
    sum:=sha256.Sum256(data)
    if hex.EncodeToString(sum[:])!=record.ArtifactSHA256 {
        receipt.State=IntegrityCorrupt
        receipt.ReasonCode="canonical_hash_mismatch"
        return nil,receipt
    }
    if ctx.Err()!=nil {receipt.ReasonCode="verification_cancelled";return nil,receipt}
    snapshot:=&VerifiedSnapshot{reader:bytes.NewReader(data),buffer:data,bytes:record.ArtifactBytes,holdsSlot:true}
    receipt.State=IntegrityAvailable
    receipt.ReasonCode="canonical_verified"
    committed=true
    return snapshot,receipt
}
