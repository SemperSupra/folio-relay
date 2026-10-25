package inbox

import (
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "io"
    "os"
    "path"
    "strings"
    "time"
)

// Integrity is a read-only reconciliation projection, not a mutation of a
// retained job or its durable canonical-artifact identity. No files are
// created, removed, repaired, renamed or promoted by VerifyCanonicalArtifact.
type Integrity string

const (
    IntegrityAvailable Integrity = "available"
    IntegrityMissing   Integrity = "missing"
    IntegrityCorrupt   Integrity = "corrupt"
    IntegrityPending   Integrity = "verification_pending"
)

// Accepted print admission currently bounds input at 100 MiB. An unbounded
// newly observed file must not expand verifier work just by declaring a size.
const maxCanonicalVerifyBytes int64 = 100 << 20

type IntegrityReceipt struct {
    JobID string `json:"job_id"`
    State Integrity `json:"state"`
    CheckedAt time.Time `json:"checked_at"`
    // ReasonCode is machine-readable and never contains a host filesystem
    // path, raw document bytes, host error string or identity claim.
    ReasonCode string `json:"reason_code"`
}

// VerifyCanonicalArtifact accepts an existing protected content-addressed
// root plus a record from the *durable accepted job projection*. It is not a
// directory importer. Orphan files are never adopted as jobs and remote/SMB
// copy integrity is a separate authority.
func VerifyCanonicalArtifact(rootDir string, record Record, checkedAt time.Time) IntegrityReceipt {
    result := IntegrityReceipt{
        JobID: record.JobID, State: IntegrityPending,
        CheckedAt: checkedAt.UTC(), ReasonCode: "not_verified",
    }
    if checkedAt.IsZero() || rootDir == "" || record.JobID == "" {
        result.ReasonCode="invalid_verification_context"
        return result
    }
    if !validCanonicalHash(record.ArtifactSHA256) ||
        record.ArtifactBytes < 0 || record.ArtifactBytes > maxCanonicalVerifyBytes ||
        record.State != "accepted" {
        result.State=IntegrityCorrupt
        result.ReasonCode="invalid_canonical_reference"
        return result
    }

    root, err:=os.OpenRoot(rootDir)
    if err!=nil {
        result.ReasonCode="canonical_root_unavailable"
        return result
    }
    defer root.Close()

    name:=path.Join("sha256",record.ArtifactSHA256[:2],record.ArtifactSHA256)
    fileInfo, err:=root.Lstat(name)
    if errors.Is(err,os.ErrNotExist) {
        result.State=IntegrityMissing
        result.ReasonCode="canonical_artifact_missing"
        return result
    }
    if err!=nil {
        result.ReasonCode="canonical_artifact_unreadable"
        return result
    }
    if !fileInfo.Mode().IsRegular() {
        result.State=IntegrityCorrupt
        result.ReasonCode="canonical_not_regular"
        return result
    }
    if fileInfo.Size()!=record.ArtifactBytes {
        result.State=IntegrityCorrupt
        result.ReasonCode="canonical_size_mismatch"
        return result
    }
    file,err:=root.Open(name)
    if errors.Is(err,os.ErrNotExist){
        result.State=IntegrityMissing
        result.ReasonCode="canonical_artifact_missing"
        return result
    }
    if err!=nil{
        result.ReasonCode="canonical_artifact_unreadable"
        return result
    }
    defer file.Close()
    opened,err:=file.Stat()
    if err!=nil{
        result.ReasonCode="canonical_artifact_unreadable"
        return result
    }
    if !opened.Mode().IsRegular() || !os.SameFile(fileInfo,opened){
        result.State=IntegrityCorrupt
        result.ReasonCode="canonical_file_changed_during_open"
        return result
    }
    // At most the expected size plus one byte is read; never trust a file
    // size previously obtained by stat as an allocation or hash-work budget.
    hash:=sha256.New()
    count,err:=io.Copy(hash,io.LimitReader(file,record.ArtifactBytes+1))
    if err!=nil{
        result.ReasonCode="canonical_read_failed"
        return result
    }
    if count!=record.ArtifactBytes {
        result.State=IntegrityCorrupt
        result.ReasonCode="canonical_size_mismatch"
        return result
    }
    if hex.EncodeToString(hash.Sum(nil))!=record.ArtifactSHA256{
        result.State=IntegrityCorrupt
        result.ReasonCode="canonical_hash_mismatch"
        return result
    }
    result.State=IntegrityAvailable
    result.ReasonCode="canonical_verified"
    return result
}

func validCanonicalHash(s string) bool {
    if len(s)!=64 || strings.ToLower(s)!=s {return false}
    for _,r:=range s {
        if r<'0'||r>'9'&&r<'a'||r>'f'{return false}
    }
    return true
}
