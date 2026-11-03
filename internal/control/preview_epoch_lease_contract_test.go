package control

// Offline, synthetic-only design oracle for #96. This file intentionally adds
// NO product trust provider, handler registration, or accepted identity path.
// A real provider must prove authenticated issuer/subject, policy authority,
// bounded propagation, epoch/expiry semantics and outage behavior independently.
import (
    "bytes"
    "context"
    "net/http"
    "sync"
    "sync/atomic"
    "testing"
    "time"
)

type syntheticEpochLease struct {
    issuer, subject, record string
    generation uint64
    expires time.Time
}
type syntheticEpochState struct {
    generation uint64
    expires time.Time
    providerHealthy bool
    stopped bool
}
type syntheticEpochCache struct {
    state atomic.Pointer[syntheticEpochState]
    checks atomic.Int64
}
func (c *syntheticEpochCache) allowed(ctx context.Context, lease syntheticEpochLease, p PreviewPrincipal, record string, now time.Time) bool {
    c.checks.Add(1)
    if ctx.Err()!=nil || lease.issuer=="" || lease.subject=="" || lease.record=="" ||
        p.Issuer=="" || p.Subject=="" || p.Issuer=="management" || p.Issuer=="shared-management" ||
        lease.issuer!=p.Issuer || lease.subject!=p.Subject || lease.record!=record ||
        lease.generation==0 || !now.Before(lease.expires) {
        return false
    }
    s:=c.state.Load()
    return s!=nil && s.providerHealthy && !s.stopped &&
        s.generation==lease.generation && now.Before(s.expires)
}
func TestPreviewSyntheticEpochLeaseFailClosedMatrix(t *testing.T) {
    now:=time.Unix(2000000000,0)
    good:=syntheticEpochLease{"test-issuer","alice","job-test",9,now.Add(time.Second)}
    actor:=PreviewPrincipal{Issuer:"test-issuer",Subject:"alice"}
    cache:=&syntheticEpochCache{}
    set:=func(g uint64, healthy, stopped bool, expiry time.Time) {
        cache.state.Store(&syntheticEpochState{g,expiry,healthy,stopped})
    }
    allowed:=func(lease syntheticEpochLease, principal PreviewPrincipal, job string, when time.Time) bool {
        return cache.allowed(context.Background(),lease,principal,job,when)
    }
    if allowed(good,actor,"job-test",now) { t.Fatal("missing authoritative state admitted") }
    set(9,true,false,now.Add(time.Second))
    if !allowed(good,actor,"job-test",now) { t.Fatal("valid synthetic local lease rejected") }
    for _,tc:=range []struct{name string; lease syntheticEpochLease; actor PreviewPrincipal; job string; at time.Time}{
        {"wrong-issuer",good,PreviewPrincipal{Issuer:"other",Subject:"alice"},"job-test",now},
        {"wrong-subject",good,PreviewPrincipal{Issuer:"test-issuer",Subject:"bob"},"job-test",now},
        {"wrong-record",good,actor,"other",now},
        {"expired-lease",good,actor,"job-test",now.Add(time.Second)},
        {"missing-issuer",syntheticEpochLease{subject:"alice",record:"job-test",generation:9,expires:now.Add(time.Second)},actor,"job-test",now},
        {"shared-management",good,PreviewPrincipal{Issuer:"management",Subject:"alice"},"job-test",now},
        {"zero-generation",syntheticEpochLease{issuer:"test-issuer",subject:"alice",record:"job-test",expires:now.Add(time.Second)},actor,"job-test",now},
    } {
        t.Run(tc.name,func(t *testing.T){if allowed(tc.lease,tc.actor,tc.job,tc.at){t.Fatal("invalid lease admitted")}})
    }
    set(10,true,false,now.Add(time.Second))
    if allowed(good,actor,"job-test",now) {t.Fatal("stale epoch admitted after generation change")}
    set(9,true,true,now.Add(time.Second))
    if allowed(good,actor,"job-test",now) {t.Fatal("global emergency stop ignored")}
    set(9,false,false,now.Add(time.Second))
    if allowed(good,actor,"job-test",now) {t.Fatal("provider outage ignored")}
    set(9,true,false,now)
    if allowed(good,actor,"job-test",now) {t.Fatal("current-state expiry ignored")}
    ctx,cancel:=context.WithCancel(context.Background())
    cancel()
    set(9,true,false,now.Add(time.Second))
    if cache.allowed(ctx,good,actor,"job-test",now) {t.Fatal("cancelled reader admitted")}
}
func TestPreviewSyntheticEpochCheapConcurrentChunkRevocation(t *testing.T) {
    now:=time.Now()
    cache:=&syntheticEpochCache{}
    lease:=syntheticEpochLease{"test-issuer","alice","job-test",77,now.Add(time.Minute)}
    actor:=PreviewPrincipal{Issuer:"test-issuer",Subject:"alice"}
    cache.state.Store(&syntheticEpochState{77,now.Add(time.Minute),true,false})
    var readers sync.WaitGroup
    for i:=0;i<16;i++ {
        readers.Add(1)
        go func(){
            defer readers.Done()
            for j:=0;j<128;j++ { _=cache.allowed(context.Background(),lease,actor,"job-test",time.Now()) }
        }()
    }
    readers.Wait()
    // A no-RPC local epoch read can be used by the existing 16-KiB loop
    // without 12,800 remote IdP calls per 100 MiB. NOT a distributed proof.
    writer:=&revokingWriter{headers:make(http.Header)}
    writer.revoke=func(){
        cache.state.Store(&syntheticEpochState{78,now.Add(time.Minute),true,true})
    }
    guard:=func() bool {
        return cache.allowed(context.Background(),lease,actor,"job-test",time.Now())
    }
    source:=bytes.NewReader(bytes.Repeat([]byte("S"),256*1024))
    err:=copyPreviewWithCurrentAccess(context.Background(),writer,source,guard)
    if err==nil || writer.chunks!=1 || writer.body.Len()>previewDeliveryChunkBytes {
        t.Fatalf("epoch revocation must stop after first 16KiB: err=%v chunks=%d bytes=%d",err,writer.chunks,writer.body.Len())
    }
    if guard() { t.Fatal("revoked epoch was subsequently accepted") }
    if cache.checks.Load()<16*128+2 { t.Fatal("local check path was not exercised") }
}
