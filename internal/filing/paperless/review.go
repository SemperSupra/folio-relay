package paperless

import (
 "errors"
 "fmt"
 "unicode/utf8"
)

// ReviewLanguage is an operator-supplied language annotation. This slice
// does not claim automatic language detection from untrusted OCR/model text.
type ReviewLanguage string

const (
 ReviewEnglish ReviewLanguage = "en"
 ReviewGerman ReviewLanguage = "de"
 ReviewThai ReviewLanguage = "th"
 ReviewMixed ReviewLanguage = "mixed"
 ReviewUnknown ReviewLanguage = "unknown"
)

var ErrReview = errors.New("paperless filing record cannot be presented as an approved decision")

// ReviewEnvelope is inert JSON-compatible evidence for a future authenticated
// reviewer. It contains no source bytes, credentials, actuator path, URL or
// method to patch/delete/re-export DMS metadata. The DMS may have ALREADY
// auto-assigned a type/path before this envelope was built; that is a risk
// visible to the reviewer, not an authorization by FolioRelay.
type ReviewEnvelope struct {
 EffectID string `json:"effect_id"`
 CanonicalSHA256 string `json:"canonical_sha256"`
 PaperlessDocumentID int `json:"paperless_document_id"`
 Language ReviewLanguage `json:"language"`
 NativeTitle string `json:"native_title"`
 NativeDocumentType *int `json:"native_document_type,omitempty"`
 NativeCorrespondent *int `json:"native_correspondent,omitempty"`
 NativeStoragePath *int `json:"native_storage_path,omitempty"`
 NativeTags []int `json:"native_tags"`
 SuggestedDocumentTypes []int `json:"suggested_document_types"`
 SuggestedCorrespondents []int `json:"suggested_correspondents"`
 SuggestedStoragePaths []int `json:"suggested_storage_paths"`
 SuggestedTags []int `json:"suggested_tags"`
 ClassificationQualified bool `json:"classification_qualified"`
 RequiresHumanReview bool `json:"requires_human_review"`
 AutoApplyAllowed bool `json:"auto_apply_allowed"`
 Risks []string `json:"risks"`
}

// BuildReviewEnvelope accepts only a fully archived durable FolioRelay effect
// and the corresponding native Paperless readback. It NEVER approves values,
// changes DMS labels, trusts an LLM-proposed filesystem path or mutates files.
func BuildReviewEnvelope(ticket Ticket, record Record, suggestions Suggestions, language ReviewLanguage)(ReviewEnvelope,error){
 if ticket.State!="archived" || ticket.ID=="" || !shaRe.MatchString(ticket.ID) ||
    ticket.DocumentID<=0 || record.ID!=ticket.DocumentID ||
    ticket.Intent.Validate()!=nil ||
    !utf8.ValidString(record.Title) || len([]rune(record.Title))>512 {
    return ReviewEnvelope{},ErrReview
 }
 switch language {
 case ReviewEnglish,ReviewGerman,ReviewThai,ReviewMixed,ReviewUnknown:
 default: return ReviewEnvelope{},ErrReview
 }
 for _,opt:=range []*int{record.Correspondent,record.DocumentType,record.StoragePath}{
    if opt!=nil && *opt<=0{return ReviewEnvelope{},ErrReview}
 }
 for _,list:=range [][]int{record.Tags,suggestions.DocumentTypes,suggestions.Correspondents,suggestions.StoragePaths,suggestions.Tags}{
    seen:=map[int]bool{}
    if len(list)>1000{return ReviewEnvelope{},ErrReview}
    for _,id:=range list {
      if id<=0||seen[id]{return ReviewEnvelope{},ErrReview}
      seen[id]=true
    }
 }
 // A mismatch is an additional caution; lack of a suggestion is not proof
 // that the native DMS assignment is correct.
 risks:=[]string{
   "Paperless may already have assigned labels during document ingestion",
   "FolioRelay has not qualified native classification for automatic filing",
 }
 if language==ReviewThai||language==ReviewMixed||language==ReviewUnknown {
   risks=append(risks,"Thai or mixed-language OCR/classification requires additional review")
 }
 if record.DocumentType!=nil&&len(suggestions.DocumentTypes)>0 {
   matched:=false
   for _,id:=range suggestions.DocumentTypes{if id==*record.DocumentType {matched=true}}
   if !matched {risks=append(risks,"Native document type conflicts with classic suggestions")}
 }
 if record.StoragePath!=nil {risks=append(risks,"Native DMS storage-path assignment requires independent review")}
 if record.Correspondent!=nil {risks=append(risks,"Native DMS correspondent assignment is not independently verified")}
 p:=ReviewEnvelope{
   EffectID:ticket.ID,
   CanonicalSHA256:ticket.Intent.CanonicalSHA256,
   PaperlessDocumentID:record.ID,
   Language:language,
   NativeTitle:record.Title,
   NativeDocumentType:record.DocumentType,
   NativeCorrespondent:record.Correspondent,
   NativeStoragePath:record.StoragePath,
   NativeTags:append([]int(nil),record.Tags...),
   SuggestedDocumentTypes:append([]int(nil),suggestions.DocumentTypes...),
   SuggestedCorrespondents:append([]int(nil),suggestions.Correspondents...),
   SuggestedStoragePaths:append([]int(nil),suggestions.StoragePaths...),
   SuggestedTags:append([]int(nil),suggestions.Tags...),
   ClassificationQualified:false,
   RequiresHumanReview:true,
   AutoApplyAllowed:false,
   Risks:risks,
 }
 if len(p.Risks)<2{return ReviewEnvelope{},fmt.Errorf("%w: missing caveats",ErrReview)}
 return p,nil
}
