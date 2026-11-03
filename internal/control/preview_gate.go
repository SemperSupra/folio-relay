package control

import (
    "context"
    "errors"
    "fmt"
    "io"
    "net/http"
    "strconv"
    "strings"
    "time"

    frinbox "github.com/SemperSupra/folio-relay/internal/inbox"
)

// PreviewPrincipal is supplied by an independently authenticated user or
// workload identity transport. A management token, job title, and request
// parameters are NEVER an identity source.
type PreviewPrincipal struct {
    Issuer string
    Subject string
}

type PreviewPrincipalResolver func(*http.Request) (PreviewPrincipal, error)
type PreviewRecordLookup func(context.Context, string) (frinbox.Record, bool, error)
type PreviewReadAuthorization func(context.Context, PreviewPrincipal, frinbox.Record) error
type PreviewEmergencyStop func(context.Context, frinbox.Record) (blocked bool, err error)

// NewPDFPreviewHandler creates an UNREGISTERED, read-only consumption handler.
// It cannot be installed in Server.Handler by itself. Four independent trust
// dependencies must be supplied by qualified AAA #96 and document custody #87.
// Tests may use synthetic doubles, but passing those tests is not a site grant.
//
// The copied original source remains owned by the existing bounded,
// hash-verified in-memory snapshot primitive. No second PDF decoder/parser,
// scratch artifact path, CDN or durable metadata writer is introduced.
func NewPDFPreviewHandler(
    canonicalRoot string,
    principal PreviewPrincipalResolver,
    lookup PreviewRecordLookup,
    authorize PreviewReadAuthorization,
    emergency PreviewEmergencyStop,
) (http.Handler, error) {
    if strings.TrimSpace(canonicalRoot)=="" || principal==nil || lookup==nil ||
        authorize==nil || emergency==nil {
        return nil, errors.New("preview requires canonical custody, independent actor, record and emergency authorities")
    }
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Cache-Control", "no-store")
        w.Header().Set("Pragma", "no-cache")
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
        w.Header().Set("Referrer-Policy", "no-referrer")
        w.Header().Set("Accept-Ranges", "none")
        // If opened without the trusted PDF.js UI, browser PDF scripting must
        // not gain the product origin. Renderer UI is always a distinct route.
        w.Header().Set("Content-Security-Policy", "sandbox")
        if r.Method != http.MethodGet {
            w.Header().Set("Allow", "GET")
            writeError(w, http.StatusMethodNotAllowed, "preview_method_denied", "preview unavailable")
            return
        }
        if r.Header.Get("Range")!="" || r.Header.Get("If-Range")!="" {
            writeError(w, http.StatusRequestedRangeNotSatisfiable, "preview_ranges_not_supported", "preview unavailable")
            return
        }
        id:=r.PathValue("job_id")
        if len(id)!=68 || !strings.HasPrefix(id,"job-") {
            writeError(w,http.StatusNotFound,"preview_not_found","preview unavailable")
            return
        }
        for _,letter:=range id[4:] {
            if !(letter>='0' && letter<='9' || letter>='a' && letter<='f') {
                writeError(w,http.StatusNotFound,"preview_not_found","preview unavailable")
                return
            }
        }
        // Real scoped authentication is mandatory. It is intentionally NOT
        // derived from Server.authorized or the installed management session.
        actor,err:=principal(r)
        if err!=nil || strings.TrimSpace(actor.Issuer)=="" ||
            strings.TrimSpace(actor.Subject)=="" ||
            actor.Issuer=="management" || actor.Issuer=="shared-management" {
            writeError(w,http.StatusUnauthorized,"preview_identity_required","preview unavailable")
            return
        }
        ctx,cancel:=context.WithTimeout(r.Context(),5*time.Minute)
        defer cancel()
        record,found,err:=lookup(ctx,id)
        if err!=nil {writeError(w,http.StatusServiceUnavailable,"preview_record_unavailable","preview unavailable");return}
        if !found || record.JobID!=id || record.State!="accepted" {
            writeError(w,http.StatusNotFound,"preview_not_found","preview unavailable")
            return
        }
        // URF conversion is a *different* admitted derivative worker, not a
        // license to serve raw image/urf as an inline PDF.
        if record.MediaType!="application/pdf" {
            writeError(w,http.StatusUnsupportedMediaType,"preview_format_unsupported","preview unavailable")
            return
        }
        guard:=func() bool {
            // Emergency / global stop precedes record ACL, both fail closed.
            blocked,err:=emergency(ctx,record)
            if err!=nil || blocked {return false}
            return authorize(ctx,actor,record)==nil
        }
        if !guard() {
            writeError(w,http.StatusForbidden,"preview_access_denied","preview unavailable")
            return
        }
        snap,receipt:=frinbox.SnapshotVerifiedArtifact(ctx,canonicalRoot,record,time.Now())
        if snap==nil {
            switch receipt.ReasonCode {
            case "snapshot_memory_busy":
                w.Header().Set("Retry-After","2")
                writeError(w,http.StatusTooManyRequests,"preview_busy","preview unavailable")
            case "canonical_artifact_missing":
                writeError(w,http.StatusNotFound,"preview_missing","preview unavailable")
            case "canonical_size_mismatch","canonical_hash_mismatch","canonical_not_regular","canonical_file_changed_during_open","invalid_canonical_reference":
                writeError(w,http.StatusConflict,"preview_corrupt","preview unavailable")
            default:
                writeError(w,http.StatusServiceUnavailable,"preview_unavailable","preview unavailable")
            }
            return
        }
        defer snap.Close()
        // ACL may have been revoked while hashing the original. Recheck the
        // external current identity/document policy immediately before send.
        // NOTE: without a linearizable external stop/read lease, revocation
        // during the subsequent network stream is STILL a separate blocker.
        if ctx.Err()!=nil || !guard() {
            writeError(w,http.StatusForbidden,"preview_access_denied","preview unavailable")
            return
        }
        w.Header().Set("Content-Type","application/pdf")
        w.Header().Set("Content-Disposition",fmt.Sprintf("inline; filename=%q",record.DownloadName))
        w.Header().Set("Content-Length",strconv.FormatInt(snap.Size(),10))
        w.WriteHeader(http.StatusOK)
        // Midstream revocation cannot recall already transmitted bytes, but
        // this bounded loop checks fresh ACL and stop state before each chunk.
        // An external source of linearizable revocation is still REQUIRED.
        _=copyPreviewWithCurrentAccess(ctx,w,snap,guard)
    }),nil
}

// Each read owns no more than 16 KiB of newly deliverable document data.
// The shared #101/#102 verified snapshot still owns and later scrubs the
// complete, previously hash-validated canonical bytes.
const previewDeliveryChunkBytes = 16 << 10

func copyPreviewWithCurrentAccess(ctx context.Context, writer io.Writer, source io.Reader, permitted func() bool) error {
    var chunk [previewDeliveryChunkBytes]byte
    defer clear(chunk[:])
    for {
        if err:=ctx.Err();err!=nil{return err}
        n,readErr:=source.Read(chunk[:])
        if n>0 {
            // A revocation during a blocking read also closes the gate.
            if ctx.Err()!=nil || !permitted() {
                return errors.New("preview delivery revoked or unavailable")
            }
            written,writeErr:=writer.Write(chunk[:n])
            if writeErr!=nil{return writeErr}
            if written!=n{return io.ErrShortWrite}
        }
        if readErr==io.EOF{return nil}
        if readErr!=nil{return readErr}
        if n==0{return io.ErrNoProgress}
    }
}
