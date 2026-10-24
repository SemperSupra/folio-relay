package paperless

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "html/template"
 "net/http"
 "strings"
)

// ReviewPrincipal is supplied by a trusted application authentication layer.
// The adapter never reads spoofable identity headers to construct principals.
type ReviewPrincipal struct {
 ActorID string
 CanRead bool // an explicit effect-scoped grant, not merely login status
}

// ReviewAuthorizer MUST authenticate the request and authorize the exact
// effect ID, including the requesting actor's record-level permission.
// Missing/unknown authorization MUST fail closed.
type ReviewAuthorizer func(r *http.Request, effectID string) (ReviewPrincipal, error)

// ReviewLoader fetches a pre-validated, immutable ReviewEnvelope. This slice
// does not query remote Paperless, access storage, or auto-assign metadata.
type ReviewLoader func(ctx context.Context, effectID string) (ReviewEnvelope, error)

var ErrReviewEndpoint = errors.New("missing review authorization or evidence source")

const reviewPage = `<!doctype html>
<html lang="{{.Language}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Labels.PageTitle}}</title>
</head>
<body>
<main>
<h1>{{.Labels.PageTitle}}</h1>
<p role="status"><strong>{{.Labels.ReviewRequired}}</strong></p>
<p>{{.Labels.ReadOnly}}</p>
<section aria-labelledby="document-heading">
<h2 id="document-heading">{{.Labels.Document}}</h2>
<dl>
<dt>{{.Labels.NativeTitle}}</dt><dd dir="auto">{{.Review.NativeTitle}}</dd>
<dt>{{.Labels.Language}}</dt><dd>{{.Review.Language}}</dd>
<dt>{{.Labels.RecordID}}</dt><dd>{{.Review.PaperlessDocumentID}}</dd>
<dt>{{.Labels.NativeType}}</dt><dd>{{.Type}}</dd>
<dt>{{.Labels.SuggestedTypes}}</dt><dd>{{.SuggestedTypes}}</dd>
<dt>{{.Labels.NativePath}}</dt><dd>{{.Path}}</dd>
<dt>{{.Labels.SuggestedPaths}}</dt><dd>{{.SuggestedPaths}}</dd>
<dt>{{.Labels.NativeTags}}</dt><dd>{{.Tags}}</dd>
<dt>{{.Labels.SuggestedTags}}</dt><dd>{{.SuggestedTags}}</dd>
<dt>{{.Labels.NativeCorrespondent}}</dt><dd>{{.Correspondent}}</dd>
<dt>{{.Labels.SuggestedCorrespondents}}</dt><dd>{{.SuggestedCorrespondents}}</dd>
</dl>
</section>
<section aria-labelledby="warnings-heading">
<h2 id="warnings-heading">{{.Labels.Warnings}}</h2>
<ul>{{range .Review.Risks}}<li dir="auto">{{.}}</li>{{end}}</ul>
</section>
<p>{{.Labels.NoAction}}</p>
</main>
</body></html>`

type reviewLabels struct {
 PageTitle string
 ReviewRequired string
 ReadOnly string
 Document string
 NativeTitle string
 Language string
 RecordID string
 NativeType string
 SuggestedTypes string
 NativePath string
 SuggestedPaths string
 NativeTags string
 SuggestedTags string
 NativeCorrespondent string
 SuggestedCorrespondents string
 Warnings string
 NoAction string
}

func labelsForReview(l ReviewLanguage)(string,reviewLabels) {
 switch l {
 case ReviewGerman:
  return "de", reviewLabels{
   "Dokumentenablage prüfen","Manuelle Prüfung erforderlich",
   "Nur lesbar – keine Änderungen an der Ablage","Dokument",
   "Aktueller Titel","Sprache","Dokumentnummer","Aktueller Dokumenttyp",
   "Vorgeschlagene Dokumenttypen","Aktueller Ablagepfad (ID)",
   "Vorgeschlagene Ablagepfade (IDs)","Aktuelle Schlagwörter (IDs)",
   "Vorgeschlagene Schlagwörter (IDs)","Aktueller Korrespondent (ID)",
   "Vorgeschlagene Korrespondenten (IDs)","Prüfhinweise",
   "Diese Ansicht kann keine Metadaten speichern oder Dateien verschieben.",
  }
 case ReviewThai:
  return "th",reviewLabels{
   "ตรวจสอบการจัดเก็บเอกสาร","ต้องตรวจสอบโดยผู้ใช้",
   "อ่านอย่างเดียว ไม่เปลี่ยนแปลงการจัดเก็บเอกสาร","เอกสาร",
   "ชื่อเอกสารปัจจุบัน","ภาษา","หมายเลขเอกสาร","ประเภทเอกสารปัจจุบัน",
   "ประเภทเอกสารที่แนะนำ","รหัสเส้นทางจัดเก็บปัจจุบัน",
   "รหัสเส้นทางจัดเก็บที่แนะนำ","รหัสแท็กปัจจุบัน",
   "รหัสแท็กที่แนะนำ","รหัสผู้ส่งเอกสารปัจจุบัน",
   "รหัสผู้ส่งเอกสารที่แนะนำ","ข้อควรตรวจสอบ",
   "หน้านี้ไม่สามารถบันทึกข้อมูลหรือย้ายไฟล์ได้",
  }
 default:
  return "en",reviewLabels{
   "Document filing review","Human review required",
   "Read-only – no filing changes","Document",
   "Current title","Language","Record ID","Current document type",
   "Suggested document types","Current storage path ID",
   "Suggested storage path IDs","Current tag IDs","Suggested tag IDs",
   "Current correspondent ID","Suggested correspondent IDs",
   "Review warnings","This view cannot save metadata or move files.",
  }
 }
}
func optionalPositiveID(v *int)string {
 if v==nil{return "—"}
 return fmt.Sprint(*v)
}
func readableIDs(items []int)string {
 if len(items)==0{return "—"}
 pieces:=make([]string,len(items))
 for i,v:=range items {pieces[i]=fmt.Sprint(v)}
 return strings.Join(pieces,", ")
}
type reviewHTML struct {
 Language string
 Labels reviewLabels
 Review ReviewEnvelope
 Type string
 Path string
 Correspondent string
 SuggestedTypes string
 SuggestedPaths string
 SuggestedCorrespondents string
 Tags string
 SuggestedTags string
}

