package control

// Synthetic source-only accounting oracles for an UNREGISTERED read handler.
// A successful ResponseWriter.Write is NOT durable proof of client receipt.
import (
    "bytes"
    "context"
    "errors"
    "io"
    "testing"
)

type scopedCountingWriter struct {
    accepted int
    writeErr error
    calls int
    buf bytes.Buffer
}
func(w *scopedCountingWriter) Write(p []byte)(int,error){
    w.calls++
    n:=w.accepted
    if n<0 { n=len(p) }
    if n>len(p) { return n,w.writeErr }
    if n>0 { _,_=w.buf.Write(p[:n]) }
    return n,w.writeErr
}

func TestScopedTransferCountsOnlyAcknowledgedBytes(t *testing.T){
    payload:=bytes.Repeat([]byte("P"),2*previewDeliveryChunkBytes+71)
    mutator:=func()bool{return true}
    var out bytes.Buffer
    sent,err:=copyPreviewWithCurrentAccessCount(context.Background(),&out,bytes.NewReader(payload),mutator)
    if err!=nil || sent!=int64(len(payload)) || !bytes.Equal(out.Bytes(),payload){
        t.Fatalf("complete source writer mismatch: n=%d error=%v",sent,err)
    }
}

func TestScopedTransferShortWriteNeverRetriesTail(t *testing.T){
    writer:=&scopedCountingWriter{accepted:7}
    source:=bytes.NewReader(bytes.Repeat([]byte("S"),2*previewDeliveryChunkBytes))
    count,err:=copyPreviewWithCurrentAccessCount(context.Background(),writer,source,func()bool{return true})
    if !errors.Is(err,io.ErrShortWrite)||count!=7||writer.calls!=1||writer.buf.Len()!=7{
        t.Fatalf("short write was counted as complete or retried: n=%d error=%v calls=%d size=%d",
            count,err,writer.calls,writer.buf.Len())
    }
}

func TestScopedTransferWriterErrorRetainsPartialCount(t *testing.T){
    failure:=errors.New("synthetic transport interrupted")
    writer:=&scopedCountingWriter{accepted:5,writeErr:failure}
    count,err:=copyPreviewWithCurrentAccessCount(context.Background(),writer,
        bytes.NewReader(bytes.Repeat([]byte("F"),previewDeliveryChunkBytes)),
        func()bool{return true})
    if !errors.Is(err,failure)||count!=5||writer.calls!=1||writer.buf.Len()!=5{
        t.Fatalf("failed writer must report already-accepted bytes: n=%d err=%v",count,err)
    }
}

func TestScopedTransferRevocationPreservesOnlyAdmittedPrefix(t *testing.T){
    payload:=bytes.Repeat([]byte("R"),3*previewDeliveryChunkBytes+10)
    checks:=0
    guard:=func()bool{checks++;return checks==1}
    var out bytes.Buffer
    count,err:=copyPreviewWithCurrentAccessCount(context.Background(),&out,
        bytes.NewReader(payload),guard)
    if err==nil || count!=previewDeliveryChunkBytes ||out.Len()!=previewDeliveryChunkBytes ||
        checks!=2 || !bytes.Equal(out.Bytes(),payload[:previewDeliveryChunkBytes]) {
        t.Fatalf("revoked stream emitted a nonadmitted suffix: bytes=%d checks=%d error=%v",count,checks,err)
    }
}

func TestScopedTransferCanceledBeforeReadReportsZero(t *testing.T){
    ctx,cancel:=context.WithCancel(context.Background())
    cancel()
    calls:=0
    count,err:=copyPreviewWithCurrentAccessCount(ctx,io.Discard,
        bytes.NewReader([]byte("synthetic")),func()bool{calls++;return true})
    if !errors.Is(err,context.Canceled)||count!=0||calls!=0 {
        t.Fatalf("cancelled transfer cannot read or authorize: n=%d calls=%d err=%v",count,calls,err)
    }
}

func TestScopedTransferRejectsImpossibleWriterCount(t *testing.T){
    writer:=&scopedCountingWriter{accepted:previewDeliveryChunkBytes+1}
    count,err:=copyPreviewWithCurrentAccessCount(context.Background(),writer,
        bytes.NewReader(bytes.Repeat([]byte("W"),previewDeliveryChunkBytes)),
        func()bool{return true})
    if !errors.Is(err,io.ErrShortWrite)||count!=0||writer.calls!=1{
        t.Fatalf("invalid writer return must not be counted: n=%d err=%v",count,err)
    }
}
