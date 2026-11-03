package control

// Source-only synthetic cancellation/stream-budget falsifications for #96.
// No new runtime handler, IdP, lease issuer, site identity, or deployment.
import (
  "bytes"
  "context"
  "errors"
  "io"
  "testing"
)

type cancellingAfterWrite struct {
  writes int
  bytes int
  cancel context.CancelFunc
}
func (w *cancellingAfterWrite) Write(p []byte)(int,error) {
  w.writes++
  w.bytes+=len(p)
  // Cancellation delivered while the first Write is in progress. The
  // first chunk is irrevocable; the next guarded write must not occur.
  if w.writes==1 { w.cancel() }
  return len(p),nil
}
type syntheticShortWriter struct { writes int; n int }
func (w *syntheticShortWriter) Write(p []byte)(int,error) {
  w.writes++
  if len(p)==0 {return 0,nil}
  w.n=len(p)-1
  return w.n,nil
}
type syntheticZeroProgressReader struct { calls int }
func (r *syntheticZeroProgressReader) Read([]byte)(int,error) {r.calls++; return 0,nil}

func TestPreviewStreamCancellationDuringFirstWriteStopsNextChunk(t *testing.T) {
  ctx,cancel:=context.WithCancel(context.Background())
  defer cancel()
  writer:=&cancellingAfterWrite{cancel:cancel}
  aclCalls:=0
  allowed:=func()bool {aclCalls++;return true}
  payload:=bytes.Repeat([]byte("S"), 5*previewDeliveryChunkBytes+73)
  err:=copyPreviewWithCurrentAccess(ctx,writer,bytes.NewReader(payload),allowed)
  if !errors.Is(err,context.Canceled) ||
      writer.writes!=1 || writer.bytes!=previewDeliveryChunkBytes ||
      aclCalls!=1 {
    t.Fatalf("cancelled stream continued: err=%v writes=%d bytes=%d acl=%d",
       err,writer.writes,writer.bytes,aclCalls)
  }
  // No claim that bytes already returned from Write could be retracted.
}
func TestPreviewStreamShortWriteNeverRetriesUnverifiedSuffix(t *testing.T) {
  writer:=&syntheticShortWriter{}
  calls:=0
  err:=copyPreviewWithCurrentAccess(context.Background(),writer,
    bytes.NewReader(bytes.Repeat([]byte("R"),3*previewDeliveryChunkBytes)),
    func()bool {calls++;return true})
  if !errors.Is(err,io.ErrShortWrite) || writer.writes!=1 ||
    writer.n!=previewDeliveryChunkBytes-1 || calls!=1 {
    t.Fatalf("short-write leak/retry: err=%v calls=%d writes=%d n=%d",
      err,calls,writer.writes,writer.n)
  }
}
func TestPreviewStreamNoProgressReaderFailsBounded(t *testing.T) {
  reader:=&syntheticZeroProgressReader{}
  writer:=&bytes.Buffer{}
  calls:=0
  err:=copyPreviewWithCurrentAccess(context.Background(),writer,reader,
    func()bool {calls++;return true})
  if !errors.Is(err,io.ErrNoProgress) || reader.calls!=1 ||
    writer.Len()!=0 || calls!=0 {
    t.Fatalf("zero-progress was not bounded: err=%v reads=%d acl=%d written=%d",
      err,reader.calls,calls,writer.Len())
  }
}
func TestPreviewStreamExpiredContextHasNoSourceOrGuardActivity(t *testing.T) {
  ctx,cancel:=context.WithCancel(context.Background())
  cancel()
  reader:=&syntheticZeroProgressReader{}
  allowed:=0
  err:=copyPreviewWithCurrentAccess(ctx,io.Discard,reader,
    func()bool {allowed++; return true})
  if !errors.Is(err,context.Canceled) || reader.calls!=0 || allowed!=0 {
    t.Fatalf("cancelled context unexpectedly read or authorized: %v %d %d",
      err,reader.calls,allowed)
  }
}
