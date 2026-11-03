package control

import (
    "bytes"
    "context"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"

    frinbox "github.com/SemperSupra/folio-relay/internal/inbox"
)

// Source-only RED oracle: revocation AFTER the response begins must stop
// subsequent verified memory-snapshot chunks. This does NOT prove that bytes
// already sent are recoverable, nor guarantee distributed linearizability.
type revokingWriter struct {
    headers http.Header
    status int
    body bytes.Buffer
    chunks int
    revoke func()
}
func (w *revokingWriter) Header() http.Header {return w.headers}
func (w *revokingWriter) WriteHeader(status int) {w.status=status}
func (w *revokingWriter) Write(p []byte)(int,error) {
    w.chunks++
    if w.status==0 {w.status=http.StatusOK}
    n,err:=w.body.Write(p)
    if w.chunks==1 {w.revoke()}
    return n,err
}
func TestPDFPreviewDoesNotContinueAfterMidstreamAccessRevocation(t *testing.T) {
    root:=t.TempDir()
    // Large enough to force >1 bounded writer call. Only synthetic bytes.
    payload:=append([]byte("%PDF-1.7\nSYNTHETIC\n"),bytes.Repeat([]byte("R"),256*1024)...)
    digest:=sha256.Sum256(payload)
    hash:=hex.EncodeToString(digest[:])
    file:=filepath.Join(root,"sha256",hash[:2],hash)
    if err:=os.MkdirAll(filepath.Dir(file),0700);err!=nil{t.Fatal(err)}
    if err:=os.WriteFile(file,payload,0600);err!=nil{t.Fatal(err)}
    record:=frinbox.Record{
        JobID:"job-"+strings.Repeat("a",64),State:"accepted",
        MediaType:"application/pdf",ArtifactSHA256:hash,
        ArtifactBytes:int64(len(payload)),DownloadName:"synthetic.pdf",
    }
    currentAllowed:=true
    checks:=0
    handler,err:=NewPDFPreviewHandler(root,
        func(*http.Request)(PreviewPrincipal,error){
            return PreviewPrincipal{Issuer:"synthetic-idp",Subject:"alice"},nil
        },
        func(_ context.Context,id string)(frinbox.Record,bool,error){return record,id==record.JobID,nil},
        func(context.Context,PreviewPrincipal,frinbox.Record)error{
            checks++
            if !currentAllowed{return errors.New("revoked")}
            return nil
        },
        func(context.Context,frinbox.Record)(bool,error){return false,nil},
    )
    if err!=nil{t.Fatal(err)}
    mux:=http.NewServeMux()
    mux.Handle("GET /api/v1/jobs/{job_id}/preview",handler)
    output:=&revokingWriter{headers:make(http.Header),revoke:func(){currentAllowed=false}}
    request:=httptest.NewRequest(http.MethodGet,"/api/v1/jobs/"+record.JobID+"/preview",nil)
    mux.ServeHTTP(output,request)
    if output.status!=http.StatusOK || output.chunks!=1 ||
       output.body.Len()>=len(payload) || checks<3 {
        t.Fatalf("preview must halt at first revoked chunk: status=%d chunks=%d bytes=%d total=%d ACLchecks=%d",
            output.status,output.chunks,output.body.Len(),len(payload),checks)
    }
}
