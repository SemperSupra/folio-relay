package control

import (
    "bytes"
    "context"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "io"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"

    frinbox "github.com/SemperSupra/folio-relay/internal/inbox"
)

func previewFixture(t *testing.T)(string,string,frinbox.Record,[]byte) {
    t.Helper()
    root:=t.TempDir()
    original:=[]byte("%PDF-1.7\nFolioRelay SYNTHETIC preview payload\n%%EOF\n")
    sum:=sha256.Sum256(original)
    hash:=hex.EncodeToString(sum[:])
    name:=filepath.Join(root,"sha256",hash[:2],hash)
    if err:=os.MkdirAll(filepath.Dir(name),0700);err!=nil {t.Fatal(err)}
    if err:=os.WriteFile(name,original,0600);err!=nil {t.Fatal(err)}
    return root,name,frinbox.Record{
        JobID:"job-"+strings.Repeat("a",64),State:"accepted",
        MediaType:"application/pdf",ArtifactSHA256:hash,
        ArtifactBytes:int64(len(original)),DownloadName:"synthetic.pdf",
    },original
}

func previewRequest(t *testing.T,h http.Handler,jobID string,headers map[string]string) *httptest.ResponseRecorder {
    t.Helper()
    req:=httptest.NewRequest(http.MethodGet,"/api/v1/jobs/"+jobID+"/preview",nil)
    for k,v:=range headers {req.Header.Set(k,v)}
    rec:=httptest.NewRecorder()
    h.ServeHTTP(rec,req)
    return rec
}

func TestScopedPreviewRequiresIndependentTrustProviders(t *testing.T){
    root,_,record,_:=previewFixture(t)
    actor:=func(*http.Request)(PreviewPrincipal,error){return PreviewPrincipal{Issuer:"synthetic-idp",Subject:"alice"},nil}
    lookup:=func(_ context.Context,id string)(frinbox.Record,bool,error){return record,true,nil}
    acl:=func(context.Context,PreviewPrincipal,frinbox.Record)error{return nil}
    stop:=func(context.Context,frinbox.Record)(bool,error){return false,nil}
    cases:=[]struct{name string;root string;p PreviewPrincipalResolver;l PreviewRecordLookup;a PreviewReadAuthorization;e PreviewEmergencyStop}{
        {"missing-root","",actor,lookup,acl,stop},
        {"missing-principal",root,nil,lookup,acl,stop},
        {"missing-durable-lookup",root,actor,nil,acl,stop},
        {"missing-current-record-acl",root,actor,lookup,nil,stop},
        {"missing-independent-stop",root,actor,lookup,acl,nil},
    }
    for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
        h,err:=NewPDFPreviewHandler(tc.root,tc.p,tc.l,tc.a,tc.e)
        if h!=nil || err==nil{t.Fatal("preview handler admitted without all required authorities")}
    })}
}