func allowedEnvelope(effectID string,p ReviewEnvelope)bool {
 if effectID!=p.EffectID || !shaRe.MatchString(p.EffectID) ||
    !shaRe.MatchString(p.CanonicalSHA256) || p.PaperlessDocumentID<=0 ||
    p.AutoApplyAllowed || p.ClassificationQualified || !p.RequiresHumanReview ||
    len(p.Risks)<2 || len(p.Risks)>64 || len([]rune(p.NativeTitle))>512 {
    return false
 }
 switch p.Language {
 case ReviewEnglish,ReviewGerman,ReviewThai,ReviewMixed,ReviewUnknown:
 default:return false
 }
 return true
}

// NewReviewHandler constructs two strictly read-only paths:
// /api/filing/reviews/<effect_sha256> machine-readable JSON, and
// /filing/reviews/<effect_sha256> accessible, escaped HTML. Authorization
// and data lookup MUST be injected by the trusted future FolioRelay runtime.
// This is intentionally NOT registered on cmd/foliorelayd or control.Server;
// there is no implicit listener, cookie, token or production endpoint.
func NewReviewHandler(authorize ReviewAuthorizer,load ReviewLoader)(http.Handler,error){
 if authorize==nil||load==nil{return nil,ErrReviewEndpoint}
 htmlTemplate,err:=template.New("review").Parse(reviewPage)
 if err!=nil{return nil,err}
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Cache-Control","private, no-store")
  w.Header().Set("Pragma","no-cache")
  w.Header().Set("X-Content-Type-Options","nosniff")
  w.Header().Set("Referrer-Policy","no-referrer")
  w.Header().Set("X-Frame-Options","DENY")
  w.Header().Set("Content-Security-Policy","default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
  if r.Method!=http.MethodGet {
    w.Header().Set("Allow","GET")
    http.Error(w,"method not allowed",http.StatusMethodNotAllowed)
    return
  }
  const jsonPrefix="/api/filing/reviews/"
  const htmlPrefix="/filing/reviews/"
  isJSON:=strings.HasPrefix(r.URL.Path,jsonPrefix)
  prefix:=htmlPrefix
  if isJSON {prefix=jsonPrefix}
  if !strings.HasPrefix(r.URL.Path,prefix) {
    http.NotFound(w,r);return
  }
  effectID:=strings.TrimPrefix(r.URL.Path,prefix)
  if !shaRe.MatchString(effectID) {
    http.NotFound(w,r);return
  }
  principal,err:=authorize(r,effectID)
  if err!=nil||principal.ActorID=="" {
    http.Error(w,"authentication required",http.StatusUnauthorized);return
  }
  if !principal.CanRead {
    http.Error(w,"forbidden",http.StatusForbidden);return
  }
  packet,err:=load(r.Context(),effectID)
  if err!=nil {
    // Avoid leaking transport/service internals, titles or approval IDs.
    http.Error(w,"review unavailable",http.StatusServiceUnavailable);return
  }
  if !allowedEnvelope(effectID,packet) {
    http.Error(w,"review evidence invalid",http.StatusServiceUnavailable);return
  }
  if isJSON {
    w.Header().Set("Content-Type","application/json; charset=utf-8")
    if err:=json.NewEncoder(w).Encode(packet);err!=nil {return}
    return
  }
  language,labels:=labelsForReview(packet.Language)
  data:=reviewHTML{
    Language:language, Labels:labels, Review:packet,
    Type:optionalPositiveID(packet.NativeDocumentType),
    Path:optionalPositiveID(packet.NativeStoragePath),
    Correspondent:optionalPositiveID(packet.NativeCorrespondent),
    SuggestedTypes:readableIDs(packet.SuggestedDocumentTypes),
    SuggestedPaths:readableIDs(packet.SuggestedStoragePaths),
    SuggestedCorrespondents:readableIDs(packet.SuggestedCorrespondents),
    Tags:readableIDs(packet.NativeTags),
    SuggestedTags:readableIDs(packet.SuggestedTags),
  }
  w.Header().Set("Content-Type","text/html; charset=utf-8")
  // All human-readable document/LLM text is escaped by html/template.
  _=htmlTemplate.Execute(w,data)
 }),nil
}
