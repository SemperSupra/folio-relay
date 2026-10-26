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
    "time"
)

// VerifiedSnapshot owns a bounded in-process copy of an accepted artifact.
// There is no scratch pathname to swap or rewrite after the digest passes.
// The caller must Close the snapshot after HTTP delivery or cancellation.
type VerifiedSnapshot struct {
    reader *bytes.Reader
    buffer []byte
    bytes int64
}

func (s *VerifiedSnapshot) Read(p []byte) (int,error) {
    if s==nil || s.reader==nil { return 0,os.ErrClosed }
    return s.reader.Read(p)
}
func (s *VerifiedSnapshot) Size() int64 {
    if s==nil {return 0}
    return s.bytes
}
func (s *VerifiedSnapshot) Close() error {
    if s==nil {return nil}
    // Release the owned content buffer once HTTP delivery is over.
    // No pathname or on-disk scratch file exists, even after a crash.
    clear(s.buffer)
    s.buffer=nil
    s.reader=nil
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
// Only one verifier per control instance is allowed by the HTTP handler.
// The 100 MiB memory bound is still an exact-target capacity/release gate.
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

    // The accepted length has already passed the 100 MiB bound. Using one
    // exact-size allocation avoids unbounded buffer growth or disk spill.
    data:=make([]byte,int(record.ArtifactBytes))
    committed:=false
    defer func(){if !committed {clear(data)}}()
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
    snapshot:=&VerifiedSnapshot{reader:bytes.NewReader(data),buffer:data,bytes:record.ArtifactBytes}
    receipt.State=IntegrityAvailable
    receipt.ReasonCode="canonical_verified"
    committed=true
    return snapshot,receipt
}