func TestScopedPreviewVerifiedBytesAndFailClosedConsumption(t *testing.T){
    root,file,record,original:=previewFixture(t)
    actorID:="alice"
    allowed:=true
    emergencyBlocked:=false
    providerOutage:=false
    aclCalls:=0
    stopCalls:=0
    actor:=func(*http.Request)(PreviewPrincipal,error){
        if actorID==""{return PreviewPrincipal{},errors.New("identity down")}
        return PreviewPrincipal{Issuer:"synthetic-idp",Subject:actorID},nil
    }
    lookup:=func(_ context.Context,id string)(frinbox.Record,bool,error){
        return record,id==record.JobID,nil
    }
    acl:=func(_ context.Context,p PreviewPrincipal,r frinbox.Record)error{
        aclCalls++
        if providerOutage || !allowed || p.Subject!="alice" || p.Issuer!="synthetic-idp" ||
            r.JobID!=record.JobID {return errors.New("deny current actor")}
        return nil
    }
    stop:=func(context.Context,frinbox.Record)(bool,error){
        stopCalls++
        if providerOutage{return false,errors.New("emergency service outage")}
        return emergencyBlocked,nil
    }
    h,err:=NewPDFPreviewHandler(root,actor,lookup,acl,stop)
    if err!=nil{t.Fatal(err)}
    mux:=http.NewServeMux()
    mux.Handle("GET /api/v1/jobs/{job_id}/preview",h)
    read:=func(want int) *httptest.ResponseRecorder{
        t.Helper()
        rec:=previewRequest(t,mux,record.JobID,nil)
        if rec.Code!=want{t.Fatalf("want %d got %d: %s",want,rec.Code,rec.Body.String())}
        if rec.Header().Get("Cache-Control")!="no-store" || rec.Header().Get("X-Content-Type-Options")!="nosniff" {
            t.Fatalf("security headers missing: %#v",rec.Header())
        }
        if want!=http.StatusOK && bytes.Contains(rec.Body.Bytes(),original) {
            t.Fatal("denial leaked canonical PDF bytes")
        }
        return rec
    }
    ok:=read(http.StatusOK)
    if !bytes.Equal(ok.Body.Bytes(),original) ||
        ok.Header().Get("Content-Type")!="application/pdf" ||
        !strings.HasPrefix(ok.Header().Get("Content-Disposition"),"inline;") ||
        aclCalls<3 || stopCalls<3 {
        t.Fatalf("verified preview mismatch or skipped read-time recheck: hdr=%v body=%q calls=%d/%d",ok.Header(),ok.Body.String(),aclCalls,stopCalls)
    }
    req:=httptest.NewRequest(http.MethodGet,"/api/v1/jobs/"+record.JobID+"/preview",nil)
    req.Header.Set("Range","bytes=0-5")
    ranged:=httptest.NewRecorder()
    mux.ServeHTTP(ranged,req)
    if ranged.Code!=http.StatusRequestedRangeNotSatisfiable ||
        ranged.Header().Get("Content-Range")!="" {t.Fatalf("partial response unexpectedly admitted: %d",ranged.Code)}
    bad:=previewRequest(t,mux,"job-"+strings.Repeat("b",64),nil)
    if bad.Code!=http.StatusNotFound{t.Fatalf("unknown record: %d",bad.Code)}
    actorID=""
    read(http.StatusUnauthorized)
    actorID="bob"
    read(http.StatusForbidden)
    actorID="alice"
    allowed=false
    read(http.StatusForbidden)
    allowed=true
    emergencyBlocked=true
    read(http.StatusForbidden)
    emergencyBlocked=false
    providerOutage=true
    read(http.StatusForbidden)
    providerOutage=false

    record.MediaType="image/urf"
    read(http.StatusUnsupportedMediaType)
    record.MediaType="application/pdf"

    corrupted:=bytes.Repeat([]byte("Z"),len(original))
    if err:=os.WriteFile(file,corrupted,0600);err!=nil {t.Fatal(err)}
    read(http.StatusConflict)
    if err:=os.Remove(file);err!=nil {t.Fatal(err)}
    read(http.StatusNotFound)
}

func TestScopedPreviewStopsRevokedDuringVerificationBeforeFirstByte(t *testing.T){
    root,_,record,_:=previewFixture(t)
    aclChecks:=0
    h,err:=NewPDFPreviewHandler(root,
        func(*http.Request)(PreviewPrincipal,error){
            return PreviewPrincipal{Issuer:"synthetic-idp",Subject:"alice"},nil
        },
        func(_ context.Context,id string)(frinbox.Record,bool,error){return record,id==record.JobID,nil},
        func(context.Context,PreviewPrincipal,frinbox.Record)error {
            aclChecks++
            if aclChecks>1{return errors.New("revoked during source verification")}
            return nil
        },
        func(context.Context,frinbox.Record)(bool,error){return false,nil},
    )
    if err!=nil{t.Fatal(err)}
    mux:=http.NewServeMux()
    mux.Handle("GET /api/v1/jobs/{job_id}/preview",h)
    got:=previewRequest(t,mux,record.JobID,nil)
    if got.Code!=http.StatusForbidden || aclChecks!=2 {
        t.Fatalf("late ACL revocation was bypassed %d calls=%d",got.Code,aclChecks)
    }
    body,_:=io.ReadAll(got.Body)
    if bytes.Contains(body,[]byte("%PDF")) || got.Header().Get("Content-Type")=="application/pdf" {
        t.Fatalf("revoked actor saw any PDF bytes/header: %q",body)
    }
}
