package control

// Offline regression for the unregistered source-only preview handler.
// No real identity, document, network service, new route or site grant.
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
    "strconv"
    "strings"
    "testing"

    frinbox "github.com/SemperSupra/folio-relay/internal/inbox"
)

// Synthetic newly verified fixture has >2 delivery chunks so a stream can
// be revoked after one chunk has actually been emitted.
func syntheticLongPreviewFixture(t *testing.T)(string,frinbox.Record,[]byte) {
    t.Helper()
    root:=t.TempDir()
    data:=append([]byte("%PDF-1.7\n"),bytes.Repeat([]byte("P"),3*previewDeliveryChunkBytes+43)...)
    sum:=sha256.Sum256(data)
    hash:=hex.EncodeToString(sum[:])
    path:=filepath.Join(root,"sha256",hash[:2],hash)
    if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{t.Fatal(err)}
    if err:=os.WriteFile(path,data,0600);err!=nil{t.Fatal(err)}
    return root,frinbox.Record{
        JobID:"job-"+strings.Repeat("e",64),State:"accepted",
        MediaType:"application/pdf",ArtifactSHA256:hash,
        ArtifactBytes:int64(len(data)),DownloadName:"synthetic.pdf",
    },data
}

func TestScopedPreviewPartialStreamAbortsRatherThanSilentlyReturning(t *testing.T){
    root,record,source:=syntheticLongPreviewFixture(t)
    aclCalls:=0
    handler,err:=NewPDFPreviewHandler(root,
        func(*http.Request)(PreviewPrincipal,error){return PreviewPrincipal{Issuer:"synthetic-idp",Subject:"alice"},nil},
        func(_ context.Context,id string)(frinbox.Record,bool,error){return record,id==record.JobID,nil},
        func(context.Context,PreviewPrincipal,frinbox.Record)error{
            aclCalls++
            if aclCalls>=4{return errors.New("synthetic actor revoked after first delivery chunk")}
            return nil
        },
        func(context.Context,frinbox.Record)(bool,error){return false,nil},
    )
    if err!=nil{t.Fatal(err)}
    mux:=http.NewServeMux()
    mux.Handle("GET /api/v1/jobs/{job_id}/preview",handler)
    req:=httptest.NewRequest(http.MethodGet,"/api/v1/jobs/"+record.JobID+"/preview",nil)
    recorder:=httptest.NewRecorder()
    var caught any
    func(){
        defer func(){caught=recover()}()
        mux.ServeHTTP(recorder,req)
    }()
    if caught!=http.ErrAbortHandler{
        t.Fatalf("interrupted stream must abort HTTP exchange, got %#v",caught)
    }
    if recorder.Code!=http.StatusOK ||
        recorder.Header().Get("Content-Length")!=strconv.Itoa(len(source)) ||
        recorder.Body.Len()!=previewDeliveryChunkBytes || aclCalls!=4 {
        t.Fatalf("silently incomplete HTTP 200: status=%d declared=%s delivered=%d acl=%d",
            recorder.Code,recorder.Header().Get("Content-Length"),
            recorder.Body.Len(),aclCalls)
    }
    if !bytes.Equal(recorder.Body.Bytes(),source[:previewDeliveryChunkBytes]){
        t.Fatal("first admitted chunk changed or leaked unsupported suffix")
    }
}

// The recorder itself cannot simulate network-level abort semantics. This
// check only proves the handler propagates the sentinel to net/http rather
// than treating a short/failed Write as successful delivery.
type syntheticShortResponseWriter struct {
    *httptest.ResponseRecorder
    writes int
}
func(w *syntheticShortResponseWriter) Write(p []byte)(int,error){
    w.writes++
    n:=len(p)-1
    if n>0{_,_ = w.ResponseRecorder.Write(p[:n])}
    return n,nil
}
func TestScopedPreviewShortHTTPWriteAborts(t *testing.T){
    root,_,record,_:=previewFixture(t)
    handler,err:=NewPDFPreviewHandler(root,
        func(*http.Request)(PreviewPrincipal,error){return PreviewPrincipal{Issuer:"synthetic-idp",Subject:"alice"},nil},
        func(_ context.Context,id string)(frinbox.Record,bool,error){return record,id==record.JobID,nil},
        func(context.Context,PreviewPrincipal,frinbox.Record)error{return nil},
        func(context.Context,frinbox.Record)(bool,error){return false,nil},
    )
    if err!=nil{t.Fatal(err)}
    mux:=http.NewServeMux()
    mux.Handle("GET /api/v1/jobs/{job_id}/preview",handler)
    req:=httptest.NewRequest(http.MethodGet,"/api/v1/jobs/"+record.JobID+"/preview",nil)
    writer:=&syntheticShortResponseWriter{ResponseRecorder:httptest.NewRecorder()}
    var caught any
    func(){
        defer func(){caught=recover()}()
        mux.ServeHTTP(writer,req)
    }()
    if caught!=http.ErrAbortHandler||writer.writes!=1||writer.Body.Len()>=int(record.ArtifactBytes){
        t.Fatalf("silent HTTP short write: abort=%v writes=%d bytes=%d",caught,writer.writes,writer.Body.Len())
    }
}
