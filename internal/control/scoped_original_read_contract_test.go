package control

// SOURCE-ONLY: these handlers are never installed in Server.Handler.
// Synthetic principals and artifacts prove distinct preview/original scopes,
// NOT a real #96 identity provider, distributed STOP lease or site admission.
import (
    "bytes"
    "context"
    "errors"
    "mime"
    "net/http"
    "net/http/httptest"
    "os"
    "strings"
    "testing"

    frinbox "github.com/SemperSupra/folio-relay/internal/inbox"
)

type scopedReadCtor func(string, PreviewPrincipalResolver, PreviewRecordLookup, ScopedArtifactAuthorization, PreviewEmergencyStop) (http.Handler,error)

func TestScopedOriginalAndPreviewRequireEveryIndependentAuthority(t *testing.T) {
    root,_,record,_:=previewFixture(t)
    principal:=func(*http.Request)(PreviewPrincipal,error){return PreviewPrincipal{Issuer:"fixture-oidc",Subject:"alice"},nil}
    lookup:=func(context.Context,string)(frinbox.Record,bool,error){return record,true,nil}
    auth:=func(context.Context,PreviewPrincipal,frinbox.Record,ScopedArtifactRead)error{return nil}
    stop:=func(context.Context,frinbox.Record)(bool,error){return false,nil}
    ctors:=[]struct{name string;fn scopedReadCtor}{
        {"preview",NewScopedPDFPreviewHandler},
        {"original",NewScopedOriginalDownloadHandler},
    }
    for _,ctor:=range ctors {
        t.Run(ctor.name,func(t *testing.T){
            cases:=[]struct{
                name,root string
                p PreviewPrincipalResolver
                l PreviewRecordLookup
                a ScopedArtifactAuthorization
                e PreviewEmergencyStop
            }{
                {"missing-root","",principal,lookup,auth,stop},
                {"missing-principal",root,nil,lookup,auth,stop},
                {"missing-record-lookup",root,principal,nil,auth,stop},
                {"missing-purpose-acl",root,principal,lookup,nil,stop},
                {"missing-stop",root,principal,lookup,auth,nil},
            }
            for _,tc:=range cases {
                t.Run(tc.name,func(t *testing.T){
                    handler,err:=ctor.fn(tc.root,tc.p,tc.l,tc.a,tc.e)
                    if handler!=nil || err==nil {t.Fatal("source-only handler allowed missing authority")}
                })
            }
        })
    }
}

