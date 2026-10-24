package paperless

import (
    "context"
    "crypto/sha256"
    "encoding/binary"
    "errors"
    "time"
)

// ReviewGrantCheckpoint binds an encrypted journal prefix to an independently
// trusted, monotonic authority. A checkpoint is not an authorization grant.
type ReviewGrantCheckpoint struct {
    Sequence uint64
    Digest [32]byte
}

// ReviewGrantAnchor MUST be implemented outside the grant journal's writable
// filesystem and protection domain. Read returns an authoritative value;
// CompareAndAdvance atomically accepts only the exact expected predecessor
// and durably commits the successor. An in-memory or same-directory adapter
// is a TEST DOUBLE, not a production implementation.
type ReviewGrantAnchor interface {
    Read(context.Context) (ReviewGrantCheckpoint, error)
    CompareAndAdvance(context.Context, ReviewGrantCheckpoint, ReviewGrantCheckpoint) error
}

const grantCheckpointDomain = "foliorelay-paperless-grant-chain/v1"
const grantAnchorTimeout = 5 * time.Second

var ErrReviewGrantAnchor = errors.New("review grant independent checkpoint unavailable or mismatched")

func advanceReviewGrantDigest(previous [32]byte, sequence uint64, ciphertext []byte) [32]byte {
    h := sha256.New()
    h.Write([]byte(grantCheckpointDomain))
    h.Write([]byte{0})
    h.Write(previous[:])
    var count [8]byte
    binary.BigEndian.PutUint64(count[:], sequence)
    h.Write(count[:])
    h.Write(ciphertext)
    var digest [32]byte
    copy(digest[:], h.Sum(nil))
    return digest
}

func (l *EncryptedReviewGrantLedger) currentCheckpoint() ReviewGrantCheckpoint {
    return ReviewGrantCheckpoint{Sequence: l.sequence, Digest: l.digest}
}

// checkAnchor runs only while holding l.mu. A missing, rolled-back, substituted,
// unavailable, or ambiguous independent authority quarantines this instance;
// even read-only reviewer lookups MUST fail before accessing a document.
func (l *EncryptedReviewGrantLedger) checkAnchor(ctx context.Context) error {
    if l.anchor == nil || l.quarantined || ctx == nil || ctx.Err() != nil {
        l.quarantined = true
        return ErrReviewGrantAnchor
    }
    anchored, err := l.anchor.Read(ctx)
    if err != nil || anchored != l.currentCheckpoint() {
        l.quarantined = true
        return ErrReviewGrantAnchor
    }
    return nil
}

func (l *EncryptedReviewGrantLedger) checkAnchorBounded(ctx context.Context) error {
    bounded, cancel := context.WithTimeout(ctx, grantAnchorTimeout)
    defer cancel()
    return l.checkAnchor(bounded)
}

// OpenAnchoredEncryptedReviewGrantLedger is the ONLY candidate entry point
// for a future opt-in deployed review-grant issuer. The existing unanchored
// source-only constructor remains for historical regression fixtures; it is
// NOT an acceptable production registration path.
//
// A clean journal requires an independently provisioned genesis checkpoint
// (sequence zero, zero digest). Existing unanchored history without a durable
// matching independent checkpoint is REFUSED rather than silently adopted.
// The trusted implementation, its key/identity admission, backup and restore
// behavior remain outside this source-only slice and require separate review.
func OpenAnchoredEncryptedReviewGrantLedger(path string, key []byte,
    decide ReviewGrantDecision, clock func() time.Time,
    anchor ReviewGrantAnchor) (*EncryptedReviewGrantLedger, error) {
    if anchor == nil {
        return nil, ErrReviewGrantAnchor
    }
    ledger, err := OpenEncryptedReviewGrantLedger(path, key, decide, clock)
    if err != nil {
        return nil, err
    }
    ledger.mu.Lock()
    ledger.anchor = anchor
    err = ledger.checkAnchorBounded(context.Background())
    ledger.mu.Unlock()
    if err != nil {
        _ = ledger.Close()
        return nil, ErrReviewGrantAnchor
    }
    return ledger, nil
}