func TestScopedOriginalAndPreviewPurposeIsolation(t *testing.T) {
    root,_,record,canonical:=previewFixture(t)
    allowed:=map[ScopedArtifactRead]bool{ScopedArtifactPreview:true,ScopedArtifactOriginal:false}
    actor:="alice"
    stopped:=false
    outage:=false
    seen:=map[ScopedArtifactRead]int{}
    principal:=func(*http.Request)(PreviewPrincipal,error){
        return PreviewPrincipal{Issuer:"fixture-oidc",Subject:actor},nil
    }
    lookup:=func(_ context.Context,id string)(frinbox.Record,bool,error){
        return record,id==record.JobID,nil
    }
    auth:=func(_ context.Context,p PreviewPrincipal,r frinbox.Record,scope ScopedArtifactRead)error {
        seen[scope]++
        if outage || !allowed[scope] || p.Subject!="alice" || p.Issuer!="fixture-oidc" ||
            r.JobID!=record.JobID {return errors.New("independent read scope or actor denied")}
        return nil
    }
    stop:=func(context.Context,frinbox.Record)(bool,error){
        if outage{return false,errors.New("independent stop unavailable")}
        return stopped,nil
    }
    pv,err:=NewScopedPDFPreviewHandler(root,principal,lookup,auth,stop)
    if err!=nil{t.Fatal(err)}
    dl,err:=NewScopedOriginalDownloadHandler(root,principal,lookup,auth,stop)
    if err!=nil{t.Fatal(err)}
    mux:=http.NewServeMux()
    mux.Handle("GET /api/v1/jobs/{job_id}/preview",pv)
    mux.Handle("GET /api/v1/jobs/{job_id}/artifact",dl)
    run:=func(route string)(*httptest.ResponseRecorder) {
        t.Helper()
        req:=httptest.NewRequest(http.MethodGet,"/api/v1/jobs/"+record.JobID+"/"+route,nil)
        rec:=httptest.NewRecorder()
        mux.ServeHTTP(rec,req)
        if rec.Header().Get("Cache-Control")!="no-store" ||
            rec.Header().Get("X-Content-Type-Options")!="nosniff" {
            t.Fatalf("unsafe security headers: %v",rec.Header())
        }
        return rec
    }
    check:=func(route string,want int) *httptest.ResponseRecorder{
        t.Helper()
        rec:=run(route)
        if rec.Code!=want{t.Fatalf("route=%s want=%d got=%d body=%s",route,want,rec.Code,rec.Body.String())}
        if want!=http.StatusOK && bytes.Contains(rec.Body.Bytes(),canonical){
            t.Fatal("denied route leaked canonical source")
        }
        return rec
    }
    preview:=check("preview",http.StatusOK)
    if !bytes.Equal(preview.Body.Bytes(),canonical){t.Fatal("preview digest-bound snapshot bytes changed")}
    download:=check("artifact",http.StatusForbidden)
    if strings.HasPrefix(download.Header().Get("Content-Disposition"),"attachment"){
        t.Fatal("denial advertised a downloadable body")
    }
    allowed[ScopedArtifactPreview]=false
    allowed[ScopedArtifactOriginal]=true
    check("preview",http.StatusForbidden)
    original:=check("artifact",http.StatusOK)
    if !bytes.Equal(original.Body.Bytes(),canonical){t.Fatal("original digest-bound snapshot bytes changed")}
    if !strings.HasPrefix(original.Header().Get("Content-Disposition"),"attachment;"){
        t.Fatalf("original failed attachment-only disposition: %q",original.Header().Get("Content-Disposition"))
    }
    if original.Header().Get("Content-Type")!="application/pdf" {t.Fatal("original MIME changed")}
    if seen[ScopedArtifactPreview]<2 || seen[ScopedArtifactOriginal]<3 {
        t.Fatalf("independent purpose policy missing, counts=%v",seen)
    }
    actor="bob"
    check("artifact",http.StatusForbidden)
    actor="alice"
    stopped=true
    check("artifact",http.StatusForbidden)
    stopped=false
    outage=true
    check("artifact",http.StatusForbidden)
    outage=false
    actor=""
    check("artifact",http.StatusUnauthorized)
    actor="alice"
    record.MediaType="image/urf"
    allowed[ScopedArtifactPreview]=true
    check("preview",http.StatusUnsupportedMediaType)
    urf:=check("artifact",http.StatusOK)
    if urf.Header().Get("Content-Type")!="image/urf" || !bytes.Equal(urf.Body.Bytes(),canonical){
        t.Fatal("original bytes and MIME must retain accepted canonical metadata")
    }
    record.MediaType="text/html\r\nX-Injected: true"
    bad:=check("artifact",http.StatusUnsupportedMediaType)
    if bad.Header().Get("X-Injected")!=""{t.Fatal("MIME injected a response header")}
}

func TestScopedOriginalDownloadSafeMultilingualFilename(t *testing.T){
    root,_,record,canonical:=previewFixture(t)
    handler,err:=NewScopedOriginalDownloadHandler(root,
        func(*http.Request)(PreviewPrincipal,error){return PreviewPrincipal{Issuer:"fixture-oidc",Subject:"alice"},nil},
        func(_ context.Context,id string)(frinbox.Record,bool,error){return record,id==record.JobID,nil},
        func(_ context.Context,_ PreviewPrincipal,_ frinbox.Record,scope ScopedArtifactRead)error{
            if scope!=ScopedArtifactOriginal{return errors.New("wrong scope")}
            return nil
        },
        func(context.Context,frinbox.Record)(bool,error){return false,nil},
    )
    if err!=nil{t.Fatal(err)}
    mux:=http.NewServeMux()
    mux.Handle("GET /api/v1/jobs/{job_id}/artifact",handler)
    read:=func() *httptest.ResponseRecorder{
        rec:=httptest.NewRecorder()
        mux.ServeHTTP(rec,httptest.NewRequest(http.MethodGet,"/api/v1/jobs/"+record.JobID+"/artifact",nil))
        return rec
    }
    for _,name:=range []string{`quoted"name.pdf`,"สวัสดี-Deutsch.pdf","test file.pdf"} {
        record.DownloadName=name
        r:=read()
        if r.Code!=http.StatusOK || !bytes.Equal(r.Body.Bytes(),canonical){
            t.Fatalf("valid filename %q refused: status=%d %s",name,r.Code,r.Body.String())
        }
        kind,attrs,err:=mime.ParseMediaType(r.Header().Get("Content-Disposition"))
        if err!=nil || kind!="attachment" || attrs["filename"]!=name {
            t.Fatalf("unsafe name encoding %q: %s attrs=%v err=%v",name,r.Header().Get("Content-Disposition"),attrs,err)
        }
    }
    for _,name:=range []string{
        "bad\r\nX-Injected: yes.pdf","x\tfile.pdf","../parent.pdf",
        "folder/file.pdf",`bad\name.pdf`,string([]byte{0xff}),"",
    }{
        record.DownloadName=name
        r:=read()
        if r.Code!=http.StatusConflict || bytes.Contains(r.Body.Bytes(),canonical) ||
            r.Header().Get("Content-Disposition")!="" || r.Header().Get("X-Injected")!="" {
            t.Fatalf("unsafe name was admitted %q: code=%d headers=%v",name,r.Code,r.Header())
        }
    }
}

func TestScopedOriginalDownloadRevocationAbortsPartialHTTP200(t *testing.T){
    root,record,canonical:=syntheticLongPreviewFixture(t)
    allowed:=true
    authChecks:=0
    handler,err:=NewScopedOriginalDownloadHandler(root,
        func(*http.Request)(PreviewPrincipal,error){return PreviewPrincipal{Issuer:"fixture-oidc",Subject:"alice"},nil},
        func(_ context.Context,id string)(frinbox.Record,bool,error){return record,id==record.JobID,nil},
        func(_ context.Context,_ PreviewPrincipal,_ frinbox.Record,scope ScopedArtifactRead)error {
            authChecks++
            if scope!=ScopedArtifactOriginal || !allowed {return errors.New("original revoked")}
            return nil
        },
        func(context.Context,frinbox.Record)(bool,error){return false,nil},
    )
    if err!=nil{t.Fatal(err)}
    mux:=http.NewServeMux()
    mux.Handle("GET /api/v1/jobs/{job_id}/artifact",handler)
    writer:=&revokingWriter{headers:make(http.Header),revoke:func(){allowed=false}}
    req:=httptest.NewRequest(http.MethodGet,"/api/v1/jobs/"+record.JobID+"/artifact",nil)
    var caught any
    func(){defer func(){caught=recover()}();mux.ServeHTTP(writer,req)}()
    if caught!=http.ErrAbortHandler || writer.status!=http.StatusOK ||
        writer.chunks!=1 || writer.body.Len()!=previewDeliveryChunkBytes || authChecks<4 ||
        !bytes.Equal(writer.body.Bytes(),canonical[:previewDeliveryChunkBytes]){
        t.Fatalf("original partial 200 not truthfully aborted: %v status=%d chunks=%d bytes=%d checks=%d",
            caught,writer.status,writer.chunks,writer.body.Len(),authChecks)
    }
}

func TestScopedOriginalDownloadShortWriteAborts(t *testing.T){
    root,_,record,_:=previewFixture(t)
    handler,err:=NewScopedOriginalDownloadHandler(root,
        func(*http.Request)(PreviewPrincipal,error){return PreviewPrincipal{Issuer:"fixture-oidc",Subject:"alice"},nil},
        func(_ context.Context,id string)(frinbox.Record,bool,error){return record,id==record.JobID,nil},
        func(context.Context,PreviewPrincipal,frinbox.Record,ScopedArtifactRead)error{return nil},
        func(context.Context,frinbox.Record)(bool,error){return false,nil},
    )
    if err!=nil{t.Fatal(err)}
    mux:=http.NewServeMux()
    mux.Handle("GET /api/v1/jobs/{job_id}/artifact",handler)
    output:=&syntheticShortResponseWriter{ResponseRecorder:httptest.NewRecorder()}
    req:=httptest.NewRequest(http.MethodGet,"/api/v1/jobs/"+record.JobID+"/artifact",nil)
    var caught any
    func(){defer func(){caught=recover()}();mux.ServeHTTP(output,req)}()
    if caught!=http.ErrAbortHandler || output.writes!=1 || output.Body.Len()>=int(record.ArtifactBytes){
        t.Fatalf("partial original transfer returned silently: panic=%v calls=%d bytes=%d",caught,output.writes,output.Body.Len())
    }
}

func TestScopedOriginalDownloadRejectsChangedCanonical(t *testing.T){
    root,file,record,_:=previewFixture(t)
    handler,err:=NewScopedOriginalDownloadHandler(root,
        func(*http.Request)(PreviewPrincipal,error){return PreviewPrincipal{Issuer:"fixture-oidc",Subject:"alice"},nil},
        func(_ context.Context,id string)(frinbox.Record,bool,error){return record,id==record.JobID,nil},
        func(context.Context,PreviewPrincipal,frinbox.Record,ScopedArtifactRead)error{return nil},
        func(context.Context,frinbox.Record)(bool,error){return false,nil},
    )
    if err!=nil{t.Fatal(err)}
    mux:=http.NewServeMux()
    mux.Handle("GET /api/v1/jobs/{job_id}/artifact",handler)
    if err:=os.WriteFile(file,bytes.Repeat([]byte("Z"),int(record.ArtifactBytes)),0600);err!=nil{t.Fatal(err)}
    result:=httptest.NewRecorder()
    mux.ServeHTTP(result,httptest.NewRequest(http.MethodGet,"/api/v1/jobs/"+record.JobID+"/artifact",nil))
    if result.Code!=http.StatusConflict || strings.HasPrefix(result.Header().Get("Content-Disposition"),"attachment;"){
        t.Fatalf("corrupt canonical must fail before attachment header, got %d %v",result.Code,result.Header())
    }
}
